// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"errors"
	"fmt"

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
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
)

var _ resource.Resource = &FieldResource{}
var _ resource.ResourceWithImportState = &FieldResource{}

func NewFieldResource() resource.Resource { return &FieldResource{} }

type FieldResource struct{ api *pipefy.Client }

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
				Description:   "Custom validation rule applied to the field value. Empty string and null are equivalent, and the API honours this attribute only on field types that support custom validation. See https://developers.pipefy.com/reference.",
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
	api, ok := req.ProviderData.(*pipefy.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *pipefy.Client, got %T", req.ProviderData))
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

	writes := fieldWrites(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	field, err := r.api.Fields.Create(ctx, pipefy.CreateFieldInput{
		PhaseID:     data.PhaseId.ValueString(),
		Type:        data.Type.ValueString(),
		Label:       data.Label.ValueString(),
		FieldWrites: writes,
	})
	if err != nil {
		resp.Diagnostics.AddError("create field failed", err.Error())
		return
	}
	fillFieldFromAPI(ctx, &data, field, &resp.Diagnostics)
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

	found, err := r.api.Fields.GetByUUID(ctx, data.PhaseId.ValueString(), data.Uuid.ValueString())
	if errors.Is(err, pipefy.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("read field failed", err.Error())
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
	writes := fieldWrites(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	in := pipefy.UpdateFieldInput{
		ID:          data.Id.ValueString(),
		UUID:        data.Uuid.ValueString(),
		FieldWrites: writes,
	}
	// Label is sent on a null-check alone, not the usual hasValue: an unknown
	// label still goes out.
	if !data.Label.IsNull() {
		label := data.Label.ValueString()
		in.Label = &label
	}
	field, err := r.api.Fields.Update(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("update field failed", err.Error())
		return
	}
	fillFieldFromAPI(ctx, &data, field, &resp.Diagnostics)
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

	if err := r.api.Fields.Delete(ctx, data.PhaseId.ValueString(), data.Id.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete field failed", err.Error())
		return
	}
}

func (r *FieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, ok := splitImportID(req.ID)
	if !ok || len(parts) != 2 {
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

func floatPtr(p *float64) types.Float64 {
	if p == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*p)
}

// fieldWrites carries each attribute only when it has a concrete value, so an
// omitted Optional+Computed attribute keeps its server value instead of being cleared.
func fieldWrites(ctx context.Context, data FieldModel, diags *diag.Diagnostics) pipefy.FieldWrites {
	writes := pipefy.FieldWrites{
		Required:         optionalBool(data.Required),
		Description:      optionalString(data.Description),
		Help:             optionalString(data.Help),
		Editable:         optionalBool(data.Editable),
		MinimalView:      optionalBool(data.MinimalView),
		CustomValidation: optionalString(data.CustomValidation),
		Index:            optionalFloat64(data.Index),
	}
	if hasValue(data.Options) {
		var opts []string
		diags.Append(data.Options.ElementsAs(ctx, &opts, false)...)
		writes.Options = &opts
	}
	return writes
}

// fillFieldFromAPI is the Create and Update mapping: take from the response
// only what the plan could not know. label and type are Required, so they stay
// as planned. options and index always take the API value; preserving the plan
// would silence a server-side change. custom_validation always goes through
// mergeEmptyish, including when the plan already knew the value, so a rule the
// API drops fails apply instead of looping as a perpetual plan.
func fillFieldFromAPI(ctx context.Context, data *FieldModel, f pipefy.Field, diags *diag.Diagnostics) {
	data.Id = fillUnknownString(data.Id, types.StringValue(f.ID))
	data.InternalId = fillUnknownString(data.InternalId, types.StringValue(f.InternalID))
	data.Uuid = fillUnknownString(data.Uuid, types.StringValue(f.UUID))
	data.Required = fillUnknownBool(data.Required, boolPtr(f.Required))
	data.Description = fillUnknownString(data.Description, strPtr(f.Description))
	data.Help = fillUnknownString(data.Help, strPtr(f.Help))
	data.Editable = fillUnknownBool(data.Editable, boolPtr(f.Editable))
	data.MinimalView = fillUnknownBool(data.MinimalView, boolPtr(f.MinimalView))
	data.CustomValidation = mergeEmptyish(data.CustomValidation, f.CustomValidation)
	data.Index = floatPtr(f.Index)
	data.Options = optionsToList(ctx, f.Options, diags)
}

// applyFieldToModel is the Read mapping. It overwrites every attribute so
// drift surfaces. phase_id is not in the payload; it is set at create/import
// and left untouched here.
func applyFieldToModel(ctx context.Context, data *FieldModel, f pipefy.Field, diags *diag.Diagnostics) {
	data.Id = types.StringValue(f.ID)
	data.InternalId = types.StringValue(f.InternalID)
	data.Uuid = types.StringValue(f.UUID)
	data.Label = types.StringValue(f.Label)
	data.Type = types.StringValue(f.Type)
	data.Required = boolPtr(f.Required)
	data.Description = strPtr(f.Description)
	data.Help = strPtr(f.Help)
	data.Editable = boolPtr(f.Editable)
	data.MinimalView = boolPtr(f.MinimalView)
	data.CustomValidation = mergeEmptyish(data.CustomValidation, f.CustomValidation)
	data.Index = floatPtr(f.Index)
	data.Options = optionsToList(ctx, f.Options, diags)
}
