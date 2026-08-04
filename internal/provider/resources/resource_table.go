// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
)

var _ resource.Resource = &TableResource{}
var _ resource.ResourceWithImportState = &TableResource{}

func NewTableResource() resource.Resource { return &TableResource{} }

type TableResource struct{ api *pipefy.Client }

type TableModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Authorization  types.String `tfsdk:"authorization"`
	Color          types.String `tfsdk:"color"`
	Icon           types.String `tfsdk:"icon"`
}

func (r *TableResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_table"
}

func (r *TableResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Pipefy's table-wise information storage system",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The ID of the table",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"organization_id": schema.StringAttribute{
				Required:      true,
				Description:   "The ID of the organization that the table belongs to",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Name of the table"},
			"description": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Description of the table",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"authorization": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Access level required to view and edit the table's records: " + strings.Join(pipefy.AuthorizationValues, ", ") + ".",
				Validators:    []validator.String{stringvalidator.OneOf(pipefy.AuthorizationValues...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"color": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Table color. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference) and the GraphiQL explorer (https://app.pipefy.com/graphiql) for in-depth definitions.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"icon": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Named table icon. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference) and the GraphiQL explorer (https://app.pipefy.com/graphiql) for in-depth definitions.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *TableResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (m *TableModel) apply(p pipefy.Table, onlyUnknown bool) {
	if !onlyUnknown || m.Id.IsUnknown() {
		m.Id = types.StringValue(p.ID)
	}
	if !onlyUnknown || m.Name.IsUnknown() {
		m.Name = types.StringValue(p.Name)
	}
	if !onlyUnknown || m.Description.IsUnknown() {
		m.Description = types.StringPointerValue(p.Description)
	}
	if !onlyUnknown || m.Authorization.IsUnknown() {
		m.Authorization = types.StringPointerValue(p.Authorization)
	}
	if !onlyUnknown || m.Color.IsUnknown() {
		m.Color = types.StringPointerValue(p.Color)
	}
	if !onlyUnknown || m.Icon.IsUnknown() {
		m.Icon = types.StringPointerValue(p.Icon)
	}
}

func (m *TableModel) writes() pipefy.TableWrites {
	return pipefy.TableWrites{
		Description:   optionalString(m.Description),
		Authorization: optionalString(m.Authorization),
		Color:         optionalString(m.Color),
		Icon:          optionalString(m.Icon),
	}
}

func (r *TableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TableModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	table, err := r.api.Tables.Create(ctx, pipefy.CreateTableInput{
		Name:           data.Name.ValueString(),
		OrganizationID: data.OrganizationId.ValueString(),
		TableWrites:    data.writes(),
	})
	if err != nil {
		resp.Diagnostics.AddError("create table failed", err.Error())
		return
	}
	data.apply(table, true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TableModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	table, err := r.api.Tables.Get(ctx, data.Id.ValueString())
	if errors.Is(err, pipefy.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("read table failed", err.Error())
		return
	}
	data.apply(table, false)
	if table.OrganizationID != "" {
		data.OrganizationId = types.StringValue(table.OrganizationID)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data TableModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	table, err := r.api.Tables.Update(ctx, pipefy.UpdateTableInput{
		ID:          data.Id.ValueString(),
		Name:        data.Name.ValueString(),
		TableWrites: data.writes(),
	})
	if err != nil {
		resp.Diagnostics.AddError("update table failed", err.Error())
		return
	}
	data.apply(table, true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TableModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.Tables.Delete(ctx, data.Id.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete table failed", err.Error())
		return
	}
}

func (r *TableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
