// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &PipeResource{}
var _ resource.ResourceWithImportState = &PipeResource{}

func NewPipeResource() resource.Resource { return &PipeResource{} }

type PipeResource struct{ api *ApiClient }

type PipeModel struct {
	Id             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	OrganizationId types.String `tfsdk:"organization_id"`
	Public         types.Bool   `tfsdk:"public"`
}

func (r *PipeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipe"
}

func (r *PipeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Pipe resource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name":            schema.StringAttribute{Required: true},
			"organization_id": schema.StringAttribute{Required: true},
			"public":          schema.BoolAttribute{Optional: true},
		},
	}
}

func (r *PipeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*ApiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *ApiClient, got %T", req.ProviderData))
		return
	}
	r.api = api
}

func (r *PipeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutation := "mutation($name:String!,$orgId:ID!){ createPipe(input:{name:$name, organization_id:$orgId}){ clientMutationId pipe{ id name } } }"
	vars := map[string]interface{}{
		"name":  data.Name.ValueString(),
		"orgId": data.OrganizationId.ValueString(),
	}
	var out struct {
		CreatePipe struct {
			Pipe struct {
				Id   string `json:"id"`
				Name string `json:"name"`
			} `json:"pipe"`
		} `json:"createPipe"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create pipe failed", err.Error())
		return
	}

	pipeId := out.CreatePipe.Pipe.Id
	data.Id = types.StringValue(pipeId)
	// TODO:When creating a pipe it creates 3 phases for it
	// so we delete them.
	// We need to find a better way to do this.
	phasesQuery := "query($id:ID!){ pipe(id:$id){ id phases { id } } }"
	phasesVars := map[string]interface{}{"id": pipeId}
	var phasesOut struct {
		Pipe struct {
			Id     string `json:"id"`
			Phases []struct {
				Id string `json:"id"`
			} `json:"phases"`
		} `json:"pipe"`
	}
	if err := r.api.DoGraphQL(ctx, phasesQuery, phasesVars, &phasesOut); err != nil {
		resp.Diagnostics.AddError("query pipe phases failed", err.Error())
		return
	}

	for _, phase := range phasesOut.Pipe.Phases {
		deleteMutation := "mutation($id:ID!){ deletePhase(input:{id:$id}){ clientMutationId success } }"
		deleteVars := map[string]interface{}{"id": phase.Id}
		var deleteOut struct {
			DeletePhase struct {
				ClientMutationId string `json:"clientMutationId"`
				Success          bool   `json:"success"`
			} `json:"deletePhase"`
		}
		if err := r.api.DoGraphQL(ctx, deleteMutation, deleteVars, &deleteOut); err != nil {
			resp.Diagnostics.AddError("delete phase failed", fmt.Sprintf("failed to delete phase %s: %s", phase.Id, err.Error()))
			return
		}
		if !deleteOut.DeletePhase.Success {
			resp.Diagnostics.AddError("delete phase failed", fmt.Sprintf("failed to delete phase %s: operation returned success=false", phase.Id))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	query := "query($id:ID!){ pipe(id:$id){ id name } }"
	vars := map[string]interface{}{"id": data.Id.ValueString()}
	var out struct {
		Pipe *struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		} `json:"pipe"`
	}
	if err := r.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read pipe failed", err.Error())
		return
	}
	if out.Pipe == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!,$name:String,$public:Boolean){ updatePipe(input:{id:$id, name:$name, public:$public}){ pipe{ id } } }"
	vars := map[string]interface{}{
		"id": data.Id.ValueString(),
	}
	if !data.Name.IsNull() {
		vars["name"] = data.Name.ValueString()
	}
	if !data.Public.IsNull() {
		vars["public"] = data.Public.ValueBool()
	}
	var out struct {
		UpdatePipe struct {
			Pipe struct {
				Id string `json:"id"`
			} `json:"pipe"`
		} `json:"updatePipe"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update pipe failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!){ deletePipe(input:{id:$id}){ success } }"
	vars := map[string]interface{}{"id": data.Id.ValueString()}
	var out struct {
		DeletePipe struct {
			Success bool `json:"success"`
		} `json:"deletePipe"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("delete pipe failed", err.Error())
		return
	}
}

func (r *PipeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
