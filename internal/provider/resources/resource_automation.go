// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
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

var _ resource.Resource = &AutomationResource{}
var _ resource.ResourceWithImportState = &AutomationResource{}
var _ resource.ResourceWithValidateConfig = &AutomationResource{}

func NewAutomationResource() resource.Resource { return &AutomationResource{} }

type AutomationResource struct{ api *pipefy.Client }

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

type AutomationModel struct {
	Id                 types.String                     `tfsdk:"id"`
	Name               types.String                     `tfsdk:"name"`
	EventId            types.String                     `tfsdk:"event_id"`
	ActionId           types.String                     `tfsdk:"action_id"`
	EventRepoId        types.String                     `tfsdk:"event_repo_id"`
	ActionRepoId       types.String                     `tfsdk:"action_repo_id"`
	EventParams        *automationEventParamsModel      `tfsdk:"event_params"`
	ActionParams       types.String                     `tfsdk:"action_params"`
	Condition          *conditionschema.Condition       `tfsdk:"condition"`
	Active             types.Bool                       `tfsdk:"active"`
	SchedulerFrequency types.String                     `tfsdk:"scheduler_frequency"`
	SchedulerCron      *automationCronModel             `tfsdk:"scheduler_cron"`
	SearchFor          []automationSearchConditionModel `tfsdk:"search_for"`
	ResponseSchema     jsontypes.Normalized             `tfsdk:"response_schema"`
}

func formatAutomationErrorDetails(details []pipefy.ErrorDetail) string {
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

// automationError renders a failed automation mutation. The order matches what
// the API can return at once: printable error_details win, then the transport
// error that may have accompanied them, then the no-information fallback.
func automationError(err error) string {
	var validationErr *pipefy.ValidationError
	if errors.As(err, &validationErr) {
		if detail := formatAutomationErrorDetails(validationErr.Details); detail != "" {
			return detail
		}
		if wrapped := errors.Unwrap(validationErr); wrapped != nil {
			return wrapped.Error()
		}
		return pipefy.ErrNoAutomation.Error()
	}
	return err.Error()
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
func automationCronToModel(c *pipefy.AutomationCron) *automationCronModel {
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

// automationSearchForToModel maps the API's search conditions back to state. An
// empty (or absent) list maps to no value so it matches an unset block, matching
// how a condition with no expressions settles.
func automationSearchForToModel(cs []pipefy.AutomationSearchCondition) []automationSearchConditionModel {
	if len(cs) == 0 {
		return nil
	}
	conds := make([]automationSearchConditionModel, len(cs))
	for i, c := range cs {
		conds[i] = automationSearchConditionModel{
			Field:     types.StringValue(c.Field),
			Id:        types.StringValue(c.ID),
			Operation: types.StringValue(c.Operation),
			Value:     automationOptionalString(c.Value),
		}
	}
	return conds
}

func automationNormalizeJSON(raw json.RawMessage) jsontypes.Normalized {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(trimmed)
}

// apply refreshes the round-trippable attributes from a fetched automation.
// search_for and condition are managed in full and settle the same way: each
// maps to no value (null) when the automation carries none, so an unset config
// settles cleanly. action_params and event_params are not read back, so drift
// in them is not detected.
func (m *AutomationModel) apply(a *pipefy.Automation, diags *diag.Diagnostics) {
	m.Id = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	if a.Active != nil {
		m.Active = types.BoolValue(*a.Active)
	}
	m.EventId = types.StringValue(a.EventID)
	m.ActionId = types.StringValue(a.ActionID)
	if a.EventRepo != nil && a.EventRepo.ID != "" {
		m.EventRepoId = types.StringValue(a.EventRepo.ID)
	}
	if a.ActionRepoV2 != nil && a.ActionRepoV2.ID != "" {
		m.ActionRepoId = types.StringValue(a.ActionRepoV2.ID)
	}
	m.SchedulerFrequency = automationOptionalString(a.SchedulerFrequency)
	m.SchedulerCron = automationCronToModel(a.SchedulerCron)
	m.SearchFor = automationSearchForToModel(a.SearchFor)
	m.ResponseSchema = automationNormalizeJSON(a.ResponseSchema)
	cond, err := conditionschema.FromPayload(a.Condition)
	if err != nil {
		diags.AddError("automation condition API inconsistency", err.Error())
		return
	}
	m.Condition = cond
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
		if len(ep) > 0 {
			input["event_params"] = ep
		}
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
	if data.Condition != nil {
		input["condition"] = data.Condition.Input()
	} else {
		input["condition"] = conditionschema.EmptyInput()
	}
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
				Description: "Parameters of the event the automation listens to. Which subfields apply depends on event_id; see the API reference (https://developers.pipefy.com/reference/automation-creation). Write-only: not read back from the API, so drift is not detected and removing the block does not clear it on the server.",
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
			"action_params": schema.StringAttribute{Optional: true, Description: "The parameters of the action for the automation, as a JSON string. Write-only: not read back from the API, so drift is not detected and removing it does not clear it on the server."},
			"condition": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Condition that gates the automation. Managed in full: the configured comparisons are authoritative, and omitting the block clears the condition on the server.",
				Attributes:  conditionschema.Attributes(),
			},
			"active": schema.BoolAttribute{Required: true, Description: "Whether the automation is active."},
			"scheduler_frequency": schema.StringAttribute{
				Optional:    true,
				Validators:  []validator.String{validators.NonBlank()},
				Description: "Frequency for time-based (scheduler) triggers. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference/automation-creation) and the GraphiQL explorer (https://app.pipefy.com/graphiql). Required while event_id is \"scheduler\".",
			},
			"scheduler_cron": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Cron schedule for time-based (scheduler) triggers. Fields use standard crontab syntax. Required while event_id is \"scheduler\".",
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
				Description: "Conditions that select the cards a recurring (scheduler) automation acts on. The list is managed in full: the configured conditions are authoritative, and omitting the block clears them on the server. Order is preserved.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"field":     schema.StringAttribute{Required: true, Description: "The id of the field used as a filter.", Validators: []validator.String{validators.NonBlank()}},
						"id":        schema.StringAttribute{Required: true, Description: "Caller-assigned identifier for the condition.", Validators: []validator.String{validators.NonBlank()}},
						"operation": schema.StringAttribute{Required: true, Description: "The filter operation. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference).", Validators: []validator.String{validators.NonBlank()}},
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

func (r *AutomationResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var eventId, frequency types.String
	var cron types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("event_id"), &eventId)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("scheduler_frequency"), &frequency)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("scheduler_cron"), &cron)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if eventId.IsNull() || eventId.IsUnknown() || eventId.ValueString() != "scheduler" {
		return
	}
	if frequency.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("scheduler_frequency"),
			"Missing scheduler_frequency",
			`scheduler_frequency is required when event_id is "scheduler". The API rejects an automation on that event without a frequency, and it cannot be cleared once set.`,
		)
	}
	if cron.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("scheduler_cron"),
			"Missing scheduler_cron",
			`scheduler_cron is required when event_id is "scheduler". The API rejects an automation on that event without a cron schedule, and it cannot be cleared once set.`,
		)
	}
}

