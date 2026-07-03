// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/piperelationgql"
)

var _ resource.Resource = &PipeRelationResource{}
var _ resource.ResourceWithImportState = &PipeRelationResource{}

func NewPipeRelationResource() resource.Resource { return &PipeRelationResource{} }

type PipeRelationResource struct{ api *client.ApiClient }

type pipeRelationFieldMapModel struct {
	FieldId   types.String `tfsdk:"field_id"`
	InputMode types.String `tfsdk:"input_mode"`
	Value     types.String `tfsdk:"value"`
}

var ownFieldMapObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"field_id":   types.StringType,
	"input_mode": types.StringType,
	"value":      types.StringType,
}}

type PipeRelationModel struct {
	Id                                  types.String                `tfsdk:"id"`
	ParentId                            types.String                `tfsdk:"parent_id"`
	ChildId                             types.String                `tfsdk:"child_id"`
	Name                                types.String                `tfsdk:"name"`
	CanCreateNewItems                   types.Bool                  `tfsdk:"can_create_new_items"`
	CanConnectExistingItems             types.Bool                  `tfsdk:"can_connect_existing_items"`
	CanConnectMultipleItems             types.Bool                  `tfsdk:"can_connect_multiple_items"`
	AllChildrenMustBeDoneToFinishParent types.Bool                  `tfsdk:"all_children_must_be_done_to_finish_parent"`
	AllChildrenMustBeDoneToMoveParent   types.Bool                  `tfsdk:"all_children_must_be_done_to_move_parent"`
	ChildMustExistToFinishParent        types.Bool                  `tfsdk:"child_must_exist_to_finish_parent"`
	ChildMustExistToMoveParent          types.Bool                  `tfsdk:"child_must_exist_to_move_parent"`
	AutoFillFieldEnabled                types.Bool                  `tfsdk:"auto_fill_field_enabled"`
	OwnFieldMaps                        []pipeRelationFieldMapModel `tfsdk:"own_field_maps"`
}

func (r *PipeRelationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipe_relation"
}

func relationBoolAttr(description string, def bool) schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional:    true,
		Computed:    true,
		Default:     booldefault.StaticBool(def),
		Description: description,
	}
}

func (r *PipeRelationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Connects a parent pipe to a child pipe. Both `parent_id` and `child_id` must be pipes; changing either forces a new relation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The ID of the pipe relation",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"parent_id": schema.StringAttribute{
				Required:      true,
				Description:   "The ID of the parent pipe",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"child_id": schema.StringAttribute{
				Required:      true,
				Description:   "The ID of the child pipe",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Name of the relation"},
			"can_create_new_items": relationBoolAttr(
				"Whether new connected items can be created through the relation.", true),
			"can_connect_existing_items": relationBoolAttr(
				"Whether existing items can be connected through the relation.", true),
			"can_connect_multiple_items": relationBoolAttr(
				"Whether multiple items can be connected through the relation.", true),
			"all_children_must_be_done_to_finish_parent": relationBoolAttr(
				"Whether all connected children must be done before the parent can be finished.", false),
			"all_children_must_be_done_to_move_parent": relationBoolAttr(
				"Whether all connected children must be done before the parent can be moved.", false),
			"child_must_exist_to_finish_parent": relationBoolAttr(
				"Whether at least one connected child must exist before the parent can be finished.", false),
			"child_must_exist_to_move_parent": relationBoolAttr(
				"Whether at least one connected child must exist before the parent can be moved.", false),
			"auto_fill_field_enabled": relationBoolAttr(
				"Whether auto-fill of child start-form fields from the parent is enabled. Pair with `own_field_maps`.", false),
			"own_field_maps": schema.SetNestedAttribute{
				Optional:    true,
				Computed:    true,
				Default:     setdefault.StaticValue(types.SetValueMust(ownFieldMapObjectType, []attr.Value{})),
				Description: "Field mappings that auto-fill a child item's start-form fields from the parent item. The set is managed in full: the configured mappings are the ones kept, and an empty list (or omitting the block) clears them on the server.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"field_id": schema.StringAttribute{
							Required:    true,
							Description: "The child start-form field to fill, given as the field's internal_id.",
						},
						"input_mode": schema.StringAttribute{
							Required:    true,
							Description: "How the value is supplied, for example `fixed_value`. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference).",
						},
						"value": schema.StringAttribute{
							Required:    true,
							Description: "The value or source-field reference for the mapping.",
						},
					},
				},
			},
		},
	}
}

