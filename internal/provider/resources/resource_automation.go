// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
)

var _ resource.Resource = &AutomationResource{}
var _ resource.ResourceWithImportState = &AutomationResource{}

func NewAutomationResource() resource.Resource { return &AutomationResource{} }

type AutomationResource struct{ api *client.ApiClient }

type automationCronModel struct {
	Minute     types.String `tfsdk:"minute"`
	Hour       types.String `tfsdk:"hour"`
	DayOfMonth types.String `tfsdk:"day_of_month"`
	Month      types.String `tfsdk:"month"`
	DayOfWeek  types.String `tfsdk:"day_of_week"`
}

type automationSearchConditionModel struct {
	Field     types.String `tfsdk:"field"`
	Id        types.String `tfsdk:"id"`
	Operation types.String `tfsdk:"operation"`
	Value     types.String `tfsdk:"value"`
}

type automationEventParamsModel struct {
	TriggerFieldIds     []types.String `tfsdk:"trigger_field_ids"`
	FromPhaseId         types.String   `tfsdk:"from_phase_id"`
	InPhaseId           types.String   `tfsdk:"in_phase_id"`
	ToPhaseId           types.String   `tfsdk:"to_phase_id"`
	TriggerAutomationId types.String   `tfsdk:"trigger_automation_id"`
	KindOfSla           types.String   `tfsdk:"kind_of_sla"`
}

type automationConditionExpressionModel struct {
	FieldAddress types.String `tfsdk:"field_address"`
	StructureId  types.String `tfsdk:"structure_id"`
	Operation    types.String `tfsdk:"operation"`
	Value        types.String `tfsdk:"value"`
}

type automationConditionModel struct {
	Expressions          []automationConditionExpressionModel `tfsdk:"expressions"`
	ExpressionsStructure [][]types.String                     `tfsdk:"expressions_structure"`
}

var searchForObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"field":     types.StringType,
	"id":        types.StringType,
	"operation": types.StringType,
	"value":     types.StringType,
}}

type AutomationModel struct {
	Id                 types.String                     `tfsdk:"id"`
	Name               types.String                     `tfsdk:"name"`
	EventId            types.String                     `tfsdk:"event_id"`
	ActionId           types.String                     `tfsdk:"action_id"`
	EventRepoId        types.String                     `tfsdk:"event_repo_id"`
	ActionRepoId       types.String                     `tfsdk:"action_repo_id"`
	EventParams        *automationEventParamsModel      `tfsdk:"event_params"`
	ActionParams       types.String                     `tfsdk:"action_params"`
	Condition          *automationConditionModel        `tfsdk:"condition"`
	Active             types.Bool                       `tfsdk:"active"`
	SchedulerFrequency types.String                     `tfsdk:"scheduler_frequency"`
	SchedulerCron      *automationCronModel             `tfsdk:"scheduler_cron"`
	SearchFor          []automationSearchConditionModel `tfsdk:"search_for"`
	ResponseSchema     jsontypes.Normalized             `tfsdk:"response_schema"`
}

type automationErrorDetail struct {
	ObjectName string   `json:"object_name"`
	ObjectKey  string   `json:"object_key"`
	Messages   []string `json:"messages"`
}

func formatAutomationErrorDetails(details []automationErrorDetail) string {
	lines := make([]string, len(details))
	for i, d := range details {
		label := d.ObjectName
		if d.ObjectKey != "" {
			label = strings.TrimSpace(label + " (" + d.ObjectKey + ")")
		}
		segments := make([]string, 0, 2)
		if label != "" {
			segments = append(segments, label)
		}
		if msg := strings.Join(d.Messages, "; "); msg != "" {
			segments = append(segments, msg)
		}
		lines[i] = strings.Join(segments, ": ")
	}
	return strings.Join(lines, "\n")
}

func automationError(automationPresent bool, details []automationErrorDetail, err error) string {
	if automationPresent {
		return ""
	}
	if detail := formatAutomationErrorDetails(details); detail != "" {
		return detail
	}
	if err != nil {
		return err.Error()
	}
	return "the API returned no automation and no error_details"
}

