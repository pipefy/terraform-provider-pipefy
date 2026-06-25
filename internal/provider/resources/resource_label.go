// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
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
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/labelgql"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

var _ resource.Resource = &LabelResource{}
var _ resource.ResourceWithImportState = &LabelResource{}

func NewLabelResource() resource.Resource { return &LabelResource{} }

type LabelResource struct{ api *client.ApiClient }

type LabelModel struct {
	Id     types.String `tfsdk:"id"`
	PipeId types.String `tfsdk:"pipe_id"`
	Name   types.String `tfsdk:"name"`
	Color  types.String `tfsdk:"color"`
}

func (r *LabelResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_label"
}

func (r *LabelResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Label resource",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, Description: "The ID of the label", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"pipe_id": schema.StringAttribute{Required: true, Description: "The ID of the pipe that the label belongs to", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":    schema.StringAttribute{Required: true, Description: "Name of the label"},
			"color": schema.StringAttribute{
				Required:    true,
				Description: "Color of the label as a hex code (e.g. #FF0000 or #FA0)",
				Validators:  []validator.String{validators.HexColor()},
			},
		},
	}
}

func (r *LabelResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *LabelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data LabelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutation := "mutation($pipeId:ID!,$name:String!,$color:String!){ createLabel(input:{ pipe_id:$pipeId, name:$name, color:$color }){ label{ " + labelgql.Selection + " } } }"
	vars := map[string]any{
		"pipeId": data.PipeId.ValueString(),
		"name":   data.Name.ValueString(),
		"color":  data.Color.ValueString(),
	}
	var out struct {
		CreateLabel struct {
			Label labelgql.Label `json:"label"`
		} `json:"createLabel"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create label failed", err.Error())
		return
	}
	data.Id = types.StringValue(out.CreateLabel.Label.Id)
	data.Name = types.StringValue(out.CreateLabel.Label.Name)
	data.Color = types.StringValue(out.CreateLabel.Label.Color)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LabelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data LabelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	query := "query($pipeId:ID!){ pipe(id:$pipeId){ labels{ " + labelgql.Selection + " } } }"
	vars := map[string]any{"pipeId": data.PipeId.ValueString()}
	var out struct {
		Pipe *struct {
			Labels []labelgql.Label `json:"labels"`
		} `json:"pipe"`
	}
	if err := r.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read label failed", err.Error())
		return
	}
	if out.Pipe == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	l, ok := labelgql.FindByID(out.Pipe.Labels, data.Id.ValueString())
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}
	data.Name = types.StringValue(l.Name)
	data.Color = types.StringValue(l.Color)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LabelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data LabelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutation := "mutation($id:ID!,$name:String!,$color:String!){ updateLabel(input:{ id:$id, name:$name, color:$color }){ label{ " + labelgql.Selection + " } } }"
	vars := map[string]any{
		"id":    data.Id.ValueString(),
		"name":  data.Name.ValueString(),
		"color": data.Color.ValueString(),
	}
	var out struct {
		UpdateLabel struct {
			Label labelgql.Label `json:"label"`
		} `json:"updateLabel"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update label failed", err.Error())
		return
	}
	data.Name = types.StringValue(out.UpdateLabel.Label.Name)
	data.Color = types.StringValue(out.UpdateLabel.Label.Color)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LabelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data LabelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!){ deleteLabel(input:{ id:$id }){ success } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		DeleteLabel struct {
			Success bool `json:"success"`
		} `json:"deleteLabel"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("delete label failed", err.Error())
		return
	}
}

func (r *LabelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"invalid import ID",
			"expected pipe_id/label_id, got "+req.ID,
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pipe_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
