// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/conditionschema"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/fieldconditiongql"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/locks"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

var _ resource.Resource = &FieldConditionResource{}
var _ resource.ResourceWithImportState = &FieldConditionResource{}

func NewFieldConditionResource() resource.Resource { return &FieldConditionResource{} }

type FieldConditionResource struct{ api *client.ApiClient }

type FieldConditionModel struct {
	Id        types.String                `tfsdk:"id"`
	PhaseId   types.String                `tfsdk:"phase_id"`
	Name      types.String                `tfsdk:"name"`
	Condition *conditionschema.Condition  `tfsdk:"condition"`
	Actions   []fieldConditionActionModel `tfsdk:"actions"`
}

type fieldConditionActionModel struct {
	Field     types.String `tfsdk:"field"`
	WhenTrue  types.String `tfsdk:"when_true"`
	WhenFalse types.String `tfsdk:"when_false"`
}

func (r *FieldConditionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_field_condition"
}

func (r *FieldConditionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Conditional show/hide (and enable/disable) logic for a phase form. A field condition evaluates a set of comparisons and, when they hold, runs actions against phase fields.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The ID of the field condition",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"phase_id": schema.StringAttribute{
				Required:      true,
				Description:   "The ID of the phase the condition belongs to. Changing it forces a new field condition.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Name that describes what this condition does"},
			"condition": schema.SingleNestedAttribute{
				Required:    true,
				Description: "The criteria that must hold for the actions to run.",
				Attributes:  conditionschema.Attributes(),
			},
			"actions": schema.ListNestedAttribute{
				Required:    true,
				Description: "What happens to each phase field when the condition holds. One entry per target field.",
				Validators:  []validator.List{validators.FieldConditionActionsUniqueField()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"field": schema.StringAttribute{
							Required:    true,
							Description: "The internal_id of the phase field affected by this action.",
						},
						"when_true": schema.StringAttribute{
							Optional:    true,
							Description: "What to do with the field when the condition evaluates to true (for example show, able). Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference).",
						},
						"when_false": schema.StringAttribute{
							Optional:    true,
							Description: "What to do with the field when the condition evaluates to false (for example hide, disable). Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference).",
						},
					},
					Validators: []validator.Object{validators.FieldConditionActionHasBranch()},
				},
			},
		},
	}
}

func (r *FieldConditionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*client.ApiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *ApiClient, got %T", req.ProviderData))
		return
	}
	r.api = api
}

