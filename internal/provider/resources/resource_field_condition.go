// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/fieldconditiongql"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/locks"
)

var _ resource.Resource = &FieldConditionResource{}
var _ resource.ResourceWithImportState = &FieldConditionResource{}

func NewFieldConditionResource() resource.Resource { return &FieldConditionResource{} }

type FieldConditionResource struct{ api *client.ApiClient }

type FieldConditionModel struct {
	Id        types.String                  `tfsdk:"id"`
	PhaseId   types.String                  `tfsdk:"phase_id"`
	Name      types.String                  `tfsdk:"name"`
	Condition *fieldConditionConditionModel `tfsdk:"condition"`
	Actions   []fieldConditionActionModel   `tfsdk:"actions"`
}

type fieldConditionConditionModel struct {
	Groups []fieldConditionGroupModel `tfsdk:"groups"`
}

type fieldConditionGroupModel struct {
	Expressions []fieldConditionExpressionModel `tfsdk:"expressions"`
}

type fieldConditionExpressionModel struct {
	FieldAddress types.String `tfsdk:"field_address"`
	Operation    types.String `tfsdk:"operation"`
	Value        types.String `tfsdk:"value"`
}

type fieldConditionActionModel struct {
	ActionId      types.String `tfsdk:"action_id"`
	PhaseFieldId  types.String `tfsdk:"phase_field_id"`
	WhenEvaluator types.Bool   `tfsdk:"when_evaluator"`
}

func (r *FieldConditionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_field_condition"
}

