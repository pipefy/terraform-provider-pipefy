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

var _ datasource.DataSource = &PhaseDataSource{}

func NewPhaseDataSource() datasource.DataSource { return &PhaseDataSource{} }

type PhaseDataSource struct{ api *client.ApiClient }

type PhaseDataSourceModel struct {
	Id     types.String `tfsdk:"id"`
	PipeId types.String `tfsdk:"pipe_id"`
	Name   types.String `tfsdk:"name"`
}

func (d *PhaseDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_phase"
}

func (d *PhaseDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Phase data source",
		Attributes: map[string]dsschema.Attribute{
			"id":      dsschema.StringAttribute{Required: true, Description: "The ID of the phase"},
			"pipe_id": dsschema.StringAttribute{Computed: true, Description: "The ID of the pipe that the phase belongs to"},
			"name":    dsschema.StringAttribute{Computed: true, Description: "Name of the phase"},
		},
	}
}

func (d *PhaseDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PhaseDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PhaseDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		resp.Diagnostics.AddError("missing id", "id must be provided")
		return
	}

	query := "query($id:ID!){ phase(id:$id){ id name } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		Phase *struct {
			Id   string `json:"id"`
			Name string `json:"name"`
			Pipe *struct {
				Id string `json:"id"`
			} `json:"pipe"`
		} `json:"phase"`
	}
	if err := d.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read phase failed", err.Error())
		return
	}
	if out.Phase == nil {
		resp.Diagnostics.AddError("phase not found", fmt.Sprintf("phase with id %s not found", data.Id.ValueString()))
		return
	}

	data.Name = types.StringValue(out.Phase.Name)
	if out.Phase.Pipe != nil {
		data.PipeId = types.StringValue(out.Phase.Pipe.Id)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
