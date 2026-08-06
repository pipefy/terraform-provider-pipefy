// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

var _ resource.Resource = &LabelResource{}
var _ resource.ResourceWithImportState = &LabelResource{}

func NewLabelResource() resource.Resource { return &LabelResource{} }

type LabelResource struct{ api *pipefy.Client }

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
	api, ok := req.ProviderData.(*pipefy.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *pipefy.Client, got %T", req.ProviderData))
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

	label, err := r.api.Labels.Create(ctx, pipefy.CreateLabelInput{
		PipeID: data.PipeId.ValueString(),
		Name:   data.Name.ValueString(),
		Color:  data.Color.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("create label failed", err.Error())
		return
	}
	data.Id = types.StringValue(label.ID)
	data.Name = types.StringValue(label.Name)
	data.Color = types.StringValue(label.Color)
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

	label, err := r.api.Labels.Get(ctx, data.PipeId.ValueString(), data.Id.ValueString())
	if errors.Is(err, pipefy.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("read label failed", err.Error())
		return
	}
	data.Name = types.StringValue(label.Name)
	data.Color = types.StringValue(label.Color)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LabelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data LabelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	label, err := r.api.Labels.Update(ctx, pipefy.UpdateLabelInput{
		ID:    data.Id.ValueString(),
		Name:  data.Name.ValueString(),
		Color: data.Color.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("update label failed", err.Error())
		return
	}
	data.Name = types.StringValue(label.Name)
	data.Color = types.StringValue(label.Color)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LabelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data LabelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.Labels.Delete(ctx, data.Id.ValueString()); err != nil {
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
