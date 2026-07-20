// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/webhookgql"
)

var _ resource.Resource = &WebhookResource{}
var _ resource.ResourceWithImportState = &WebhookResource{}

func NewWebhookResource() resource.Resource { return &WebhookResource{} }

type WebhookResource struct{ api *client.ApiClient }

type WebhookModel struct {
	Id      types.String         `tfsdk:"id"`
	PipeId  types.String         `tfsdk:"pipe_id"`
	Url     types.String         `tfsdk:"url"`
	Actions types.List           `tfsdk:"actions"`
	Name    types.String         `tfsdk:"name"`
	Headers jsontypes.Normalized `tfsdk:"headers"`
	Filters jsontypes.Normalized `tfsdk:"filters"`
}

func (r *WebhookResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *WebhookResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sends an HTTP request to a URL when the selected events occur on a pipe.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, Description: "The ID of the webhook", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"pipe_id": schema.StringAttribute{Required: true, Description: "The ID of the pipe that the webhook belongs to", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"url": schema.StringAttribute{
				Required:    true,
				Description: "The URL that receives the webhook notifications.",
				Validators:  []validator.String{validators.URL()},
			},
			"actions": schema.ListAttribute{
				ElementType:   types.StringType,
				Required:      true,
				Description:   "The events that trigger the webhook (e.g. card.create, card.move). The supported values are defined by the Pipefy API; see https://developers.pipefy.com/reference for the current list.",
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Name of the webhook"},
			"headers": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				CustomType:  jsontypes.NormalizedType{},
				Description: "Custom HTTP headers sent with the webhook, as a JSON object string (e.g. \"{\\\"Authorization\\\":\\\"Bearer ...\\\"}\"). Being sensitive, it is not read back from the API: the configured value is authoritative and re-sent on every apply, and removing it clears the headers. Changes made outside Terraform are not detected.",
			},
			"filters": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Description: "Filters that restrict when the webhook fires, as a JSON string. Refreshed from the API so drift is detected, and removing it clears the filters. The supported keys and constraints per action are defined by the API; see https://developers.pipefy.com/reference.",
			},
		},
	}
}

