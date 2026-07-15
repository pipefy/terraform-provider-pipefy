// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/fieldgql"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/locks"
)

var _ resource.Resource = &FieldResource{}
var _ resource.ResourceWithImportState = &FieldResource{}

func NewFieldResource() resource.Resource { return &FieldResource{} }

type FieldResource struct{ api *client.ApiClient }

type FieldModel struct {
	Id         types.String `tfsdk:"id"`
	InternalId types.String `tfsdk:"internal_id"`
	Uuid       types.String `tfsdk:"uuid"`
	PhaseId    types.String `tfsdk:"phase_id"`
	Type       types.String `tfsdk:"type"`
	Label      types.String `tfsdk:"label"`
	Required   types.Bool   `tfsdk:"required"`
	Options    types.List   `tfsdk:"options"`

	Description      types.String  `tfsdk:"description"`
	Help             types.String  `tfsdk:"help"`
	Editable         types.Bool    `tfsdk:"editable"`
	MinimalView      types.Bool    `tfsdk:"minimal_view"`
	CustomValidation types.String  `tfsdk:"custom_validation"`
	Index            types.Float64 `tfsdk:"index"`
}

func (r *FieldResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_field"
}

func (r *FieldResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Phase field resource",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, Description: "The slug of the field", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"internal_id": schema.StringAttribute{Computed: true, Description: "The unique internal ID of the field", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"uuid":        schema.StringAttribute{Computed: true, Description: "The field's UUID. A stable identifier that does not change when the label changes.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"phase_id":    schema.StringAttribute{Required: true, Description: "The ID of the phase that the field belongs to", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"type":        schema.StringAttribute{Required: true, Description: "The field type. See https://developers.pipefy.com/reference for the current list of supported types.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"label":       schema.StringAttribute{Required: true, Description: "The displayed name of the field"},
			"required": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Whether the field is required or not",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"options": schema.ListAttribute{
				ElementType:   types.StringType,
				Optional:      true,
				Computed:      true,
				Description:   "Choices for option-based field types (checklist_vertical, checklist_horizontal, radio_vertical, radio_horizontal, select, label_select). Order is preserved and user-visible.",
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
			"description": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Helper description shown under the field",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"help": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Help text shown for the field",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"editable": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Whether the field value can be edited after creation",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"minimal_view": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Whether the field is shown in the card's minimal (summary) view",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"custom_validation": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Custom validation rule applied to the field value",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"index": schema.Float64Attribute{
				Optional:      true,
				Computed:      true,
				Description:   "Position of the field within the phase form",
				PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()},
			},
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

	// Resolve repo_id from the phase to lock per repo
	// pipefy api does not allow multiple field creations at the same time for the same repo
	phaseQuery := "query GetPhaseRepoId_tf($id:ID!){ phase(id:$id){ repo_id } }"
	phaseVars := map[string]any{"id": data.PhaseId.ValueString()}
	var phaseOut struct {
		Phase *struct {
			RepoId int `json:"repo_id"`
		} `json:"phase"`
	}
	if err := r.api.DoGraphQL(ctx, phaseQuery, phaseVars, &phaseOut); err != nil {
		resp.Diagnostics.AddError("create field failed", fmt.Sprintf("failed to fetch phase repo_id: %s", err.Error()))
		return
	}
	if phaseOut.Phase == nil || phaseOut.Phase.RepoId == 0 {
		resp.Diagnostics.AddError("create field failed", "could not resolve valid phase repo_id from phase query")
		return
	}
	repoIDStr := strconv.FormatInt(int64(phaseOut.Phase.RepoId), 10)

	unlock := locks.LockRepo(repoIDStr)
	defer unlock()

	mutation := "mutation CreatePhaseField_tf($phaseId:ID!,$type:ID!,$label:String!,$required:Boolean,$options:[String],$description:String,$help:String,$editable:Boolean,$minimalView:Boolean,$customValidation:String,$index:Float){ createPhaseField(input:{ phase_id:$phaseId, type:$type, label:$label, required:$required, options:$options, description:$description, help:$help, editable:$editable, minimal_view:$minimalView, custom_validation:$customValidation, index:$index }){ phase_field{ " + fieldgql.Selection + " } } }"
	vars := map[string]any{
		"phaseId": data.PhaseId.ValueString(),
		"type":    data.Type.ValueString(),
		"label":   data.Label.ValueString(),
	}
	addFieldWriteVars(ctx, data, vars, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		CreatePhaseField struct {
			PhaseField fieldgql.Field `json:"phase_field"`
		} `json:"createPhaseField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create field failed", err.Error())
		return
	}
	applyFieldToModel(ctx, &data, out.CreatePhaseField.PhaseField, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data FieldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// uuid is Read's lookup key; on import id is unset and resolved here.
	if data.Uuid.IsNull() || data.Uuid.ValueString() == "" {
		return
	}

	// Query the phase to get the field information
	query := "query GetPhaseFields_tf($phaseId:ID!){ phase(id:$phaseId){ fields{ " + fieldgql.Selection + " } } }"
	vars := map[string]any{"phaseId": data.PhaseId.ValueString()}
	var out struct {
		Phase *struct {
			Fields []fieldgql.Field `json:"fields"`
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

	found, ok := fieldgql.FindByUUID(out.Phase.Fields, data.Uuid.ValueString())
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}

	applyFieldToModel(ctx, &data, found, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
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
	mutation := "mutation UpdatePhaseField_tf($id:ID!,$uuid:ID!,$label:String!,$required:Boolean,$options:[String],$description:String,$help:String,$editable:Boolean,$minimalView:Boolean,$customValidation:String,$index:Float){ updatePhaseField(input:{ id:$id, uuid:$uuid, label:$label, required:$required, options:$options, description:$description, help:$help, editable:$editable, minimal_view:$minimalView, custom_validation:$customValidation, index:$index }){ phase_field{ " + fieldgql.Selection + " } } }"
	vars := map[string]any{
		"id":   data.Id.ValueString(),
		"uuid": data.Uuid.ValueString(),
	}
	if !data.Label.IsNull() {
		vars["label"] = data.Label.ValueString()
	}
	addFieldWriteVars(ctx, data, vars, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		UpdatePhaseField struct {
			PhaseField fieldgql.Field `json:"phase_field"`
		} `json:"updatePhaseField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update field failed", err.Error())
		return
	}
	applyFieldToModel(ctx, &data, out.UpdatePhaseField.PhaseField, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
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

	// Fetch repo_id from the phase
	phaseQuery := "query GetPhaseRepoId_tf($id:ID!){ phase(id:$id){ repo_id } }"
	phaseVars := map[string]any{"id": data.PhaseId.ValueString()}
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
	pipeQuery := "query GetPipeUuid_tf($id:ID!){ pipe(id:$id){ uuid } }"
	pipeVars := map[string]any{"id": repoIDStr}
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

	mutation := "mutation DeletePhaseField_tf($id:ID!,$pipeUuid:ID!){ deletePhaseField(input:{ id:$id, pipeUuid:$pipeUuid }){ success } }"
	vars := map[string]any{"id": data.Id.ValueString(), "pipeUuid": pipeOut.Pipe.Uuid}
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
	parts, ok := splitImportID(req.ID, 2)
	if !ok {
		resp.Diagnostics.AddError("invalid import ID", "expected phase_id/field_uuid, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("phase_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), parts[1])...)
}

