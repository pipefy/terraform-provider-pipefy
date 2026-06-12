// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/locks"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

var _ resource.Resource = &WebhookResource{}
var _ resource.ResourceWithImportState = &WebhookResource{}

func NewWebhookResource() resource.Resource { return &WebhookResource{} }

type WebhookResource struct{ api *client.ApiClient }

type WebhookModel struct {
	Id      types.String `tfsdk:"id"`
	PipeId  types.String `tfsdk:"pipe_id"`
	Name    types.String `tfsdk:"name"`
	Url     types.String `tfsdk:"url"`
	Actions types.Set    `tfsdk:"actions"`
	Headers types.Map    `tfsdk:"headers"`
	Filters types.String `tfsdk:"filters"`
}

func (r *WebhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *WebhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Webhook resource",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, Description: "The ID of the webhook", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"pipe_id": schema.StringAttribute{Required: true, Description: "The ID of the pipe this webhook belongs to", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":    schema.StringAttribute{Required: true, Description: "Name of the webhook"},
			"url": schema.StringAttribute{
				Required:    true,
				Description: "The HTTP or HTTPS URL that Pipefy will POST events to",
				Validators:  []validator.String{validators.WebhookURL()},
			},
			"actions": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Set of event names that trigger this webhook. Valid values: card.create, card.done, card.expired, card.late, card.move, card.overdue, card.deleted, card.field_update",
				Validators:  []validator.Set{validators.WebhookActions()},
			},
			"headers": schema.MapAttribute{
				Optional:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "HTTP headers sent with each webhook delivery (e.g. Authorization). Treated as write-only: not refreshed from the API to avoid perpetual diffs on sensitive values.",
			},
			"filters": schema.StringAttribute{
				Optional:    true,
				Description: `JSON-encoded filter conditions for this webhook. Must be an object where each value is an array of numeric IDs, e.g. {"on_phase_id":[123]}. Only one action may be set when filters are used. Treated as write-only: not refreshed from the API to avoid JSON formatting diffs.`,
				Validators:  []validator.String{validators.WebhookFilters()},
			},
		},
	}
}

func (r *WebhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *WebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data WebhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := locks.LockRepo(data.PipeId.ValueString())
	defer unlock()

	var actions []string
	resp.Diagnostics.Append(data.Actions.ElementsAs(ctx, &actions, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vars := map[string]any{
		"pipeId":  data.PipeId.ValueString(),
		"name":    data.Name.ValueString(),
		"url":     data.Url.ValueString(),
		"actions": actions,
	}
	if !data.Headers.IsNull() && !data.Headers.IsUnknown() {
		var h map[string]string
		resp.Diagnostics.Append(data.Headers.ElementsAs(ctx, &h, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		hJSON, err := json.Marshal(h)
		if err != nil {
			resp.Diagnostics.AddError("failed to encode headers", err.Error())
			return
		}
		vars["headers"] = string(hJSON)
	}
	if !data.Filters.IsNull() && !data.Filters.IsUnknown() {
		var fv any
		if err := json.Unmarshal([]byte(data.Filters.ValueString()), &fv); err != nil {
			resp.Diagnostics.AddError("invalid filters JSON", err.Error())
			return
		}
		vars["filters"] = fv
	}

	mutation := `mutation($pipeId:ID!,$name:String!,$url:String!,$actions:[String!]!,$headers:Json,$filters:JSON){ createWebhook(input:{pipe_id:$pipeId,name:$name,url:$url,actions:$actions,headers:$headers,filters:$filters}){ webhook{ id name url actions } } }`
	var out struct {
		CreateWebhook struct {
			Webhook struct {
				Id      string   `json:"id"`
				Name    string   `json:"name"`
				Url     string   `json:"url"`
				Actions []string `json:"actions"`
			} `json:"webhook"`
		} `json:"createWebhook"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create webhook failed", err.Error())
		return
	}

	w := out.CreateWebhook.Webhook
	data.Id = types.StringValue(w.Id)
	data.Name = types.StringValue(w.Name)
	data.Url = types.StringValue(w.Url)
	actSet, diags := types.SetValueFrom(ctx, types.StringType, w.Actions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Actions = actSet
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data WebhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	query := "query($pipeId:ID!){ pipe(id:$pipeId){ webhooks{ id name url actions } } }"
	vars := map[string]any{"pipeId": data.PipeId.ValueString()}
	var out struct {
		Pipe *struct {
			Webhooks []struct {
				Id      string   `json:"id"`
				Name    string   `json:"name"`
				Url     string   `json:"url"`
				Actions []string `json:"actions"`
			} `json:"webhooks"`
		} `json:"pipe"`
	}
	if err := r.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read webhook failed", err.Error())
		return
	}
	if out.Pipe == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	id := data.Id.ValueString()
	for _, w := range out.Pipe.Webhooks {
		if w.Id == id {
			data.Name = types.StringValue(w.Name)
			data.Url = types.StringValue(w.Url)
			actSet, diags := types.SetValueFrom(ctx, types.StringType, w.Actions)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			data.Actions = actSet
			// headers and filters are write-only: not refreshed from the API
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *WebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data WebhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := locks.LockRepo(data.PipeId.ValueString())
	defer unlock()

	var actions []string
	resp.Diagnostics.Append(data.Actions.ElementsAs(ctx, &actions, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vars := map[string]any{
		"id":      data.Id.ValueString(),
		"name":    data.Name.ValueString(),
		"url":     data.Url.ValueString(),
		"actions": actions,
	}
	if !data.Headers.IsNull() && !data.Headers.IsUnknown() {
		var h map[string]string
		resp.Diagnostics.Append(data.Headers.ElementsAs(ctx, &h, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		hJSON, err := json.Marshal(h)
		if err != nil {
			resp.Diagnostics.AddError("failed to encode headers", err.Error())
			return
		}
		vars["headers"] = string(hJSON)
	}
	if !data.Filters.IsNull() && !data.Filters.IsUnknown() {
		// filters uses GraphQL::Types::JSON (built-in pass-through scalar); the
		// model expects a Hash, so send a parsed object — not a JSON string.
		var fv any
		if err := json.Unmarshal([]byte(data.Filters.ValueString()), &fv); err != nil {
			resp.Diagnostics.AddError("invalid filters JSON", err.Error())
			return
		}
		vars["filters"] = fv
	}

	mutation := `mutation($id:ID!,$name:String,$url:String,$actions:[String],$headers:Json,$filters:JSON){ updateWebhook(input:{id:$id,name:$name,url:$url,actions:$actions,headers:$headers,filters:$filters}){ webhook{ id name url actions } } }`
	var out struct {
		UpdateWebhook struct {
			Webhook struct {
				Id      string   `json:"id"`
				Name    string   `json:"name"`
				Url     string   `json:"url"`
				Actions []string `json:"actions"`
			} `json:"webhook"`
		} `json:"updateWebhook"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update webhook failed", err.Error())
		return
	}

	w := out.UpdateWebhook.Webhook
	data.Name = types.StringValue(w.Name)
	data.Url = types.StringValue(w.Url)
	actSet, diags := types.SetValueFrom(ctx, types.StringType, w.Actions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Actions = actSet
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data WebhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unlock := locks.LockRepo(data.PipeId.ValueString())
	defer unlock()

	mutation := "mutation($id:ID!){ deleteWebhook(input:{id:$id}){ success } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		DeleteWebhook struct {
			Success bool `json:"success"`
		} `json:"deleteWebhook"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("delete webhook failed", err.Error())
	}
}

func (r *WebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"invalid import ID",
			"expected pipe_id/webhook_id, got "+req.ID,
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pipe_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