func (r *FieldConditionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data FieldConditionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock, ok := r.lockPhaseRepo(ctx, data.PhaseId.ValueString(), "create field condition failed", &resp.Diagnostics)
	if !ok {
		return
	}
	defer unlock()

	input := map[string]any{
		"name":    data.Name.ValueString(),
		"phaseId": data.PhaseId.ValueString(),
	}
	input["condition"] = data.Condition.Input()
	input["actions"] = data.actionsInput()

	mutation := "mutation CreateFieldCondition_tf($input:createFieldConditionInput!){ createFieldCondition(input:$input){ fieldCondition{ " + fieldconditiongql.Selection + " } } }"
	var out struct {
		CreateFieldCondition struct {
			FieldCondition *fieldconditiongql.FieldCondition `json:"fieldCondition"`
		} `json:"createFieldCondition"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"input": input}, &out); err != nil {
		resp.Diagnostics.AddError("create field condition failed", err.Error())
		return
	}
	if out.CreateFieldCondition.FieldCondition == nil {
		resp.Diagnostics.AddError("create field condition failed", "the API returned no field condition")
		return
	}
	applyFieldConditionToModel(&data, out.CreateFieldCondition.FieldCondition, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FieldConditionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data FieldConditionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	query := "query GetFieldCondition_tf($id:ID!){ fieldCondition(id:$id){ " + fieldconditiongql.Selection + " } }"
	var out struct {
		FieldCondition *fieldconditiongql.FieldCondition `json:"fieldCondition"`
	}
	if err := r.api.DoGraphQL(ctx, query, map[string]any{"id": data.Id.ValueString()}, &out); err != nil {
		resp.Diagnostics.AddError("read field condition failed", err.Error())
		return
	}
	if out.FieldCondition == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	applyFieldConditionToModel(&data, out.FieldCondition, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FieldConditionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data FieldConditionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock, ok := r.lockPhaseRepo(ctx, data.PhaseId.ValueString(), "update field condition failed", &resp.Diagnostics)
	if !ok {
		return
	}
	defer unlock()

	input := map[string]any{
		"id":       data.Id.ValueString(),
		"name":     data.Name.ValueString(),
		"phase_id": data.PhaseId.ValueString(),
	}
	input["condition"] = data.Condition.Input()
	input["actions"] = data.actionsInput()

	mutation := "mutation UpdateFieldCondition_tf($input:UpdateFieldConditionInput!){ updateFieldCondition(input:$input){ fieldCondition{ " + fieldconditiongql.Selection + " } } }"
	var out struct {
		UpdateFieldCondition struct {
			FieldCondition *fieldconditiongql.FieldCondition `json:"fieldCondition"`
		} `json:"updateFieldCondition"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"input": input}, &out); err != nil {
		resp.Diagnostics.AddError("update field condition failed", err.Error())
		return
	}
	if out.UpdateFieldCondition.FieldCondition == nil {
		resp.Diagnostics.AddError("update field condition failed", "the API returned no field condition")
		return
	}
	applyFieldConditionToModel(&data, out.UpdateFieldCondition.FieldCondition, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FieldConditionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data FieldConditionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unlike Create/Update, a missing phase is not a hard error here: a field
	// condition whose phase was already deleted out-of-band has nothing left
	// to lock, and should still be removable rather than stuck in state.
	repoID, found, err := r.resolvePhaseRepoID(ctx, data.PhaseId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("delete field condition failed", fmt.Sprintf("failed to fetch phase repo_id: %s", err.Error()))
		return
	}
	if found {
		unlock := locks.LockRepo(repoID)
		defer unlock()
	}

	mutation := "mutation DeleteFieldCondition_tf($id:ID!){ deleteFieldCondition(input:{id:$id}){ success } }"
	var out struct {
		DeleteFieldCondition struct {
			Success bool `json:"success"`
		} `json:"deleteFieldCondition"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"id": data.Id.ValueString()}, &out); err != nil {
		resp.Diagnostics.AddError("delete field condition failed", err.Error())
		return
	}
}

func (r *FieldConditionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The field condition id is enough; Read resolves phase_id, name, condition,
	// and actions from the API (phase_id comes from the payload's phase.id).
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// lockPhaseRepo resolves the phase's repo_id and takes the per-repo lock,
// matching the pipefy_field serialization: the API does not accept
// concurrent field-level mutations for the same repo. Create/Update treat a
// missing phase as a hard error since phase_id is user-supplied and should
// resolve; Delete calls resolvePhaseRepoID directly instead so it can
// tolerate a missing phase.
func (r *FieldConditionResource) lockPhaseRepo(ctx context.Context, phaseID, errSummary string, diags *diag.Diagnostics) (func(), bool) {
	repoID, found, err := r.resolvePhaseRepoID(ctx, phaseID)
	if err != nil {
		diags.AddError(errSummary, fmt.Sprintf("failed to fetch phase repo_id: %s", err.Error()))
		return nil, false
	}
	if !found {
		diags.AddError(errSummary, "could not resolve valid phase repo_id from phase query")
		return nil, false
	}
	return locks.LockRepo(repoID), true
}

// resolvePhaseRepoID fetches the repo_id owning phaseID. found is false when
// the phase query resolves but returns no phase or a zero repo_id (the phase
// no longer exists), a distinct and expected condition from a query error.
func (r *FieldConditionResource) resolvePhaseRepoID(ctx context.Context, phaseID string) (repoID string, found bool, err error) {
	query := "query GetPhaseRepoId_tf($id:ID!){ phase(id:$id){ repo_id } }"
	var out struct {
		Phase *struct {
			RepoId int `json:"repo_id"`
		} `json:"phase"`
	}
	if err := r.api.DoGraphQL(ctx, query, map[string]any{"id": phaseID}, &out); err != nil {
		return "", false, err
	}
	if out.Phase == nil || out.Phase.RepoId == 0 {
		return "", false, nil
	}
	return strconv.FormatInt(int64(out.Phase.RepoId), 10), true, nil
}

func (m *FieldConditionModel) actionsInput() []map[string]any {
	var actions []map[string]any
	for _, a := range m.Actions {
		field := a.Field.ValueString()
		if !a.WhenTrue.IsNull() && !a.WhenTrue.IsUnknown() {
			actions = append(actions, map[string]any{
				"actionId":      a.WhenTrue.ValueString(),
				"phaseFieldId":  field,
				"whenEvaluator": true,
			})
		}
		if !a.WhenFalse.IsNull() && !a.WhenFalse.IsUnknown() {
			actions = append(actions, map[string]any{
				"actionId":      a.WhenFalse.ValueString(),
				"phaseFieldId":  field,
				"whenEvaluator": false,
			})
		}
	}
	return actions
}

// applyFieldConditionToModel maps a fetched field condition onto the model.
func applyFieldConditionToModel(data *FieldConditionModel, fc *fieldconditiongql.FieldCondition, diags *diag.Diagnostics) {
	data.Id = types.StringValue(fc.Id)
	data.Name = types.StringValue(fc.Name)
	// Observed against the live API: the field condition is associated with
	// the pipe's start-form phase regardless of the phaseId passed to
	// createFieldCondition, so fc.Phase.Id can legitimately differ from the
	// phase_id the config requested. This assigns whatever the API reports
	// rather than trusting the request, since that's the actual owning phase.
	if fc.Phase != nil && fc.Phase.Id != "" {
		data.PhaseId = types.StringValue(fc.Phase.Id)
	}

	cond, err := conditionschema.FromPayload(fc.Condition)
	if err != nil {
		diags.AddError("field condition API inconsistency", err.Error())
		return
	}
	// condition is Required here, so it must never settle to null.
	if cond == nil {
		cond = &conditionschema.Condition{}
	}
	data.Condition = cond

	data.Actions = actionsFromFieldCondition(fc.Actions, diags)
}

// actionsFromFieldCondition groups the API's flat actions by target field,
// first-seen order fixing each field's position in the result, and routes
// each action into when_true or when_false by its whenEvaluator flag. A wire
// action with no whenEvaluator is the same class of inconsistency as an
// orphan structure_id: reported rather than guessed.
func actionsFromFieldCondition(actions []fieldconditiongql.Action, diags *diag.Diagnostics) []fieldConditionActionModel {
	order := make([]string, 0, len(actions))
	byField := make(map[string]*fieldConditionActionModel, len(actions))

	for _, a := range actions {
		var fieldID string
		if a.PhaseField != nil {
			fieldID = a.PhaseField.InternalId
		}
		entry, ok := byField[fieldID]
		if !ok {
			entry = &fieldConditionActionModel{
				Field:     types.StringValue(fieldID),
				WhenTrue:  types.StringNull(),
				WhenFalse: types.StringNull(),
			}
			byField[fieldID] = entry
			order = append(order, fieldID)
		}
		if a.WhenEvaluator == nil {
			diags.AddError(
				"field condition API inconsistency",
				fmt.Sprintf("action on field %q has no whenEvaluator, so it cannot be assigned to when_true or when_false", fieldID),
			)
			continue
		}
		if *a.WhenEvaluator {
			entry.WhenTrue = types.StringValue(a.ActionId)
		} else {
			entry.WhenFalse = types.StringValue(a.ActionId)
		}
	}

	result := make([]fieldConditionActionModel, len(order))
	for i, fieldID := range order {
		result[i] = *byField[fieldID]
	}
	return result
}
