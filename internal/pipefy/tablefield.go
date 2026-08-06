// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"context"

	"github.com/pipefy/terraform-provider-pipefy/internal/locks"
)

const tableFieldSelection = "id internal_id uuid label type required options " +
	"description help minimal_view custom_validation unique"

const createTableFieldMutation = "mutation CreateTableField_tf($tableId:ID!,$type:ID!,$label:String!,$required:Boolean,$options:[String],$description:String,$help:String,$minimalView:Boolean,$customValidation:String,$unique:Boolean){ createTableField(input:{ table_id:$tableId, type:$type, label:$label, required:$required, options:$options, description:$description, help:$help, minimal_view:$minimalView, custom_validation:$customValidation, unique:$unique }){ table_field{ " + tableFieldSelection + " } } }"

const getTableFieldsQuery = "query GetTableFields_tf($tableId:ID!){ table(id:$tableId){ table_fields{ " + tableFieldSelection + " } } }"

const updateTableFieldMutation = "mutation UpdateTableField_tf($id:ID!,$tableId:ID!,$label:String,$required:Boolean,$options:[String],$description:String,$help:String,$minimalView:Boolean,$customValidation:String,$unique:Boolean){ updateTableField(input:{ id:$id, table_id:$tableId, label:$label, required:$required, options:$options, description:$description, help:$help, minimal_view:$minimalView, custom_validation:$customValidation, unique:$unique }){ table_field{ " + tableFieldSelection + " } } }"

const deleteTableFieldMutation = "mutation DeleteTableField_tf($id:ID!,$tableId:ID!){ deleteTableField(input:{ id:$id, table_id:$tableId }){ success } }"

// TableField is a field of a database table. It has no index, unlike a phase
// field, and it has a uniqueness flag, which a phase field does not.
type TableField struct {
	ID               string   `json:"id"`
	InternalID       string   `json:"internal_id"`
	UUID             string   `json:"uuid"`
	Label            string   `json:"label"`
	Type             string   `json:"type"`
	Required         *bool    `json:"required"`
	Options          []string `json:"options"`
	Description      *string  `json:"description"`
	Help             *string  `json:"help"`
	MinimalView      *bool    `json:"minimal_view"`
	CustomValidation *string  `json:"custom_validation"`
	Unique           *bool    `json:"unique"`
}

// TableFieldWrites are the settings both table-field mutations accept. A nil
// field is left out of the request.
type TableFieldWrites struct {
	Required         *bool
	Options          *[]string
	Description      *string
	Help             *string
	MinimalView      *bool
	CustomValidation *string
	Unique           *bool
}

func (w TableFieldWrites) addTo(vars map[string]any) {
	setIf(vars, "required", w.Required)
	setIf(vars, "options", w.Options)
	setIf(vars, "description", w.Description)
	setIf(vars, "help", w.Help)
	setIf(vars, "minimalView", w.MinimalView)
	setIf(vars, "customValidation", w.CustomValidation)
	setIf(vars, "unique", w.Unique)
}

// CreateTableFieldInput is the argument set of createTableField.
type CreateTableFieldInput struct {
	TableID string
	Type    string
	Label   string
	TableFieldWrites
}

// UpdateTableFieldInput is the argument set of updateTableField, which takes the
// table id alongside the field id.
type UpdateTableFieldInput struct {
	TableID string
	ID      string
	Label   *string
	TableFieldWrites
}

// TableFieldService reads and writes table fields.
type TableFieldService struct{ c *Client }

// Create adds a field to a table. It serializes on the table's own id: unlike a
// phase field, a table is already a top-level repo, so there is no parent repo
// to resolve first.
func (s *TableFieldService) Create(ctx context.Context, in CreateTableFieldInput) (TableField, error) {
	unlock := locks.LockRepo(in.TableID)
	defer unlock()

	vars := map[string]any{"tableId": in.TableID, "type": in.Type, "label": in.Label}
	in.TableFieldWrites.addTo(vars)

	var out struct {
		CreateTableField struct {
			TableField TableField `json:"table_field"`
		} `json:"createTableField"`
	}
	if err := s.c.do(ctx, "CreateTableField_tf", createTableFieldMutation, vars, &out); err != nil {
		return TableField{}, err
	}
	return out.CreateTableField.TableField, nil
}

// GetByUUID returns one field of a table. The UUID is the stable key: unlike the
// id, it survives a relabel.
func (s *TableFieldService) GetByUUID(ctx context.Context, tableID, uuid string) (TableField, error) {
	var out struct {
		Table *struct {
			TableFields []TableField `json:"table_fields"`
		} `json:"table"`
	}
	if err := s.c.do(ctx, "GetTableFields_tf", getTableFieldsQuery, map[string]any{"tableId": tableID}, &out); err != nil {
		return TableField{}, err
	}
	if out.Table == nil {
		return TableField{}, ErrNotFound
	}
	for _, field := range out.Table.TableFields {
		if field.UUID == uuid {
			return field, nil
		}
	}
	return TableField{}, ErrNotFound
}

// Update changes a field's label and settings. It takes no lock, unlike Create
// and Delete, matching the resource it replaces.
func (s *TableFieldService) Update(ctx context.Context, in UpdateTableFieldInput) (TableField, error) {
	vars := map[string]any{"id": in.ID, "tableId": in.TableID}
	setIf(vars, "label", in.Label)
	in.TableFieldWrites.addTo(vars)

	var out struct {
		UpdateTableField struct {
			TableField TableField `json:"table_field"`
		} `json:"updateTableField"`
	}
	if err := s.c.do(ctx, "UpdateTableField_tf", updateTableFieldMutation, vars, &out); err != nil {
		return TableField{}, err
	}
	return out.UpdateTableField.TableField, nil
}

// Delete removes a field, serializing on the table id the same way Create does.
// Unlike deletePhaseField this needs no pipe UUID, since a table has no parent
// pipe.
func (s *TableFieldService) Delete(ctx context.Context, tableID, fieldID string) error {
	unlock := locks.LockRepo(tableID)
	defer unlock()

	var out struct {
		DeleteTableField struct {
			Success bool `json:"success"`
		} `json:"deleteTableField"`
	}
	vars := map[string]any{"id": fieldID, "tableId": tableID}
	return s.c.do(ctx, "DeleteTableField_tf", deleteTableFieldMutation, vars, &out)
}