func (r *PipeRelationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// writeInput sends every boolean unconditionally: the API types them Boolean!
// (non-null), so an omitted flag is rejected.
func (m *PipeRelationModel) writeInput() map[string]any {
	in := map[string]any{
		"name":                                m.Name.ValueString(),
		"canCreateNewItems":                   m.CanCreateNewItems.ValueBool(),
		"canConnectExistingItems":             m.CanConnectExistingItems.ValueBool(),
		"canConnectMultipleItems":             m.CanConnectMultipleItems.ValueBool(),
		"allChildrenMustBeDoneToFinishParent": m.AllChildrenMustBeDoneToFinishParent.ValueBool(),
		"allChildrenMustBeDoneToMoveParent":   m.AllChildrenMustBeDoneToMoveParent.ValueBool(),
		"childMustExistToFinishParent":        m.ChildMustExistToFinishParent.ValueBool(),
		"childMustExistToMoveParent":          m.ChildMustExistToMoveParent.ValueBool(),
		"autoFillFieldEnabled":                m.AutoFillFieldEnabled.ValueBool(),
	}
	maps := make([]map[string]any, len(m.OwnFieldMaps))
	for i, fm := range m.OwnFieldMaps {
		maps[i] = map[string]any{
			"fieldId":   fm.FieldId.ValueString(),
			"inputMode": fm.InputMode.ValueString(),
			"value":     fm.Value.ValueString(),
		}
	}
	in["ownFieldMaps"] = maps
	return in
}

func fieldMapsToModel(maps []piperelationgql.FieldMap) []pipeRelationFieldMapModel {
	out := make([]pipeRelationFieldMapModel, len(maps))
	for i, fm := range maps {
		out[i] = pipeRelationFieldMapModel{
			FieldId:   types.StringValue(fm.FieldId),
			InputMode: types.StringValue(fm.InputMode),
			Value:     types.StringValue(fm.Value),
		}
	}
	return out
}

func boolOr(p *bool, current types.Bool) types.Bool {
	if p == nil {
		return current
	}
	return types.BoolValue(*p)
}

func (m *PipeRelationModel) apply(rel piperelationgql.Relation) {
	m.Id = types.StringValue(rel.Id)
	m.Name = types.StringValue(rel.Name)
	m.CanCreateNewItems = boolOr(rel.CanCreateNewItems, m.CanCreateNewItems)
	m.CanConnectExistingItems = boolOr(rel.CanConnectExistingItems, m.CanConnectExistingItems)
	m.CanConnectMultipleItems = boolOr(rel.CanConnectMultipleItems, m.CanConnectMultipleItems)
	m.AllChildrenMustBeDoneToFinishParent = boolOr(rel.AllChildrenMustBeDoneToFinishParent, m.AllChildrenMustBeDoneToFinishParent)
	m.AllChildrenMustBeDoneToMoveParent = boolOr(rel.AllChildrenMustBeDoneToMoveParent, m.AllChildrenMustBeDoneToMoveParent)
	m.ChildMustExistToFinishParent = boolOr(rel.ChildMustExistToFinishParent, m.ChildMustExistToFinishParent)
	m.ChildMustExistToMoveParent = boolOr(rel.ChildMustExistToMoveParent, m.ChildMustExistToMoveParent)
	m.AutoFillFieldEnabled = boolOr(rel.AutoFillFieldEnabled, m.AutoFillFieldEnabled)
	if rel.Parent != nil && rel.Parent.Id != "" {
		m.ParentId = types.StringValue(rel.Parent.Id)
	}
	if rel.Child != nil && rel.Child.Id != "" {
		m.ChildId = types.StringValue(rel.Child.Id)
	}
	m.OwnFieldMaps = fieldMapsToModel(rel.OwnFieldMaps)
}

func (r *PipeRelationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PipeRelationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := data.writeInput()
	input["parentId"] = data.ParentId.ValueString()
	input["childId"] = data.ChildId.ValueString()

	mutation := "mutation CreatePipeRelation_tf($input:CreatePipeRelationInput!){ createPipeRelation(input:$input){ pipeRelation{ id } } }"
	var out struct {
		CreatePipeRelation struct {
			PipeRelation struct {
				Id string `json:"id"`
			} `json:"pipeRelation"`
		} `json:"createPipeRelation"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"input": input}, &out); err != nil {
		resp.Diagnostics.AddError("create pipe relation failed", err.Error())
		return
	}
	data.Id = types.StringValue(out.CreatePipeRelation.PipeRelation.Id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeRelationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PipeRelationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	query := "query GetPipeRelations_tf($pipeId:ID!){ pipe(id:$pipeId){ childrenRelations{ " + piperelationgql.Selection + " } } }"
	var out struct {
		Pipe *struct {
			ChildrenRelations []piperelationgql.Relation `json:"childrenRelations"`
		} `json:"pipe"`
	}
	if err := r.api.DoGraphQL(ctx, query, map[string]any{"pipeId": data.ParentId.ValueString()}, &out); err != nil {
		resp.Diagnostics.AddError("read pipe relation failed", err.Error())
		return
	}
	if out.Pipe == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	rel, ok := piperelationgql.FindByID(out.Pipe.ChildrenRelations, data.Id.ValueString())
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}
	data.apply(rel)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeRelationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PipeRelationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := data.writeInput()
	input["id"] = data.Id.ValueString()

	mutation := "mutation UpdatePipeRelation_tf($input:UpdatePipeRelationInput!){ updatePipeRelation(input:$input){ pipeRelation{ id } } }"
	var out struct {
		UpdatePipeRelation struct {
			PipeRelation struct {
				Id string `json:"id"`
			} `json:"pipeRelation"`
		} `json:"updatePipeRelation"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"input": input}, &out); err != nil {
		resp.Diagnostics.AddError("update pipe relation failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeRelationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PipeRelationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation DeletePipeRelation_tf($id:ID!){ deletePipeRelation(input:{ id:$id }){ success } }"
	var out struct {
		DeletePipeRelation struct {
			Success bool `json:"success"`
		} `json:"deletePipeRelation"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"id": data.Id.ValueString()}, &out); err != nil {
		resp.Diagnostics.AddError("delete pipe relation failed", err.Error())
		return
	}
}

func (r *PipeRelationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"invalid import ID",
			"expected parent_id/relation_id, got "+req.ID,
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("parent_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
