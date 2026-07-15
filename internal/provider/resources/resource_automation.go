// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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
	EventParams        types.String                     `tfsdk:"event_params"`
	ActionParams       types.String                     `tfsdk:"action_params"`
	Condition          types.String                     `tfsdk:"condition"`
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
	"searchFor{ field id operation value } responseSchema"

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

func automationNormalizeJSON(raw json.RawMessage) jsontypes.Normalized {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(trimmed)
}

// apply refreshes the round-trippable attributes from a fetched automation.
// search_for is managed in full, so it always maps to a list (empty, not null,
// when the automation has no conditions). event_params, action_params, and
// condition are left untouched: their read types differ from the write inputs
// and do not round-trip.
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
}

// addAutomationOptionalInputs adds the optional inputs shared by Create and
// Update. The JSON-string params are sent as decoded values; scheduler_cron and
// search_for use the API's camelCase field keys. search_for is always sent (as
// an empty list when there are no conditions) so it is managed in full. It
// returns false after recording a diagnostic when a JSON string cannot be
// parsed.
func addAutomationOptionalInputs(input map[string]any, data *AutomationModel, diags *diag.Diagnostics) bool {
	jsonParams := []struct {
		key   string
		value types.String
	}{
		{"event_params", data.EventParams},
		{"action_params", data.ActionParams},
		{"condition", data.Condition},
	}
	for _, p := range jsonParams {
		if p.value.IsNull() || p.value.ValueString() == "" {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(p.value.ValueString()), &v); err != nil {
			diags.AddError("invalid "+p.key+" JSON", err.Error())
			return false
		}
		input[p.key] = v
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
			// JSON strings for complex structures to avoid over-modeling in Terraform schema
			"event_params":  schema.StringAttribute{Optional: true, Description: "The parameters of the event for the automation, as a JSON string. Not read back from the API, so drift is not detected."},
			"action_params": schema.StringAttribute{Optional: true, Description: "The parameters of the action for the automation, as a JSON string. Not read back from the API, so drift is not detected."},
			"condition":     schema.StringAttribute{Optional: true, Description: "The condition for the automation to be executed, as a JSON string. Not read back from the API, so drift is not detected."},
			"active":        schema.BoolAttribute{Required: true, Description: "Whether the automation is active."},
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