func (r *FieldConditionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Conditional show/hide (and enable/disable) logic for a phase form. A field condition evaluates a set of expressions and, when they hold, runs actions against phase fields.",
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
				Description: "The criteria that must hold for the actions to run. Groups are ORed together; expressions within a group are ANDed.",
				Attributes: map[string]schema.Attribute{
					"groups": schema.ListNestedAttribute{
						Required:    true,
						Description: "Groups of expressions, ORed together. The condition holds when any group holds.",
						Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"expressions": schema.ListNestedAttribute{
									Required:    true,
									Description: "Comparisons within this group, ANDed together.",
									Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
									NestedObject: schema.NestedAttributeObject{
										Attributes: map[string]schema.Attribute{
											"field_address": schema.StringAttribute{
												Required:    true,
												Description: "The internal_id of the field this expression compares.",
											},
											"operation": schema.StringAttribute{
												Required:    true,
												Description: "The comparison operator (for example equals, not_equals, present, blank). Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference).",
											},
											"value": schema.StringAttribute{
												Optional:    true,
												Description: "The value compared against. Omit for operators that take no value, such as present and blank.",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"actions": schema.ListNestedAttribute{
				Required:    true,
				Description: "What happens to phase fields when the condition holds.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action_id": schema.StringAttribute{
							Required:    true,
							Description: "What to do with the target field (for example show, hide, able, disable). Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference).",
						},
						"phase_field_id": schema.StringAttribute{
							Required:    true,
							Description: "The internal_id of the phase field affected by this action.",
						},
						"when_evaluator": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Whether the action runs when the condition evaluates to true.",
						},
					},
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
	input["condition"] = data.conditionInput()
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
	input["condition"] = data.conditionInput()
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

	unlock, ok := r.lockPhaseRepo(ctx, data.PhaseId.ValueString(), "delete field condition failed", &resp.Diagnostics)
	if !ok {
		return
	}
	defer unlock()

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

// lockPhaseRepo resolves the phase's repo_id and takes the per-repo lock, matching
// the pipefy_field serialization: the API does not accept concurrent field-level
// mutations for the same repo.
func (r *FieldConditionResource) lockPhaseRepo(ctx context.Context, phaseID, errSummary string, diags *diag.Diagnostics) (func(), bool) {
	query := "query GetPhaseRepoId_tf($id:ID!){ phase(id:$id){ repo_id } }"
	var out struct {
		Phase *struct {
			RepoId int `json:"repo_id"`
		} `json:"phase"`
	}
	if err := r.api.DoGraphQL(ctx, query, map[string]any{"id": phaseID}, &out); err != nil {
		diags.AddError(errSummary, fmt.Sprintf("failed to fetch phase repo_id: %s", err.Error()))
		return nil, false
	}
	if out.Phase == nil || out.Phase.RepoId == 0 {
		diags.AddError(errSummary, "could not resolve valid phase repo_id from phase query")
		return nil, false
	}
	return locks.LockRepo(strconv.FormatInt(int64(out.Phase.RepoId), 10)), true
}

// conditionInput flattens condition.groups into the wire ConditionInput shape:
// a flat expressions list plus expressions_structure, assigning fresh
// sequential integer structure_ids in flattening order.
func (m *FieldConditionModel) conditionInput() map[string]any {
	var exprs []map[string]any
	groups := make([][]any, len(m.Condition.Groups))

	nextID := 0
	for gi, g := range m.Condition.Groups {
		row := make([]any, len(g.Expressions))
		for ei, e := range g.Expressions {
			expr := map[string]any{
				"structure_id":  nextID,
				"field_address": e.FieldAddress.ValueString(),
				"operation":     e.Operation.ValueString(),
			}
			if !e.Value.IsNull() && !e.Value.IsUnknown() {
				expr["value"] = e.Value.ValueString()
			}
			exprs = append(exprs, expr)
			row[ei] = nextID
			nextID++
		}
		groups[gi] = row
	}

	return map[string]any{
		"expressions":           exprs,
		"expressions_structure": groups,
	}
}

func (m *FieldConditionModel) actionsInput() []map[string]any {
	actions := make([]map[string]any, len(m.Actions))
	for i, a := range m.Actions {
		action := map[string]any{
			"actionId":     a.ActionId.ValueString(),
			"phaseFieldId": a.PhaseFieldId.ValueString(),
		}
		if !a.WhenEvaluator.IsNull() && !a.WhenEvaluator.IsUnknown() {
			action["whenEvaluator"] = a.WhenEvaluator.ValueBool()
		}
		actions[i] = action
	}
	return actions
}

// applyFieldConditionToModel maps a fetched field condition onto the model.
func applyFieldConditionToModel(data *FieldConditionModel, fc *fieldconditiongql.FieldCondition, diags *diag.Diagnostics) {
	data.Id = types.StringValue(fc.Id)
	data.Name = types.StringValue(fc.Name)
	if fc.Phase != nil && fc.Phase.Id != "" {
		data.PhaseId = types.StringValue(fc.Phase.Id)
	}

	cond := &fieldConditionConditionModel{}
	if fc.Condition != nil {
		cond.Groups = groupsFromFieldCondition(fc.Condition, diags)
	}
	data.Condition = cond

	data.Actions = make([]fieldConditionActionModel, len(fc.Actions))
	for i, a := range fc.Actions {
		var phaseFieldId string
		if a.PhaseField != nil {
			phaseFieldId = a.PhaseField.InternalId
		}
		data.Actions[i] = fieldConditionActionModel{
			ActionId:      types.StringValue(a.ActionId),
			PhaseFieldId:  types.StringValue(phaseFieldId),
			WhenEvaluator: boolPtr(a.WhenEvaluator),
		}
	}
}

// groupsFromFieldCondition reconstructs condition.groups from the API's flat
// expressions plus expressions_structure. Outer order follows
// expressions_structure; inner (within-group) order follows each inner array.
// A structure_id referenced by a group with no matching expression indicates
// an inconsistency in the API response rather than a configuration error, so
// it is reported as a diagnostic rather than silently dropped.
func groupsFromFieldCondition(cond *fieldconditiongql.Condition, diags *diag.Diagnostics) []fieldConditionGroupModel {
	byID := make(map[string]fieldConditionExpressionModel, len(cond.Expressions))
	for _, e := range cond.Expressions {
		byID[e.StructureId] = fieldConditionExpressionModel{
			FieldAddress: types.StringValue(e.FieldAddress),
			Operation:    types.StringValue(e.Operation),
			Value:        strPtr(e.Value),
		}
	}

	groups := make([]fieldConditionGroupModel, len(cond.ExpressionsStructure))
	for gi, ids := range cond.ExpressionsStructure {
		exprs := make([]fieldConditionExpressionModel, 0, len(ids))
		for _, rawID := range ids {
			key := stringifyStructureElem(rawID)
			expr, ok := byID[key]
			if !ok {
				diags.AddError(
					"field condition API inconsistency",
					fmt.Sprintf("expressions_structure group %d references structure_id %q, which has no matching entry in expressions", gi, key),
				)
				continue
			}
			exprs = append(exprs, expr)
		}
		groups[gi] = fieldConditionGroupModel{Expressions: exprs}
	}
	return groups
}

// stringifyStructureElem normalizes an expressions_structure element (which the
// API returns untyped, as a number or a string) to the same string form used to
// key expressions by structure_id.
func stringifyStructureElem(v any) string {
	switch n := v.(type) {
	case string:
		return n
	case float64:
		return strconv.FormatInt(int64(n), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}