func (r *AutomationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AutomationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

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
	created, err := r.api.Automations.Create(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("create automation failed", automationError(err))
		return
	}
	data.Id = types.StringValue(created.ID)
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

	automation, err := r.api.Automations.Get(ctx, data.Id.ValueString())
	if errors.Is(err, pipefy.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("read automation failed", err.Error())
		return
	}
	data.apply(&automation, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func updateResponseSchemaInput(input map[string]any, value jsontypes.Normalized) {
	if value.IsNull() || value.ValueString() == "" {
		input["responseSchema"] = nil
	}
}

func (r *AutomationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := map[string]any{
		"id":             data.Id.ValueString(),
		"name":           data.Name.ValueString(),
		"event_id":       data.EventId.ValueString(),
		"action_id":      data.ActionId.ValueString(),
		"event_repo_id":  data.EventRepoId.ValueString(),
		"action_repo_id": data.ActionRepoId.ValueString(),
		"active":         data.Active.ValueBool(),
	}
	if !addAutomationOptionalInputs(input, &data, &resp.Diagnostics) {
		return
	}
	updateResponseSchemaInput(input, data.ResponseSchema)

	if err := r.api.Automations.Update(ctx, input); err != nil {
		resp.Diagnostics.AddError("update automation failed", automationError(err))
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
	if err := r.api.Automations.Delete(ctx, data.Id.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete automation failed", err.Error())
		return
	}
}

func (r *AutomationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
