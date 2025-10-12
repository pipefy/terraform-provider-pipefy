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
)

var _ datasource.DataSource = &PipeDataSource{}

func NewPipeDataSource() datasource.DataSource { return &PipeDataSource{} }

type PipeDataSource struct{ api *client.ApiClient }

type PipeDataSourceModel struct {
	Id             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	OrganizationId types.String `tfsdk:"organization_id"`
	Public         types.Bool   `tfsdk:"public"`
}

func (d *PipeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipe"
}

func (d *PipeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Pipe data source",
		Attributes: map[string]dsschema.Attribute{
			"id":              dsschema.StringAttribute{Required: true, Description: "The ID of the pipe"},
			"name":            dsschema.StringAttribute{Computed: true, Description: "Name of the pipe"},
			"organization_id": dsschema.StringAttribute{Computed: true, Description: "The ID of the organization that the pipe belongs to"},
			"public":          dsschema.BoolAttribute{Computed: true, Description: "Whether the pipe is public or not"},
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

	query := "query($id:ID!){ pipe(id:$id){ id name } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		Pipe *struct {
			Id           string `json:"id"`
			Name         string `json:"name"`
			Public       bool   `json:"public"`
			Organization *struct {
				Id string `json:"id"`
			} `json:"organization"`
		} `json:"pipe"`
	}
	if err := d.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read pipe failed", err.Error())
		return
	}
	if out.Pipe == nil {
		resp.Diagnostics.AddError("pipe not found", fmt.Sprintf("pipe with id %s not found", data.Id.ValueString()))
		return
	}

	data.Name = types.StringValue(out.Pipe.Name)
	if out.Pipe.Organization != nil {
		data.OrganizationId = types.StringValue(out.Pipe.Organization.Id)
	}
	data.Public = types.BoolValue(out.Pipe.Public)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
