// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/conditionschema"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

var _ resource.Resource = &FieldConditionResource{}
var _ resource.ResourceWithImportState = &FieldConditionResource{}

func NewFieldConditionResource() resource.Resource { return &FieldConditionResource{} }

type FieldConditionResource struct{ api *pipefy.Client }

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
	api, ok := req.ProviderData.(*pipefy.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *pipefy.Client, got %T", req.ProviderData))
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

	input := map[string]any{
		"name":    data.Name.ValueString(),
		"phaseId": data.PhaseId.ValueString(),
	}
	input["condition"] = data.Condition.Input()
	input["actions"] = data.actionsInput()

	requestedPhase := data.PhaseId.ValueString()
	fc, err := r.api.FieldConditions.Create(ctx, requestedPhase, input)
	if errors.Is(err, pipefy.ErrNoFieldCondition) {
		resp.Diagnostics.AddError("create field condition failed", "the API returned no field condition")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("create field condition failed", err.Error())
		return
	}
	if detail := phaseMismatchDetail(requestedPhase, &fc); detail != "" {
		r.rollbackCreate(ctx, requestedPhase, fc.ID, detail, resp)
		return
	}
	applyFieldConditionToModel(&data, &fc, &resp.Diagnostics)
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

	fc, err := r.api.FieldConditions.Get(ctx, data.Id.ValueString())
	if errors.Is(err, pipefy.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("read field condition failed", err.Error())
		return
	}
	applyFieldConditionToModel(&data, &fc, &resp.Diagnostics)
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

	input := map[string]any{
		"id":       data.Id.ValueString(),
		"name":     data.Name.ValueString(),
		"phase_id": data.PhaseId.ValueString(),
	}
	input["condition"] = data.Condition.Input()
	input["actions"] = data.actionsInput()

	requestedPhase := data.PhaseId.ValueString()
	fc, err := r.api.FieldConditions.Update(ctx, requestedPhase, input)
	if errors.Is(err, pipefy.ErrNoFieldCondition) {
		resp.Diagnostics.AddError("update field condition failed", "the API returned no field condition")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("update field condition failed", err.Error())
		return
	}
	// No rollback: the condition already exists and prior state still describes
	// it, so a later refresh reports whatever phase the API now claims.
	if detail := phaseMismatchDetail(requestedPhase, &fc); detail != "" {
		resp.Diagnostics.AddError("update field condition failed", detail)
		return
	}
	applyFieldConditionToModel(&data, &fc, &resp.Diagnostics)
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

	if err := r.api.FieldConditions.Delete(ctx, data.PhaseId.ValueString(), data.Id.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete field condition failed", err.Error())
		return
	}
}

func (r *FieldConditionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The field condition id is enough; Read resolves phase_id, name, condition,
	// and actions from the API (phase_id comes from the payload's phase.id).
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// phaseMismatchDetail describes a write that landed on a phase other than the
// one it asked for. Observed against the live API: createFieldCondition
// attaches the condition to the pipe's start-form phase whatever phaseId the
// request carries. Recording either phase misrepresents the result, one
// breaking the apply and the other claiming a phase the condition is not on,
// so the write is reported as the failure it is. A response with no phase is
// nothing to contradict.
func phaseMismatchDetail(requested string, fc *pipefy.FieldCondition) string {
	if fc.Phase == nil || fc.Phase.ID == "" || fc.Phase.ID == requested {
		return ""
	}
	return fmt.Sprintf(
		"the field condition was requested on phase %s, but the API attached it to phase %s. "+
			"Pipefy currently attaches every field condition to the pipe's start form phase, "+
			"whatever phase the request names, so a condition on any other phase cannot be kept "+
			"in sync. Point phase_id at the pipe's start form phase (start_form_phase_id on the "+
			"pipefy_pipe resource or data source) until the API honors the requested phase.",
		requested, fc.Phase.ID,
	)
}

// rollbackCreate deletes a condition the provider created but cannot manage,
// so a failed apply leaves nothing behind for no state to own. It follows the
// AI agent resource: report the original failure, or the orphan with both
// failures when the delete fails too.
func (r *FieldConditionResource) rollbackCreate(ctx context.Context, phaseID, id, detail string, resp *resource.CreateResponse) {
	if rollbackErr := r.api.FieldConditions.Delete(ctx, phaseID, id); rollbackErr != nil {
		resp.Diagnostics.AddError(
			"create field condition failed and rollback failed",
			fmt.Sprintf("field condition %q is orphaned: %s; rollback failed: %v", id, detail, rollbackErr),
		)
		return
	}
	resp.State.RemoveResource(ctx)
	resp.Diagnostics.AddError("create field condition failed", detail)
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
func applyFieldConditionToModel(data *FieldConditionModel, fc *pipefy.FieldCondition, diags *diag.Diagnostics) {
	data.Id = types.StringValue(fc.ID)
	data.Name = types.StringValue(fc.Name)
	if fc.Phase != nil && fc.Phase.ID != "" {
		data.PhaseId = types.StringValue(fc.Phase.ID)
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
func actionsFromFieldCondition(actions []pipefy.FieldConditionAction, diags *diag.Diagnostics) []fieldConditionActionModel {
	order := make([]string, 0, len(actions))
	byField := make(map[string]*fieldConditionActionModel, len(actions))

	for _, a := range actions {
		var fieldID string
		if a.PhaseField != nil {
			fieldID = a.PhaseField.InternalID
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
			entry.WhenTrue = types.StringValue(a.ActionID)
		} else {
			entry.WhenFalse = types.StringValue(a.ActionID)
		}
	}

	result := make([]fieldConditionActionModel, len(order))
	for i, fieldID := range order {
		result[i] = *byField[fieldID]
	}
	return result
}
