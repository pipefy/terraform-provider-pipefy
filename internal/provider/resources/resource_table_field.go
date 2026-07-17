// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
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
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/locks"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/tablefieldgql"
)

var _ resource.Resource = &TableFieldResource{}
var _ resource.ResourceWithImportState = &TableFieldResource{}

func NewTableFieldResource() resource.Resource { return &TableFieldResource{} }

type TableFieldResource struct{ api *client.ApiClient }

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
				Description:   "Custom validation rule applied to the field value",
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
	api, ok := req.ProviderData.(*client.ApiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *ApiClient, got %T", req.ProviderData))
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

	// Table fields lock on the table's own id: unlike phase fields, a table is
	// already a top-level repo, so there is no parent repo_id to resolve first.
	unlock := locks.LockRepo(data.TableId.ValueString())
	defer unlock()

	mutation := "mutation CreateTableField_tf($tableId:ID!,$type:ID!,$label:String!,$required:Boolean,$options:[String],$description:String,$help:String,$minimalView:Boolean,$customValidation:String,$unique:Boolean){ createTableField(input:{ table_id:$tableId, type:$type, label:$label, required:$required, options:$options, description:$description, help:$help, minimal_view:$minimalView, custom_validation:$customValidation, unique:$unique }){ table_field{ " + tablefieldgql.Selection + " } } }"
	vars := map[string]any{
		"tableId": data.TableId.ValueString(),
		"type":    data.Type.ValueString(),
		"label":   data.Label.ValueString(),
	}
	addTableFieldWriteVars(ctx, data, vars, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		CreateTableField struct {
			TableField tablefieldgql.Field `json:"table_field"`
		} `json:"createTableField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("create table field failed", err.Error())
		return
	}
	applyTableFieldToModel(ctx, &data, out.CreateTableField.TableField, &resp.Diagnostics)
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

	query := "query GetTableFields_tf($tableId:ID!){ table(id:$tableId){ table_fields{ " + tablefieldgql.Selection + " } } }"
	vars := map[string]any{"tableId": data.TableId.ValueString()}
	var out struct {
		Table *struct {
			TableFields []tablefieldgql.Field `json:"table_fields"`
		} `json:"table"`
	}
	if err := r.api.DoGraphQL(ctx, query, vars, &out); err != nil {
		resp.Diagnostics.AddError("read table field failed", err.Error())
		return
	}
	if out.Table == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	found, ok := tablefieldgql.FindByUUID(out.Table.TableFields, data.Uuid.ValueString())
	if !ok {
		resp.State.RemoveResource(ctx)
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
	mutation := "mutation UpdateTableField_tf($id:ID!,$tableId:ID!,$label:String,$required:Boolean,$options:[String],$description:String,$help:String,$minimalView:Boolean,$customValidation:String,$unique:Boolean){ updateTableField(input:{ id:$id, table_id:$tableId, label:$label, required:$required, options:$options, description:$description, help:$help, minimal_view:$minimalView, custom_validation:$customValidation, unique:$unique }){ table_field{ " + tablefieldgql.Selection + " } } }"
	vars := map[string]any{
		"id":      data.Id.ValueString(),
		"tableId": data.TableId.ValueString(),
	}
	if !data.Label.IsNull() {
		vars["label"] = data.Label.ValueString()
	}
	addTableFieldWriteVars(ctx, data, vars, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		UpdateTableField struct {
			TableField tablefieldgql.Field `json:"table_field"`
		} `json:"updateTableField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update table field failed", err.Error())
		return
	}
	applyTableFieldToModel(ctx, &data, out.UpdateTableField.TableField, &resp.Diagnostics)
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

	unlock := locks.LockRepo(data.TableId.ValueString())
	defer unlock()

	// Unlike deletePhaseField, deleteTableField needs only the field id and its
	// own table_id: no pipe/uuid lookup, since a table has no parent pipe.
	mutation := "mutation DeleteTableField_tf($id:ID!,$tableId:ID!){ deleteTableField(input:{ id:$id, table_id:$tableId }){ success } }"
	vars := map[string]any{"id": data.Id.ValueString(), "tableId": data.TableId.ValueString()}
	var out struct {
		DeleteTableField struct {
			Success bool `json:"success"`
		} `json:"deleteTableField"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("delete table field failed", err.Error())
		return
	}
}

func (r *TableFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, ok := splitImportID(req.ID, 2)
	if !ok {
		resp.Diagnostics.AddError("invalid import ID", "expected table_id/field_uuid, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("table_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), parts[1])...)
}

// addTableFieldWriteVars sends each attribute only when it has a concrete value, so an
// omitted Optional+Computed attribute keeps its server value instead of being cleared.
func addTableFieldWriteVars(ctx context.Context, data TableFieldModel, vars map[string]any, diags *diag.Diagnostics) {
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
	if !data.MinimalView.IsNull() && !data.MinimalView.IsUnknown() {
		vars["minimalView"] = data.MinimalView.ValueBool()
	}
	if !data.CustomValidation.IsNull() && !data.CustomValidation.IsUnknown() {
		vars["customValidation"] = data.CustomValidation.ValueString()
	}
	if !data.Unique.IsNull() && !data.Unique.IsUnknown() {
		vars["unique"] = data.Unique.ValueBool()
	}
}

// applyTableFieldToModel maps a fetched field onto the model. table_id is not in the
// payload; it is set at create/import and left untouched here.
func applyTableFieldToModel(ctx context.Context, data *TableFieldModel, f tablefieldgql.Field, diags *diag.Diagnostics) {
	data.Id = types.StringValue(f.Id)
	data.InternalId = types.StringValue(f.InternalId)
	data.Uuid = types.StringValue(f.Uuid)
	data.Label = types.StringValue(f.Label)
	data.Type = types.StringValue(f.Type)
	data.Required = boolPtr(f.Required)
	data.Description = strPtr(f.Description)
	data.Help = strPtr(f.Help)
	data.MinimalView = boolPtr(f.MinimalView)
	data.CustomValidation = strPtr(f.CustomValidation)
	data.Unique = boolPtr(f.Unique)
	data.Options = optionsToList(ctx, f.Options, diags)
}
