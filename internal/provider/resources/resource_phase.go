// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/locks"
)

var _ resource.Resource = &PhaseResource{}
var _ resource.ResourceWithImportState = &PhaseResource{}

func NewPhaseResource() resource.Resource { return &PhaseResource{} }

type PhaseResource struct{ api *client.ApiClient }

type PhaseModel struct {
	Id                              types.String  `tfsdk:"id"`
	PipeId                          types.String  `tfsdk:"pipe_id"`
	Name                            types.String  `tfsdk:"name"`
	Done                            types.Bool    `tfsdk:"done"`
	Description                     types.String  `tfsdk:"description"`
	Index                           types.Float64 `tfsdk:"index"`
	LatenessTime                    types.Int64   `tfsdk:"lateness_time"`
	CanReceiveCardDirectlyFromDraft types.Bool    `tfsdk:"can_receive_card_directly_from_draft"`
}

const phaseSelection = "id name done description index lateness_time can_receive_card_directly_from_draft repo_id"

type phasePayload struct {
	Id                              string   `json:"id"`
	Name                            string   `json:"name"`
	Done                            bool     `json:"done"`
	Description                     *string  `json:"description"`
	Index                           *float64 `json:"index"`
	LatenessTime                    *int64   `json:"lateness_time"`
	CanReceiveCardDirectlyFromDraft *bool    `json:"can_receive_card_directly_from_draft"`
	RepoId                          int64    `json:"repo_id"`
}

func (m *PhaseModel) setFromApi(p phasePayload) {
	m.Id = types.StringValue(p.Id)
	m.Name = types.StringValue(p.Name)
	if p.RepoId != 0 {
		m.PipeId = types.StringValue(strconv.FormatInt(p.RepoId, 10))
	}
	m.Done = types.BoolValue(p.Done)
	m.Description = types.StringPointerValue(p.Description)
	m.Index = types.Float64PointerValue(p.Index)
	m.LatenessTime = types.Int64PointerValue(p.LatenessTime)
	m.CanReceiveCardDirectlyFromDraft = types.BoolPointerValue(p.CanReceiveCardDirectlyFromDraft)
}

func (m *PhaseModel) fillUnknowns(p phasePayload) {
	if m.Done.IsUnknown() {
		m.Done = types.BoolValue(p.Done)
	}
	if m.Description.IsUnknown() {
		m.Description = types.StringPointerValue(p.Description)
	}
	if m.Index.IsUnknown() {
		m.Index = types.Float64PointerValue(p.Index)
	}
	if m.LatenessTime.IsUnknown() {
		m.LatenessTime = types.Int64PointerValue(p.LatenessTime)
	}
	if m.CanReceiveCardDirectlyFromDraft.IsUnknown() {
		m.CanReceiveCardDirectlyFromDraft = types.BoolPointerValue(p.CanReceiveCardDirectlyFromDraft)
	}
}

func hasValue(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }

func (m *PhaseModel) addSharedPhaseVars(vars map[string]any) {
	if hasValue(m.Done) {
		vars["done"] = m.Done.ValueBool()
	}
	if hasValue(m.Description) {
		vars["description"] = m.Description.ValueString()
	}
	if hasValue(m.LatenessTime) {
		vars["latenessTime"] = m.LatenessTime.ValueInt64()
	}
	if hasValue(m.CanReceiveCardDirectlyFromDraft) {
		vars["canReceiveCardDirectlyFromDraft"] = m.CanReceiveCardDirectlyFromDraft.ValueBool()
	}
}

func (r *PhaseResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_phase"
}

func (r *PhaseResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Phase resource",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, Description: "The ID of the phase", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"pipe_id":     schema.StringAttribute{Required: true, Description: "The ID of the pipe that the phase belongs to", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":        schema.StringAttribute{Required: true, Description: "Name of the phase"},
			"done":        schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether the phase is a final phase"},
			"description": schema.StringAttribute{Optional: true, Computed: true, Description: "Description of the phase"},
			"index": schema.Float64Attribute{
				Optional:      true,
				Computed:      true,
				Description:   "Position of the phase on the board. The API only accepts index at creation, so changing a configured index forces replacement of the phase (cards in the phase are lost). Reordering phases outside Terraform also changes index, so a configured index can trigger replacement after such drift.",
				PlanModifiers: []planmodifier.Float64{float64planmodifier.RequiresReplaceIfConfigured()},
			},
			// color is intentionally not managed: the Pipefy API rejects
			// changing a phase color, so exposing it would only error.
			"lateness_time":                        schema.Int64Attribute{Optional: true, Computed: true, Description: "SLA of the phase, in seconds"},
			"can_receive_card_directly_from_draft": schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether cards can be created directly in this phase"},
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

	// Pipefy rejects concurrent phase creates for the same pipe; serialize per pipe.
	unlock := locks.LockRepo(data.PipeId.ValueString())
	defer unlock()

	mutation := "mutation($pipeId:ID!,$name:String!,$done:Boolean,$description:String,$index:Float,$latenessTime:Int,$canReceiveCardDirectlyFromDraft:Boolean){ createPhase(input:{ pipe_id:$pipeId, name:$name, done:$done, description:$description, index:$index, lateness_time:$latenessTime, can_receive_card_directly_from_draft:$canReceiveCardDirectlyFromDraft }){ phase{ " + phaseSelection + " } } }"
	vars := map[string]any{"pipeId": data.PipeId.ValueString(), "name": data.Name.ValueString()}
	data.addSharedPhaseVars(vars)
	if hasValue(data.Index) {
		vars["index"] = data.Index.ValueFloat64()
	}
	var out struct {
		CreatePhase struct {
			Phase phasePayload `json:"phase"`
		} `json:"createPhase"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create phase failed", err.Error())
		return
	}
	phase := out.CreatePhase.Phase

	data.Id = types.StringValue(phase.Id)
	data.fillUnknowns(phase)
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

	query := "query($id:ID!){ phase(id:$id){ " + phaseSelection + " } }"
	vars := map[string]any{"id": data.Id.ValueString()}
	var out struct {
		Phase *phasePayload `json:"phase"`
	}
	if err := r.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read phase failed", err.Error())
		return
	}
	if out.Phase == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	data.setFromApi(*out.Phase)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PhaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PhaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!,$name:String!,$done:Boolean,$description:String,$latenessTime:Int,$canReceiveCardDirectlyFromDraft:Boolean){ updatePhase(input:{ id:$id, name:$name, done:$done, description:$description, lateness_time:$latenessTime, can_receive_card_directly_from_draft:$canReceiveCardDirectlyFromDraft }){ phase{ " + phaseSelection + " } } }"
	vars := map[string]any{"id": data.Id.ValueString(), "name": data.Name.ValueString()}
	data.addSharedPhaseVars(vars)
	var out struct {
		UpdatePhase struct {
			Phase phasePayload `json:"phase"`
		} `json:"updatePhase"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update phase failed", err.Error())
		return
	}
	data.fillUnknowns(out.UpdatePhase.Phase)
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