func (r *WebhookResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	var actions []string
	resp.Diagnostics.Append(data.Actions.ElementsAs(ctx, &actions, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := map[string]any{
		"pipe_id": data.PipeId.ValueString(),
		"url":     data.Url.ValueString(),
		"name":    data.Name.ValueString(),
		"actions": actions,
	}
	addHeadersInput(input, data.Headers)
	if !addFiltersInput(input, data.Filters, &resp.Diagnostics) {
		return
	}

	mutation := "mutation CreateWebhook_tf($input:CreateWebhookInput!){ createWebhook(input:$input){ webhook{ " + webhookgql.Selection + " } } }"
	vars := map[string]any{"input": input}
	var out struct {
		CreateWebhook struct {
			Webhook webhookgql.Webhook `json:"webhook"`
		} `json:"createWebhook"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create webhook failed", err.Error())
		return
	}
	data.Id = types.StringValue(out.CreateWebhook.Webhook.Id)
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

	query := "query GetPipeWebhooks_tf($pipeId:ID!){ pipe(id:$pipeId){ webhooks{ " + webhookgql.Selection + " } } }"
	vars := map[string]any{"pipeId": data.PipeId.ValueString()}
	var out struct {
		Pipe *struct {
			Webhooks []webhookgql.Webhook `json:"webhooks"`
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

	w, ok := webhookgql.FindByID(out.Pipe.Webhooks, data.Id.ValueString())
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}
	data.Name = types.StringValue(w.Name)
	data.Url = types.StringValue(w.Url)
	actions, d := types.ListValueFrom(ctx, types.StringType, w.Actions)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Actions = actions
	// filters is refreshed so drift is detected; jsontypes.Normalized applies
	// semantic equality, so formatting or key order does not cause a diff.
	// headers is left as-is: it is sensitive and not read back from the API.
	data.Filters = normalizeFilters(w.Filters)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// normalizeFilters maps the API's filters payload to the attribute value. The
// API returns an empty object (or null) when no filter is set, which maps to a
// null attribute so it matches an unset config. A non-empty payload keeps the
// API's raw JSON so large numeric IDs never round-trip through float64;
// jsontypes.Normalized compares it semantically, so formatting does not diff.
func normalizeFilters(raw json.RawMessage) jsontypes.Normalized {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return jsontypes.NewNormalizedNull()
	}
	// Unmarshal only to detect an empty object; the raw bytes, not this decoded
	// value, are what get stored.
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
		if m, ok := v.(map[string]any); ok && len(m) == 0 {
			return jsontypes.NewNormalizedNull()
		}
	}
	return jsontypes.NewNormalizedValue(trimmed)
}

func (r *WebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data WebhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := map[string]any{"id": data.Id.ValueString()}
	if !data.Name.IsNull() {
		input["name"] = data.Name.ValueString()
	}
	if !data.Url.IsNull() {
		input["url"] = data.Url.ValueString()
	}
	if !data.Actions.IsNull() {
		var actions []string
		resp.Diagnostics.Append(data.Actions.ElementsAs(ctx, &actions, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		input["actions"] = actions
	}
	updateHeadersInput(input, data.Headers)
	if !updateFiltersInput(input, data.Filters, &resp.Diagnostics) {
		return
	}

	mutation := "mutation UpdateWebhook_tf($input:UpdateWebhookInput!){ updateWebhook(input:$input){ webhook{ id } } }"
	vars := map[string]any{"input": input}
	var out struct {
		UpdateWebhook struct {
			Webhook struct {
				Id string `json:"id"`
			} `json:"webhook"`
		} `json:"updateWebhook"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update webhook failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data WebhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation DeleteWebhook_tf($id:ID!){ deleteWebhook(input:{ id:$id }){ success } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		DeleteWebhook struct {
			Success bool `json:"success"`
		} `json:"deleteWebhook"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("delete webhook failed", err.Error())
		return
	}
}

func (r *WebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, ok := splitImportID(req.ID)
	if !ok || len(parts) != 2 {
		resp.Diagnostics.AddError("invalid import ID", "expected pipe_id/webhook_id, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pipe_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// addHeadersInput adds headers to the input map when set. The API's headers
// field is the Json scalar, which expects a JSON document encoded as a string
// (e.g. "{\"Authorization\":\"...\"}"), so the raw attribute value is passed
// through unchanged. Validity is enforced at plan time by jsontypes.Normalized.
func addHeadersInput(input map[string]any, value jsontypes.Normalized) {
	if value.IsNull() || value.ValueString() == "" {
		return
	}
	input["headers"] = value.ValueString()
}

// addFiltersInput unmarshals the filters JSON into a value and adds it to the
// input map when set. The API's filters field is the JSON scalar, which expects
// an actual object rather than a string. It reports an error and returns false
// when the value cannot be unmarshaled so the caller can stop.
func addFiltersInput(input map[string]any, value jsontypes.Normalized, diags *diag.Diagnostics) bool {
	if value.IsNull() || value.ValueString() == "" {
		return true
	}
	var v any
	diags.Append(value.Unmarshal(&v)...)
	if diags.HasError() {
		return false
	}
	input["filters"] = v
	return true
}

// updateHeadersInput always sets headers on an update so that removing it from
// config clears the remote value. The API otherwise resets omitted headers to
// an empty object, but sending an explicit value (or null) keeps config
// authoritative. It sends the JSON string when set, or null when unset.
func updateHeadersInput(input map[string]any, value jsontypes.Normalized) {
	if value.IsNull() || value.ValueString() == "" {
		input["headers"] = nil
		return
	}
	input["headers"] = value.ValueString()
}

// updateFiltersInput always sets filters on an update so that removing it from
// config clears the remote value. The API keeps omitted filters, so an explicit
// null is required to clear them. It sends the unmarshaled object when set, or
// null when unset.
func updateFiltersInput(input map[string]any, value jsontypes.Normalized, diags *diag.Diagnostics) bool {
	if value.IsNull() || value.ValueString() == "" {
		input["filters"] = nil
		return true
	}
	var v any
	diags.Append(value.Unmarshal(&v)...)
	if diags.HasError() {
		return false
	}
	input["filters"] = v
	return true
}
