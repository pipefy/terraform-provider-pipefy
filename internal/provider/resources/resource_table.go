// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
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
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/tablegql"
)

var _ resource.Resource = &TableResource{}
var _ resource.ResourceWithImportState = &TableResource{}

func NewTableResource() resource.Resource { return &TableResource{} }

type TableResource struct{ api *client.ApiClient }

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
				Description:   "Access level required to view and edit the table's records: " + strings.Join(tablegql.AuthorizationValues, ", ") + ".",
				Validators:    []validator.String{stringvalidator.OneOf(tablegql.AuthorizationValues...)},
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
	api, ok := req.ProviderData.(*client.ApiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *ApiClient, got %T", req.ProviderData))
		return
	}
	r.api = api
}

func (m *TableModel) apply(p tablegql.Payload, onlyUnknown bool) {
	if !onlyUnknown || m.Id.IsUnknown() {
		m.Id = types.StringValue(p.Id)
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

func (m *TableModel) addVars(vars map[string]any) {
	if hasValue(m.Description) {
		vars["description"] = m.Description.ValueString()
	}
	if hasValue(m.Authorization) {
		vars["authorization"] = m.Authorization.ValueString()
	}
	if hasValue(m.Color) {
		vars["color"] = m.Color.ValueString()
	}
	if hasValue(m.Icon) {
		vars["icon"] = m.Icon.ValueString()
	}
}

const createTableMutation = "mutation CreateTable_tf($name:String!,$orgId:ID!,$authorization:TableAuthorization," +
	"$description:String,$color:Colors,$icon:String){ createTable(input:{ name:$name, organization_id:$orgId, " +
	"authorization:$authorization, description:$description, color:$color, icon:$icon }){ table{ " +
	tablegql.Selection + " } } }"

func (r *TableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TableModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vars := map[string]any{
		"name":  data.Name.ValueString(),
		"orgId": data.OrganizationId.ValueString(),
	}
	data.addVars(vars)

	var out struct {
		CreateTable struct {
			Table tablegql.Payload `json:"table"`
		} `json:"createTable"`
	}
	if err := r.api.DoGraphQL(ctx, createTableMutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create table failed", err.Error())
		return
	}
	data.apply(out.CreateTable.Table, true)
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

	query := "query GetTable_tf($id:ID!){ table(id:$id){ " + tablegql.Selection + " organization { id } } }"
	var out struct {
		Table *struct {
			tablegql.Payload
			Organization *struct {
				Id string `json:"id"`
			} `json:"organization"`
		} `json:"table"`
	}
	if err := r.api.DoGraphQL(ctx, query, map[string]any{"id": data.Id.ValueString()}, &out); err != nil {
		resp.Diagnostics.AddError("read table failed", err.Error())
		return
	}
	if out.Table == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	data.apply(out.Table.Payload, false)
	if out.Table.Organization != nil {
		data.OrganizationId = types.StringValue(out.Table.Organization.Id)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

const updateTableMutation = "mutation UpdateTable_tf($id:ID!,$name:String,$authorization:TableAuthorization," +
	"$description:String,$color:Colors,$icon:String){ updateTable(input:{ id:$id, name:$name, " +
	"authorization:$authorization, description:$description, color:$color, icon:$icon }){ table{ " +
	tablegql.Selection + " } } }"

func (r *TableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data TableModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vars := map[string]any{"id": data.Id.ValueString(), "name": data.Name.ValueString()}
	data.addVars(vars)

	var out struct {
		UpdateTable struct {
			Table tablegql.Payload `json:"table"`
		} `json:"updateTable"`
	}
	if err := r.api.DoGraphQL(ctx, updateTableMutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update table failed", err.Error())
		return
	}
	data.apply(out.UpdateTable.Table, true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TableModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation DeleteTable_tf($id:ID!){ deleteTable(input:{id:$id}){ success } }"
	var out struct {
		DeleteTable struct {
			Success bool `json:"success"`
		} `json:"deleteTable"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"id": data.Id.ValueString()}, &out); err != nil {
		resp.Diagnostics.AddError("delete table failed", err.Error())
		return
	}
}

func (r *TableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
