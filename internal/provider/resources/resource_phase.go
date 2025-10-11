// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
)

var _ resource.Resource = &PhaseResource{}
var _ resource.ResourceWithImportState = &PhaseResource{}

func NewPhaseResource() resource.Resource { return &PhaseResource{} }

type PhaseResource struct{ api *client.ApiClient }

type PhaseModel struct {
	Id     types.String `tfsdk:"id"`
	PipeId types.String `tfsdk:"pipe_id"`
	Name   types.String `tfsdk:"name"`
}

func (r *PhaseResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_phase"
}

func (r *PhaseResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Phase resource",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"pipe_id": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":    schema.StringAttribute{Required: true},
		},
	}
}

func (r *PhaseResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PhaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PhaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutation := "mutation($pipeId:ID!,$name:String!){ createPhase(input:{ pipe_id: $pipeId, name: $name }){ phase{ id name } } }"
	vars := map[string]any{"pipeId": data.PipeId.ValueString(), "name": data.Name.ValueString()}
	var out struct {
		CreatePhase struct {
			Phase struct {
				Id   string `json:"id"`
				Name string `json:"name"`
			} `json:"phase"`
		} `json:"createPhase"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create phase failed", err.Error())
		return
	}
	data.Id = types.StringValue(out.CreatePhase.Phase.Id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PhaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PhaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	query := "query($id:ID!){ phase(id:$id){ id name } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		Phase *struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		} `json:"phase"`
	}
	if err := r.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read phase failed", err.Error())
		return
	}
	if out.Phase == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PhaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PhaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!,$name:String!){ updatePhase(input:{ id:$id, name:$name }){ phase{ id } } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	if !data.Name.IsNull() {
		vars["name"] = data.Name.ValueString()
	}
	var out struct {
		UpdatePhase struct {
			Phase struct {
				Id string `json:"id"`
			} `json:"phase"`
		} `json:"updatePhase"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update phase failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PhaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PhaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!){ deletePhase(input:{ id:$id }){ success } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		DeletePhase struct {
			Success bool `json:"success"`
		} `json:"deletePhase"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("delete phase failed", err.Error())
		return
	}
}

func (r *PhaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
