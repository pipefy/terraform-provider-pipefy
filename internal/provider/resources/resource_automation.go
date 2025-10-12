// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
)

var _ resource.Resource = &AutomationResource{}
var _ resource.ResourceWithImportState = &AutomationResource{}

func NewAutomationResource() resource.Resource { return &AutomationResource{} }

type AutomationResource struct{ api *client.ApiClient }

type AutomationModel struct {
	Id           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	EventId      types.String `tfsdk:"event_id"`
	ActionId     types.String `tfsdk:"action_id"`
	EventRepoId  types.String `tfsdk:"event_repo_id"`
	ActionRepoId types.String `tfsdk:"action_repo_id"`
	EventParams  types.String `tfsdk:"event_params"`
	ActionParams types.String `tfsdk:"action_params"`
	Condition    types.String `tfsdk:"condition"`
	Active       types.Bool   `tfsdk:"active"`
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
			"event_params":  schema.StringAttribute{Optional: true, Description: "The parameters of the event for the automation"},
			"action_params": schema.StringAttribute{Optional: true, Description: "The parameters of the action for the automation"},
			"condition":     schema.StringAttribute{Optional: true, Description: "The condition for the automation to be executed"},
			"active":        schema.BoolAttribute{Optional: true, Description: "Whether the automation is active or not"},
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

	mutation := "mutation($input:CreateAutomationInput!){ createAutomation(input:$input){ automation{ id name action_id event_id active } error_details{ object_name object_key messages } } }"
	input := map[string]any{
		"name":           data.Name.ValueString(),
		"action_id":      data.ActionId.ValueString(),
		"event_id":       data.EventId.ValueString(),
		"event_repo_id":  data.EventRepoId.ValueString(),
		"action_repo_id": data.ActionRepoId.ValueString(),
	}
	if !data.EventParams.IsNull() && data.EventParams.ValueString() != "" {
		var ep any
		// Accept raw JSON string for event_params
		if err := json.Unmarshal([]byte(data.EventParams.ValueString()), &ep); err != nil {
			resp.Diagnostics.AddError("invalid event_params JSON", err.Error())
			return
		}
		input["event_params"] = ep
	}
	if !data.ActionParams.IsNull() && data.ActionParams.ValueString() != "" {
		var ap any
		// Accept raw JSON string for action_params
		if err := json.Unmarshal([]byte(data.ActionParams.ValueString()), &ap); err != nil {
			resp.Diagnostics.AddError("invalid action_params JSON", err.Error())
			return
		}
		input["action_params"] = ap
	}
	if !data.Condition.IsNull() && data.Condition.ValueString() != "" {
		var cond any
		if err := json.Unmarshal([]byte(data.Condition.ValueString()), &cond); err != nil {
			resp.Diagnostics.AddError("invalid condition JSON", err.Error())
			return
		}
		input["condition"] = cond
	}
	if !data.Active.IsNull() {
		input["active"] = data.Active.ValueBool()
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
			ErrorDetails any `json:"error_details"`
		} `json:"createAutomation"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create automation failed", err.Error())
		return
	}
	if out.CreateAutomation.Automation == nil {
		resp.Diagnostics.AddError("create automation failed", "no automation returned; check error_details in API")
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

	query := "query($id:ID!){ automation(id:$id){ id name action_id event_id active event_repo{ id } action_repo_v2{ ... on Pipe{ id } ... on Table{ id } } } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		Automation *struct {
			Id        string `json:"id"`
			Name      string `json:"name"`
			ActionId  string `json:"action_id"`
			EventId   string `json:"event_id"`
			Active    bool   `json:"active"`
			EventRepo *struct {
				Id string `json:"id"`
			} `json:"event_repo"`
			ActionRepoV2 *struct {
				Id string `json:"id"`
			} `json:"action_repo_v2"`
		} `json:"automation"`
	}
	if err := r.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read automation failed", err.Error())
		return
	}
	if out.Automation == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AutomationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutation := "mutation($input:UpdateAutomationInput!){ updateAutomation(input:$input){ automation{ id } error_details{ object_name object_key messages } } }"
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
	if !data.EventParams.IsNull() && data.EventParams.ValueString() != "" {
		var ep any
		if err := json.Unmarshal([]byte(data.EventParams.ValueString()), &ep); err != nil {
			resp.Diagnostics.AddError("invalid event_params JSON", err.Error())
			return
		}
		input["event_params"] = ep
	}
	if !data.ActionParams.IsNull() && data.ActionParams.ValueString() != "" {
		var ap any
		if err := json.Unmarshal([]byte(data.ActionParams.ValueString()), &ap); err != nil {
			resp.Diagnostics.AddError("invalid action_params JSON", err.Error())
			return
		}
		input["action_params"] = ap
	}
	if !data.Condition.IsNull() && data.Condition.ValueString() != "" {
		var cond any
		if err := json.Unmarshal([]byte(data.Condition.ValueString()), &cond); err != nil {
			resp.Diagnostics.AddError("invalid condition JSON", err.Error())
			return
		}
		input["condition"] = cond
	}
	if !data.Active.IsNull() {
		input["active"] = data.Active.ValueBool()
	}

	vars := map[string]any{"input": input}
	var out struct {
		UpdateAutomation struct {
			Automation *struct {
				Id string `json:"id"`
			} `json:"automation"`
			ErrorDetails any `json:"error_details"`
		} `json:"updateAutomation"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update automation failed", err.Error())
		return
	}
	if out.UpdateAutomation.Automation == nil {
		resp.Diagnostics.AddError("update automation failed", "no automation returned; check error_details in API")
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
	mutation := "mutation($id:ID!){ deleteAutomation(input:{id:$id}){ success } }"
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