// automationSelection is the field set read back for an automation. Read uses it
// to refresh state so out-of-band changes are detected.
const automationSelection = "id name active event_id action_id " +
	"event_repo{ id } action_repo_v2{ ... on Pipe{ id } ... on Table{ id } } " +
	"scheduler_frequency schedulerCron{ minute hour dayOfMonth month dayOfWeek } " +
	"searchFor{ field id operation value } responseSchema " +
	"event_params{ triggerFieldIds fromPhaseId inPhaseId to_phase_id triggerAutomationId kindOfSla } " +
	"condition{ expressions{ field_address structure_id operation value } expressions_structure }"

type automationRepoRef struct {
	Id string `json:"id"`
}

type automationCron struct {
	Minute     *string `json:"minute"`
	Hour       *string `json:"hour"`
	DayOfMonth *string `json:"dayOfMonth"`
	Month      *string `json:"month"`
	DayOfWeek  *string `json:"dayOfWeek"`
}

type automationSearchCondition struct {
	Field     string  `json:"field"`
	Id        string  `json:"id"`
	Operation string  `json:"operation"`
	Value     *string `json:"value"`
}

type automationEventParamsData struct {
	TriggerFieldIds     []string `json:"triggerFieldIds"`
	FromPhaseId         *string  `json:"fromPhaseId"`
	InPhaseId           *string  `json:"inPhaseId"`
	ToPhaseId           *string  `json:"to_phase_id"`
	TriggerAutomationId *string  `json:"triggerAutomationId"`
	KindOfSla           *string  `json:"kindOfSla"`
}

type automationConditionExpressionData struct {
	FieldAddress string  `json:"field_address"`
	StructureId  string  `json:"structure_id"`
	Operation    string  `json:"operation"`
	Value        *string `json:"value"`
}

type automationConditionData struct {
	Expressions          []automationConditionExpressionData `json:"expressions"`
	ExpressionsStructure [][]string                          `json:"expressions_structure"`
}

type automationData struct {
	Id                 string                      `json:"id"`
	Name               string                      `json:"name"`
	Active             *bool                       `json:"active"`
	EventId            string                      `json:"event_id"`
	ActionId           string                      `json:"action_id"`
	EventRepo          *automationRepoRef          `json:"event_repo"`
	ActionRepoV2       *automationRepoRef          `json:"action_repo_v2"`
	SchedulerFrequency *string                     `json:"scheduler_frequency"`
	SchedulerCron      *automationCron             `json:"schedulerCron"`
	SearchFor          []automationSearchCondition `json:"searchFor"`
	ResponseSchema     json.RawMessage             `json:"responseSchema"`
	EventParams        *automationEventParamsData  `json:"event_params"`
	Condition          *automationConditionData    `json:"condition"`
}