func optionsToList(ctx context.Context, opts []string, diags *diag.Diagnostics) types.List {
	if len(opts) == 0 {
		return types.ListNull(types.StringType)
	}
	list, d := types.ListValueFrom(ctx, types.StringType, opts)
	diags.Append(d...)
	return list
}

func strPtr(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

func boolPtr(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}

// addFieldWriteVars sends each attribute only when it has a concrete value, so an
// omitted Optional+Computed attribute keeps its server value instead of being cleared.
func addFieldWriteVars(ctx context.Context, data FieldModel, vars map[string]any, diags *diag.Diagnostics) {
	if !data.Required.IsNull() && !data.Required.IsUnknown() {
		vars["required"] = data.Required.ValueBool()
	}
	if !data.Options.IsNull() && !data.Options.IsUnknown() {
		var opts []string
		diags.Append(data.Options.ElementsAs(ctx, &opts, false)...)
		vars["options"] = opts
	}
	if !data.Description.IsNull() && !data.Description.IsUnknown() {
		vars["description"] = data.Description.ValueString()
	}
	if !data.Help.IsNull() && !data.Help.IsUnknown() {
		vars["help"] = data.Help.ValueString()
	}
	if !data.Editable.IsNull() && !data.Editable.IsUnknown() {
		vars["editable"] = data.Editable.ValueBool()
	}
	if !data.MinimalView.IsNull() && !data.MinimalView.IsUnknown() {
		vars["minimalView"] = data.MinimalView.ValueBool()
	}
	if !data.CustomValidation.IsNull() && !data.CustomValidation.IsUnknown() {
		vars["customValidation"] = data.CustomValidation.ValueString()
	}
	if !data.Index.IsNull() && !data.Index.IsUnknown() {
		vars["index"] = data.Index.ValueFloat64()
	}
}

// applyFieldToModel maps a fetched field onto the model. phase_id is not in the
// payload; it is set at create/import and left untouched here.
func applyFieldToModel(ctx context.Context, data *FieldModel, f fieldgql.Field, diags *diag.Diagnostics) {
	data.Id = types.StringValue(f.Id)
	data.InternalId = types.StringValue(f.InternalId)
	data.Uuid = types.StringValue(f.Uuid)
	data.Label = types.StringValue(f.Label)
	data.Type = types.StringValue(f.Type)
	data.Required = boolPtr(f.Required)
	data.Description = strPtr(f.Description)
	data.Help = strPtr(f.Help)
	data.Editable = boolPtr(f.Editable)
	data.MinimalView = boolPtr(f.MinimalView)
	data.CustomValidation = strPtr(f.CustomValidation)
	if f.Index == nil {
		data.Index = types.Float64Null()
	} else {
		data.Index = types.Float64Value(*f.Index)
	}
	data.Options = optionsToList(ctx, f.Options, diags)
}
