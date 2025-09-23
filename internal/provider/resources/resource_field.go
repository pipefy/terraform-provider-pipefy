// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
)

var _ resource.Resource = &FieldResource{}
var _ resource.ResourceWithImportState = &FieldResource{}

func NewFieldResource() resource.Resource { return &FieldResource{} }

type FieldResource struct{ api *client.ApiClient }

type FieldModel struct {
	Id         types.String `tfsdk:"id"`
	InternalId types.String `tfsdk:"internal_id"`
	PhaseId    types.String `tfsdk:"phase_id"`
	Type       types.String `tfsdk:"type"`
	Label      types.String `tfsdk:"label"`
	Required   types.Bool   `tfsdk:"required"`
}

func (r *FieldResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_field"
}

func (r *FieldResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Phase field resource",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"internal_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"phase_id":    schema.StringAttribute{Required: true},
			"type":        schema.StringAttribute{Required: true},
			"label":       schema.StringAttribute{Required: true},
			"required":    schema.BoolAttribute{Optional: true},
		},
	}
}

func (r *FieldResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data FieldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutation := "mutation($phaseId:ID!,$type:ID!,$label:String!,$required:Boolean){ createPhaseField(input:{ phase_id:$phaseId, type:$type, label:$label, required:$required }){ phase_field{ id internal_id label } } }"
	vars := map[string]interface{}{
		"phaseId": data.PhaseId.ValueString(),
		"type":    data.Type.ValueString(),
		"label":   data.Label.ValueString(),
	}
	if !data.Required.IsNull() {
		vars["required"] = data.Required.ValueBool()
	}
	var out struct {
		CreatePhaseField struct {
			PhaseField struct {
				Id         string `json:"id"`
				InternalId string `json:"internal_id"`
				Label      string `json:"label"`
			} `json:"phase_field"`
		} `json:"createPhaseField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create field failed", err.Error())
		return
	}
	data.Id = types.StringValue(out.CreatePhaseField.PhaseField.Id)
	data.InternalId = types.StringValue(out.CreatePhaseField.PhaseField.InternalId)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data FieldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	// Query the phase to get the field information
	query := "query($phaseId:ID!){ phase(id:$phaseId){ fields{ id internal_id label } } }"
	vars := map[string]interface{}{"phaseId": data.PhaseId.ValueString()}
	var out struct {
		Phase *struct {
			Fields []struct {
				Id         string `json:"id"`
				InternalId string `json:"internal_id"`
				Label      string `json:"label"`
			} `json:"fields"`
		} `json:"phase"`
	}
	if err := r.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read field failed", err.Error())
		return
	}
	if out.Phase == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Find the field with matching ID
	var foundField *struct {
		Id         string `json:"id"`
		InternalId string `json:"internal_id"`
		Label      string `json:"label"`
	}
	for _, field := range out.Phase.Fields {
		if field.Id == data.Id.ValueString() {
			foundField = &field
			break
		}
	}

	if foundField == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	data.InternalId = types.StringValue(foundField.InternalId)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FieldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data FieldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!,$label:String!,$required:Boolean){ updatePhaseField(input:{ id:$id, label:$label, required:$required }){ phase_field{ id internal_id } } }"
	vars := map[string]interface{}{"id": data.Id.ValueString()}
	if !data.Label.IsNull() {
		vars["label"] = data.Label.ValueString()
	}
	if !data.Required.IsNull() {
		vars["required"] = data.Required.ValueBool()
	}
	var out struct {
		UpdatePhaseField struct {
			PhaseField struct {
				Id         string `json:"id"`
				InternalId string `json:"internal_id"`
			} `json:"phase_field"`
		} `json:"updatePhaseField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update field failed", err.Error())
		return
	}
	data.InternalId = types.StringValue(out.UpdatePhaseField.PhaseField.InternalId)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data FieldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fetch repo_id from the phase
	phaseQuery := "query($id:ID!){ phase(id:$id){ repo_id } }"
	phaseVars := map[string]interface{}{"id": data.PhaseId.ValueString()}
	var phaseOut struct {
		Phase *struct {
			RepoId int `json:"repo_id"`
		} `json:"phase"`
	}
	if err := r.api.DoGraphQL(ctx, phaseQuery, phaseVars, &phaseOut); err != nil {
		resp.Diagnostics.AddError("delete field failed", fmt.Sprintf("failed to fetch phase repo_id: %s", err.Error()))
		return
	}
	if phaseOut.Phase == nil {
		resp.Diagnostics.AddError("delete field failed", "could not resolve phase from phase query")
		return
	}
	repoIDStr := strconv.FormatInt(int64(phaseOut.Phase.RepoId), 10)
	if repoIDStr == "0" {
		resp.Diagnostics.AddError("delete field failed", "could not resolve valid phase repo_id from phase query")
		return
	}

	// Fetch pipe uuid with repo_id
	pipeQuery := "query($id:ID!){ pipe(id:$id){ uuid } }"
	pipeVars := map[string]interface{}{"id": repoIDStr}
	var pipeOut struct {
		Pipe *struct {
			Uuid string `json:"uuid"`
		} `json:"pipe"`
	}
	if err := r.api.DoGraphQL(ctx, pipeQuery, pipeVars, &pipeOut); err != nil {
		resp.Diagnostics.AddError("delete field failed", fmt.Sprintf("failed to fetch pipe uuid: %s", err.Error()))
		return
	}
	if pipeOut.Pipe == nil || pipeOut.Pipe.Uuid == "" {
		resp.Diagnostics.AddError("delete field failed", "could not resolve pipe uuid from pipe query")
		return
	}

	mutation := "mutation($id:ID!,$pipeUuid:ID!){ deletePhaseField(input:{ id:$id, pipeUuid:$pipeUuid }){ success } }"
	vars := map[string]interface{}{"id": data.Id.ValueString(), "pipeUuid": pipeOut.Pipe.Uuid}
	var out struct {
		DeletePhaseField struct {
			Success bool `json:"success"`
		} `json:"deletePhaseField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("delete field failed", err.Error())
		return
	}
}

func (r *FieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
