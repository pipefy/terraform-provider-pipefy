// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/pipegql"
)

var _ datasource.DataSource = &PipeDataSource{}

func NewPipeDataSource() datasource.DataSource { return &PipeDataSource{} }

type PipeDataSource struct{ api *client.ApiClient }

type pipeDSPreferencesModel struct {
	InboxEmailEnabled types.Bool `tfsdk:"inbox_email_enabled"`
	MainTabViews      types.List `tfsdk:"main_tab_views"`
}

type pipeDSSLAModel struct {
	Time types.Int64  `tfsdk:"time"`
	Unit types.String `tfsdk:"unit"`
}

type PipeDataSourceModel struct {
	Id                        types.String            `tfsdk:"id"`
	Name                      types.String            `tfsdk:"name"`
	OrganizationId            types.String            `tfsdk:"organization_id"`
	Public                    types.Bool              `tfsdk:"public"`
	Icon                      types.String            `tfsdk:"icon"`
	Color                     types.String            `tfsdk:"color"`
	OnlyAdminCanRemoveCards   types.Bool              `tfsdk:"only_admin_can_remove_cards"`
	OnlyAssigneesCanEditCards types.Bool              `tfsdk:"only_assignees_can_edit_cards"`
	Preferences               *pipeDSPreferencesModel `tfsdk:"preferences"`
	SLA                       *pipeDSSLAModel         `tfsdk:"sla"`
}

func (d *PipeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipe"
}

func (d *PipeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Pipe data source",
		Attributes: map[string]dsschema.Attribute{
			"id":                            dsschema.StringAttribute{Required: true, Description: "The ID of the pipe"},
			"name":                          dsschema.StringAttribute{Computed: true, Description: "Name of the pipe"},
			"organization_id":               dsschema.StringAttribute{Computed: true, Description: "The ID of the organization that the pipe belongs to"},
			"public":                        dsschema.BoolAttribute{Computed: true, Description: "Whether the pipe is public"},
			"icon":                          dsschema.StringAttribute{Computed: true, Description: "Named pipe icon"},
			"color":                         dsschema.StringAttribute{Computed: true, Description: "Pipe color"},
			"only_admin_can_remove_cards":   dsschema.BoolAttribute{Computed: true, Description: "Whether only admins can delete cards"},
			"only_assignees_can_edit_cards": dsschema.BoolAttribute{Computed: true, Description: "Whether only card assignees can edit a card"},
			"preferences": dsschema.SingleNestedAttribute{
				Computed:    true,
				Description: "Pipe preferences",
				Attributes: map[string]dsschema.Attribute{
					"inbox_email_enabled": dsschema.BoolAttribute{Computed: true, Description: "Whether the email inbox is enabled"},
					"main_tab_views":      dsschema.ListAttribute{ElementType: types.StringType, Computed: true, Description: "Card views to show"},
				},
			},
			"sla": dsschema.SingleNestedAttribute{
				Computed:    true,
				Description: "Card SLA",
				Attributes: map[string]dsschema.Attribute{
					"time": dsschema.Int64Attribute{Computed: true, Description: "Count of units"},
					"unit": dsschema.StringAttribute{Computed: true, Description: "SLA unit: minutes, hours, or days"},
				},
			},
		},
	}
}

func (d *PipeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*client.ApiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *ApiClient, got %T", req.ProviderData))
		return
	}
	d.api = api
}

func (d *PipeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PipeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		resp.Diagnostics.AddError("missing id", "id must be provided")
		return
	}

	query := "query($id:ID!){ pipe(id:$id){ " + pipegql.Selection + " organization { id } } }"
	var out struct {
		Pipe *struct {
			pipegql.Payload
			Organization *struct {
				Id string `json:"id"`
			} `json:"organization"`
		} `json:"pipe"`
	}
	if err := d.api.DoGraphQL(ctx, query, map[string]any{"id": data.Id.ValueString()}, &out); err != nil {
		resp.Diagnostics.AddError("read pipe failed", err.Error())
		return
	}
	if out.Pipe == nil {
		resp.Diagnostics.AddError("pipe not found", fmt.Sprintf("pipe with id %s not found", data.Id.ValueString()))
		return
	}

	p := out.Pipe.Payload
	data.Name = types.StringValue(p.Name)
	data.Public = types.BoolPointerValue(p.Public)
	data.Icon = types.StringPointerValue(p.Icon)
	data.Color = types.StringPointerValue(p.Color)
	data.OnlyAdminCanRemoveCards = types.BoolPointerValue(p.OnlyAdminCanRemoveCards)
	data.OnlyAssigneesCanEditCards = types.BoolPointerValue(p.OnlyAssigneesCanEditCards)
	if out.Pipe.Organization != nil {
		data.OrganizationId = types.StringValue(out.Pipe.Organization.Id)
	}
	if p.Preferences != nil {
		views, diags := types.ListValueFrom(ctx, types.StringType, p.Preferences.MainTabViews)
		resp.Diagnostics.Append(diags...)
		data.Preferences = &pipeDSPreferencesModel{
			InboxEmailEnabled: types.BoolPointerValue(p.Preferences.InboxEmailEnabled),
			MainTabViews:      views,
		}
	}
	if count, unit, ok := p.SLA(); ok {
		data.SLA = &pipeDSSLAModel{Time: types.Int64Value(count), Unit: types.StringValue(unit)}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