// automationOptionalString maps a nullable API string to state: a null becomes
// a null attribute, and any present value (including an empty string) is kept
// verbatim so it round-trips.
func automationOptionalString(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// automationCronToModel maps the API's cron back to the nested block. A
// non-scheduler automation returns an all-null cron object, which maps to no
// block so it matches an unset config.
func automationCronToModel(c *automationCron) *automationCronModel {
	if c == nil || (c.Minute == nil && c.Hour == nil && c.DayOfMonth == nil && c.Month == nil && c.DayOfWeek == nil) {
		return nil
	}
	deref := func(p *string) types.String {
		if p == nil {
			return types.StringValue("")
		}
		return types.StringValue(*p)
	}
	return &automationCronModel{
		Minute:     deref(c.Minute),
		Hour:       deref(c.Hour),
		DayOfMonth: deref(c.DayOfMonth),
		Month:      deref(c.Month),
		DayOfWeek:  deref(c.DayOfWeek),
	}
}

// automationEventParamsToModel maps the API's event params back to the nested
// block. A null object, or one whose fields are all empty, maps to no block so
// it matches an unset config. The API adds a phase object that the input never
// carried; it is not selected and not mapped.
func automationEventParamsToModel(e *automationEventParamsData) *automationEventParamsModel {
	if e == nil {
		return nil
	}
	empty := len(e.TriggerFieldIds) == 0 && e.FromPhaseId == nil && e.InPhaseId == nil &&
		e.ToPhaseId == nil && e.TriggerAutomationId == nil && e.KindOfSla == nil
	if empty {
		return nil
	}
	var ids []types.String
	if len(e.TriggerFieldIds) > 0 {
		ids = make([]types.String, len(e.TriggerFieldIds))
		for i, v := range e.TriggerFieldIds {
			ids[i] = types.StringValue(v)
		}
	}
	return &automationEventParamsModel{
		TriggerFieldIds:     ids,
		FromPhaseId:         automationOptionalString(e.FromPhaseId),
		InPhaseId:           automationOptionalString(e.InPhaseId),
		ToPhaseId:           automationOptionalString(e.ToPhaseId),
		TriggerAutomationId: automationOptionalString(e.TriggerAutomationId),
		KindOfSla:           automationOptionalString(e.KindOfSla),
	}
}

// automationConditionToModel maps the API's condition back to the nested block.
// A null object, or one with no expressions, maps to no block so it matches an
// unset config. The server-assigned expression id and the wrapper id /
// related_cards are not selected and not mapped.
func automationConditionToModel(c *automationConditionData) *automationConditionModel {
	if c == nil || len(c.Expressions) == 0 {
		return nil
	}
	exprs := make([]automationConditionExpressionModel, len(c.Expressions))
	for i, e := range c.Expressions {
		exprs[i] = automationConditionExpressionModel{
			FieldAddress: types.StringValue(e.FieldAddress),
			StructureId:  types.StringValue(e.StructureId),
			Operation:    types.StringValue(e.Operation),
			Value:        automationOptionalString(e.Value),
		}
	}
	structure := make([][]types.String, len(c.ExpressionsStructure))
	for i, grp := range c.ExpressionsStructure {
		g := make([]types.String, len(grp))
		for j, s := range grp {
			g[j] = types.StringValue(s)
		}
		structure[i] = g
	}
	return &automationConditionModel{
		Expressions:          exprs,
		ExpressionsStructure: structure,
	}
}

func automationNormalizeJSON(raw json.RawMessage) jsontypes.Normalized {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(trimmed)
}

// apply refreshes the round-trippable attributes from a fetched automation.
// search_for and condition are managed in full: search_for always maps to a
// list (empty, not null, when the automation has no conditions), and
// condition maps to no block when the automation has no expressions, so an
// empty config settles cleanly. action_params is left untouched: its read
// type differs from the write input and does not round-trip.
func (m *AutomationModel) apply(a *automationData) {
	m.Id = types.StringValue(a.Id)
	m.Name = types.StringValue(a.Name)
	if a.Active != nil {
		m.Active = types.BoolValue(*a.Active)
	}
	m.EventId = types.StringValue(a.EventId)
	m.ActionId = types.StringValue(a.ActionId)
	if a.EventRepo != nil && a.EventRepo.Id != "" {
		m.EventRepoId = types.StringValue(a.EventRepo.Id)
	}
	if a.ActionRepoV2 != nil && a.ActionRepoV2.Id != "" {
		m.ActionRepoId = types.StringValue(a.ActionRepoV2.Id)
	}
	m.SchedulerFrequency = automationOptionalString(a.SchedulerFrequency)
	m.SchedulerCron = automationCronToModel(a.SchedulerCron)
	conds := make([]automationSearchConditionModel, len(a.SearchFor))
	for i, c := range a.SearchFor {
		conds[i] = automationSearchConditionModel{
			Field:     types.StringValue(c.Field),
			Id:        types.StringValue(c.Id),
			Operation: types.StringValue(c.Operation),
			Value:     automationOptionalString(c.Value),
		}
	}
	m.SearchFor = conds
	m.ResponseSchema = automationNormalizeJSON(a.ResponseSchema)
	m.EventParams = automationEventParamsToModel(a.EventParams)
	m.Condition = automationConditionToModel(a.Condition)
}

// addAutomationOptionalInputs adds the optional inputs shared by Create and
// Update. action_params is sent as a decoded value from its JSON string;
// scheduler_cron uses the API's camelCase field keys, and event_params mixes
// snake_case and camelCase keys per its subfield. search_for and condition
// are always sent (an empty list for search_for, an empty condition object
// for condition) when unset, so both are managed in full. It returns false
// after recording a diagnostic when a JSON string cannot be parsed.
func addAutomationOptionalInputs(input map[string]any, data *AutomationModel, diags *diag.Diagnostics) bool {
	if !data.ActionParams.IsNull() && data.ActionParams.ValueString() != "" {
		var v any
		if err := json.Unmarshal([]byte(data.ActionParams.ValueString()), &v); err != nil {
			diags.AddError("invalid action_params JSON", err.Error())
			return false
		}
		input["action_params"] = v
	}
	if data.EventParams != nil {
		ev := data.EventParams
		ep := map[string]any{}
		if len(ev.TriggerFieldIds) > 0 {
			ids := make([]string, len(ev.TriggerFieldIds))
			for i, v := range ev.TriggerFieldIds {
				ids[i] = v.ValueString()
			}
			ep["triggerFieldIds"] = ids
		}
		if !ev.FromPhaseId.IsNull() {
			ep["fromPhaseId"] = ev.FromPhaseId.ValueString()
		}
		if !ev.InPhaseId.IsNull() {
			ep["inPhaseId"] = ev.InPhaseId.ValueString()
		}
		if !ev.ToPhaseId.IsNull() {
			ep["to_phase_id"] = ev.ToPhaseId.ValueString()
		}
		if !ev.TriggerAutomationId.IsNull() {
			ep["triggerAutomationId"] = ev.TriggerAutomationId.ValueString()
		}
		if !ev.KindOfSla.IsNull() {
			ep["kindOfSla"] = ev.KindOfSla.ValueString()
		}
		input["event_params"] = ep
	}
	if !data.SchedulerFrequency.IsNull() && data.SchedulerFrequency.ValueString() != "" {
		input["scheduler_frequency"] = data.SchedulerFrequency.ValueString()
	}
	if data.SchedulerCron != nil {
		input["schedulerCron"] = map[string]any{
			"minute":     data.SchedulerCron.Minute.ValueString(),
			"hour":       data.SchedulerCron.Hour.ValueString(),
			"dayOfMonth": data.SchedulerCron.DayOfMonth.ValueString(),
			"month":      data.SchedulerCron.Month.ValueString(),
			"dayOfWeek":  data.SchedulerCron.DayOfWeek.ValueString(),
		}
	}
	conds := make([]map[string]any, len(data.SearchFor))
	for i, c := range data.SearchFor {
		cond := map[string]any{
			"field":     c.Field.ValueString(),
			"id":        c.Id.ValueString(),
			"operation": c.Operation.ValueString(),
		}
		if !c.Value.IsNull() {
			cond["value"] = c.Value.ValueString()
		}
		conds[i] = cond
	}
	input["searchFor"] = conds
	if !data.ResponseSchema.IsNull() && data.ResponseSchema.ValueString() != "" {
		var v any
		if err := json.Unmarshal([]byte(data.ResponseSchema.ValueString()), &v); err != nil {
			diags.AddError("invalid response_schema JSON", err.Error())
			return false
		}
		input["responseSchema"] = v
	}
	condInput := map[string]any{
		"expressions":           []map[string]any{},
		"expressions_structure": [][]string{},
	}
	if data.Condition != nil {
		exprs := make([]map[string]any, len(data.Condition.Expressions))
		for i, e := range data.Condition.Expressions {
			expr := map[string]any{
				"field_address": e.FieldAddress.ValueString(),
				"operation":     e.Operation.ValueString(),
				"structure_id":  e.StructureId.ValueString(),
			}
			if !e.Value.IsNull() {
				expr["value"] = e.Value.ValueString()
			}
			exprs[i] = expr
		}
		structure := make([][]string, len(data.Condition.ExpressionsStructure))
		for i, grp := range data.Condition.ExpressionsStructure {
			g := make([]string, len(grp))
			for j, s := range grp {
				g[j] = s.ValueString()
			}
			structure[i] = g
		}
		condInput["expressions"] = exprs
		condInput["expressions_structure"] = structure
	}
	input["condition"] = condInput
	return true
}

func (r *AutomationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_automation"
}

func (r *AutomationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Automation resource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name":           schema.StringAttribute{Required: true, Description: "Name of the automation"},
			"event_id":       schema.StringAttribute{Required: true, Description: "The type of the event that the automation listens to"},
			"action_id":      schema.StringAttribute{Required: true, Description: "The type of the action that the automation performs"},
			"event_repo_id":  schema.StringAttribute{Required: true, Description: "The ID of the pipe that the automation listens to", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"action_repo_id": schema.StringAttribute{Required: true, Description: "The ID of the pipe that the automation performs actions on"},
			"event_params": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Parameters of the event the automation listens to. Which subfields apply depends on event_id; see the API reference (https://developers.pipefy.com/reference/automation-creation). Removing the whole block does not clear it on the server.",
				Attributes: map[string]schema.Attribute{
					"trigger_field_ids":     schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Field ids whose update triggers the automation."},
					"from_phase_id":         schema.StringAttribute{Optional: true, Description: "Source phase id for phase-based events."},
					"in_phase_id":           schema.StringAttribute{Optional: true, Description: "Phase id the event is scoped to."},
					"to_phase_id":           schema.StringAttribute{Optional: true, Description: "Destination phase id for move events."},
					"trigger_automation_id": schema.StringAttribute{Optional: true, Description: "Id of the automation that triggers this one."},
					"kind_of_sla":           schema.StringAttribute{Optional: true, Description: "SLA kind for sla_based events."},
				},
			},
			// action_params is a JSON string to avoid over-modeling in the Terraform schema.
			"action_params": schema.StringAttribute{Optional: true, Description: "The parameters of the action for the automation, as a JSON string. Not read back from the API, so drift is not detected."},
			"condition": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Condition that gates the automation. Managed in full: the configured expressions are authoritative, and omitting the block clears the condition on the server.",
				Attributes: map[string]schema.Attribute{
					"expressions": schema.ListNestedAttribute{
						Required:    true,
						Description: "Condition expressions.",
						Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"field_address": schema.StringAttribute{Required: true, Description: "Field id the expression tests."},
								"operation":     schema.StringAttribute{Required: true, Description: "Comparison operation. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference)."},
								"value":         schema.StringAttribute{Optional: true, Description: "Value to compare against."},
								"structure_id":  schema.StringAttribute{Required: true, Description: "Caller-assigned handle referenced by expressions_structure."},
							},
						},
					},
					"expressions_structure": schema.ListAttribute{
						Required:    true,
						ElementType: types.ListType{ElemType: types.StringType},
						Description: "Boolean grouping of expressions by structure_id. Outer list is OR, inner lists are AND.",
						Validators:  []validator.List{listvalidator.SizeAtLeast(1)},
					},
				},
			},
			"active": schema.BoolAttribute{Required: true, Description: "Whether the automation is active."},
			"scheduler_frequency": schema.StringAttribute{
				Optional:    true,
				Description: "Frequency for time-based (scheduler) triggers. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference/automation-creation) and the GraphiQL explorer (https://app.pipefy.com/graphiql).",
			},
			"scheduler_cron": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Cron schedule for time-based (scheduler) triggers. Fields use standard crontab syntax.",
				Attributes: map[string]schema.Attribute{
					"minute":       schema.StringAttribute{Required: true, Description: "Cron minute field."},
					"hour":         schema.StringAttribute{Required: true, Description: "Cron hour field."},
					"day_of_month": schema.StringAttribute{Required: true, Description: "Cron day-of-month field."},
					"month":        schema.StringAttribute{Required: true, Description: "Cron month field."},
					"day_of_week":  schema.StringAttribute{Required: true, Description: "Cron day-of-week field."},
				},
			},
			"search_for": schema.ListNestedAttribute{
				Optional:    true,
				Computed:    true,
				Default:     listdefault.StaticValue(types.ListValueMust(searchForObjectType, []attr.Value{})),
				Description: "Conditions that select the cards a recurring (scheduler) automation acts on. The list is managed in full: the configured conditions are authoritative, and an empty list (or omitting the block) clears them on the server. Order is preserved.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"field":     schema.StringAttribute{Required: true, Description: "The id of the field used as a filter."},
						"id":        schema.StringAttribute{Required: true, Description: "Caller-assigned identifier for the condition."},
						"operation": schema.StringAttribute{Required: true, Description: "The filter operation. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference)."},
						"value":     schema.StringAttribute{Optional: true, Description: "The value or field id to compare against."},
					},
				},
			},
			"response_schema": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Description: "JSON response schema for the automation, as a JSON string. Compared semantically, so formatting and key order do not cause a diff.",
			},
		},
	}
}

