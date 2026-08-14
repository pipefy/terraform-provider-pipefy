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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
)

var _ resource.Resource = &TableFieldResource{}
var _ resource.ResourceWithImportState = &TableFieldResource{}

func NewTableFieldResource() resource.Resource { return &TableFieldResource{} }

type TableFieldResource struct{ api *pipefy.Client }

type TableFieldModel struct {
	Id         types.String `tfsdk:"id"`
	InternalId types.String `tfsdk:"internal_id"`
	Uuid       types.String `tfsdk:"uuid"`
	TableId    types.String `tfsdk:"table_id"`
	Type       types.String `tfsdk:"type"`
	Label      types.String `tfsdk:"label"`
	Required   types.Bool   `tfsdk:"required"`
	Options    types.List   `tfsdk:"options"`

	Description      types.String `tfsdk:"description"`
	Help             types.String `tfsdk:"help"`
	MinimalView      types.Bool   `tfsdk:"minimal_view"`
	CustomValidation types.String `tfsdk:"custom_validation"`
	Unique           types.Bool   `tfsdk:"unique"`
}

func (r *TableFieldResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_table_field"
}

func (r *TableFieldResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Table field resource",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, Description: "The slug of the field", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"internal_id": schema.StringAttribute{Computed: true, Description: "The unique internal ID of the field", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"uuid":        schema.StringAttribute{Computed: true, Description: "The field's UUID. A stable identifier that does not change when the label changes.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"table_id":    schema.StringAttribute{Required: true, Description: "The ID of the table that the field belongs to", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
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
			"minimal_view": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Whether the field is shown in the record's minimal (summary) view",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"custom_validation": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Custom validation rule applied to the field value. Empty string and null are equivalent, and the API honours this attribute only on field types that support custom validation. See https://developers.pipefy.com/reference.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"unique": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Whether the field value must be unique across the table's records",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *TableFieldResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TableFieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TableFieldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	writes := tableFieldWrites(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	field, err := r.api.TableFields.Create(ctx, pipefy.CreateTableFieldInput{
		TableID:          data.TableId.ValueString(),
		Type:             data.Type.ValueString(),
		Label:            data.Label.ValueString(),
		TableFieldWrites: writes,
	})
	if err != nil {
		resp.Diagnostics.AddError("create table field failed", err.Error())
		return
	}
	fillTableFieldFromAPI(ctx, &data, field, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TableFieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TableFieldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// uuid is Read's lookup key; on import id is unset and resolved here.
	if data.Uuid.IsNull() || data.Uuid.ValueString() == "" {
		return
	}

	found, err := r.api.TableFields.GetByUUID(ctx, data.TableId.ValueString(), data.Uuid.ValueString())
	if errors.Is(err, pipefy.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("read table field failed", err.Error())
		return
	}

	applyTableFieldToModel(ctx, &data, found, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TableFieldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data TableFieldModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	writes := tableFieldWrites(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	in := pipefy.UpdateTableFieldInput{
		TableID:          data.TableId.ValueString(),
		ID:               data.Id.ValueString(),
		TableFieldWrites: writes,
	}
	// Label is sent on a null-check alone, not the usual hasValue: an unknown
	// label still goes out.
	if !data.Label.IsNull() {
		label := data.Label.ValueString()
		in.Label = &label
	}
	field, err := r.api.TableFields.Update(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("update table field failed", err.Error())
		return
	}
	fillTableFieldFromAPI(ctx, &data, field, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TableFieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TableFieldModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.api.TableFields.Delete(ctx, data.TableId.ValueString(), data.Id.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete table field failed", err.Error())
		return
	}
}

func (r *TableFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, ok := splitImportID(req.ID)
	if !ok || len(parts) != 2 {
		resp.Diagnostics.AddError("invalid import ID", "expected table_id/field_uuid, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("table_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), parts[1])...)
}

// tableFieldWrites carries each attribute only when it has a concrete value, so an
// omitted Optional+Computed attribute keeps its server value instead of being cleared.
func tableFieldWrites(ctx context.Context, data TableFieldModel, diags *diag.Diagnostics) pipefy.TableFieldWrites {
	writes := pipefy.TableFieldWrites{
		Required:         optionalBool(data.Required),
		Description:      optionalString(data.Description),
		Help:             optionalString(data.Help),
		MinimalView:      optionalBool(data.MinimalView),
		CustomValidation: optionalString(data.CustomValidation),
		Unique:           optionalBool(data.Unique),
	}
	if hasValue(data.Options) {
		var opts []string
		diags.Append(data.Options.ElementsAs(ctx, &opts, false)...)
		writes.Options = &opts
	}
	return writes
}

// fillTableFieldFromAPI takes unknowns from the response. options always takes
// the API value. custom_validation always goes through mergeEmptyish so a
// dropped rule fails apply.
func fillTableFieldFromAPI(ctx context.Context, data *TableFieldModel, f pipefy.TableField, diags *diag.Diagnostics) {
	data.Id = fillUnknownString(data.Id, types.StringValue(f.ID))
	data.InternalId = fillUnknownString(data.InternalId, types.StringValue(f.InternalID))
	data.Uuid = fillUnknownString(data.Uuid, types.StringValue(f.UUID))
	data.Required = fillUnknownBool(data.Required, boolPtr(f.Required))
	data.Description = fillUnknownString(data.Description, strPtr(f.Description))
	data.Help = fillUnknownString(data.Help, strPtr(f.Help))
	data.MinimalView = fillUnknownBool(data.MinimalView, boolPtr(f.MinimalView))
	data.CustomValidation = mergeEmptyish(data.CustomValidation, f.CustomValidation)
	data.Unique = fillUnknownBool(data.Unique, boolPtr(f.Unique))
	data.Options = optionsToList(ctx, f.Options, diags)
}

// applyTableFieldToModel maps a fetched field onto the model. table_id is not
// in the payload; it is set at create/import and left untouched here.
func applyTableFieldToModel(ctx context.Context, data *TableFieldModel, f pipefy.TableField, diags *diag.Diagnostics) {
	data.Id = types.StringValue(f.ID)
	data.InternalId = types.StringValue(f.InternalID)
	data.Uuid = types.StringValue(f.UUID)
	data.Label = types.StringValue(f.Label)
	data.Type = types.StringValue(f.Type)
	data.Required = boolPtr(f.Required)
	data.Description = strPtr(f.Description)
	data.Help = strPtr(f.Help)
	data.MinimalView = boolPtr(f.MinimalView)
	data.CustomValidation = mergeEmptyish(data.CustomValidation, f.CustomValidation)
	data.Unique = boolPtr(f.Unique)
	data.Options = optionsToList(ctx, f.Options, diags)
}
