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

var _ resource.Resource = &FieldResource{}
var _ resource.ResourceWithImportState = &FieldResource{}

func NewFieldResource() resource.Resource { return &FieldResource{} }

type FieldResource struct{ api *ApiClient }

type FieldModel struct {
	Id       types.String `tfsdk:"id"`
	PhaseId  types.String `tfsdk:"phase_id"`
	Type     types.String `tfsdk:"type"`
	Label    types.String `tfsdk:"label"`
	Required types.Bool   `tfsdk:"required"`
}

func (r *FieldResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_field"
}

func (r *FieldResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Phase field resource",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"phase_id": schema.StringAttribute{Required: true},
			"type":     schema.StringAttribute{Required: true},
			"label":    schema.StringAttribute{Required: true},
			"required": schema.BoolAttribute{Optional: true},
		},
	}
}

func (r *FieldResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data FieldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mutation := "mutation($phaseId:ID!,$type:ID!,$label:String!,$required:Boolean){ createPhaseField(input:{ phase_id:$phaseId, type:$type, label:$label, required:$required }){ phase_field{ id label } } }"
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
				Id    string `json:"id"`
				Label string `json:"label"`
			} `json:"phase_field"`
		} `json:"createPhaseField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create field failed", err.Error())
		return
	}
	data.Id = types.StringValue(out.CreatePhaseField.PhaseField.Id)
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
	query := "query($phaseId:ID!){ phase(id:$phaseId){ fields{ id label } } }"
	vars := map[string]interface{}{"phaseId": data.PhaseId.ValueString()}
	var out struct {
		Phase *struct {
			Fields []struct {
				Id    string `json:"id"`
				Label string `json:"label"`
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
		Id    string `json:"id"`
		Label string `json:"label"`
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FieldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data FieldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!,$label:String!,$required:Boolean){ updatePhaseField(input:{ id:$id, label:$label, required:$required }){ phase_field{ id } } }"
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
				Id string `json:"id"`
			} `json:"phase_field"`
		} `json:"updatePhaseField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update field failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data FieldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!){ deletePhaseField(input:{ id:$id }){ success } }"
	vars := map[string]interface{}{"id": data.Id.ValueString()}
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