func (r *AutomationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AutomationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutation := "mutation CreateAutomation_tf($input:CreateAutomationInput!){ createAutomation(input:$input){ automation{ id name action_id event_id active } error_details{ object_name object_key messages } } }"
	input := map[string]any{
		"name":           data.Name.ValueString(),
		"action_id":      data.ActionId.ValueString(),
		"event_id":       data.EventId.ValueString(),
		"event_repo_id":  data.EventRepoId.ValueString(),
		"action_repo_id": data.ActionRepoId.ValueString(),
		"active":         data.Active.ValueBool(),
	}
	if !addAutomationOptionalInputs(input, &data, &resp.Diagnostics) {
		return
	}
	vars := map[string]any{"input": input}

	var out struct {
		CreateAutomation struct {
			Automation *struct {
				Id       string `json:"id"`
				Name     string `json:"name"`
				ActionId string `json:"action_id"`
				EventId  string `json:"event_id"`
				Active   bool   `json:"active"`
			} `json:"automation"`
			ErrorDetails []automationErrorDetail `json:"error_details"`
		} `json:"createAutomation"`
	}
	err := r.api.DoGraphQL(ctx, mutation, vars, &out)
	if detail := automationError(out.CreateAutomation.Automation != nil, out.CreateAutomation.ErrorDetails, err); detail != "" {
		resp.Diagnostics.AddError("create automation failed", detail)
		return
	}
	data.Id = types.StringValue(out.CreateAutomation.Automation.Id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AutomationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AutomationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	query := "query GetAutomation_tf($id:ID!){ automation(id:$id){ " + automationSelection + " } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		Automation *automationData `json:"automation"`
	}
	if err := r.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read automation failed", err.Error())
		return
	}
	if out.Automation == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	data.apply(out.Automation)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AutomationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutation := "mutation UpdateAutomation_tf($input:UpdateAutomationInput!){ updateAutomation(input:$input){ automation{ id } error_details{ object_name object_key messages } } }"
	input := map[string]any{
		"id": data.Id.ValueString(),
	}
	if !data.Name.IsNull() {
		input["name"] = data.Name.ValueString()
	}
	if !data.EventId.IsNull() {
		input["event_id"] = data.EventId.ValueString()
	}
	if !data.ActionId.IsNull() {
		input["action_id"] = data.ActionId.ValueString()
	}
	if !data.EventRepoId.IsNull() {
		input["event_repo_id"] = data.EventRepoId.ValueString()
	}
	if !data.ActionRepoId.IsNull() {
		input["action_repo_id"] = data.ActionRepoId.ValueString()
	}
	if !data.Active.IsNull() {
		input["active"] = data.Active.ValueBool()
	}
	if !addAutomationOptionalInputs(input, &data, &resp.Diagnostics) {
		return
	}

	vars := map[string]any{"input": input}
	var out struct {
		UpdateAutomation struct {
			Automation *struct {
				Id string `json:"id"`
			} `json:"automation"`
			ErrorDetails []automationErrorDetail `json:"error_details"`
		} `json:"updateAutomation"`
	}
	err := r.api.DoGraphQL(ctx, mutation, vars, &out)
	if detail := automationError(out.UpdateAutomation.Automation != nil, out.UpdateAutomation.ErrorDetails, err); detail != "" {
		resp.Diagnostics.AddError("update automation failed", detail)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AutomationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AutomationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation DeleteAutomation_tf($id:ID!){ deleteAutomation(input:{id:$id}){ success } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		DeleteAutomation struct {
			Success bool `json:"success"`
		} `json:"deleteAutomation"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("delete automation failed", err.Error())
		return
	}
}

func (r *AutomationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
