// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import "context"

const tableSelection = "id name description authorization color icon"

const createTableMutation = "mutation CreateTable_tf($name:String!,$orgId:ID!,$authorization:TableAuthorization," +
	"$description:String,$color:Colors,$icon:String){ createTable(input:{ name:$name, organization_id:$orgId, " +
	"authorization:$authorization, description:$description, color:$color, icon:$icon }){ table{ " +
	tableSelection + " } } }"

const getTableQuery = "query GetTable_tf($id:ID!){ table(id:$id){ " + tableSelection + " organization { id } } }"

const updateTableMutation = "mutation UpdateTable_tf($id:ID!,$name:String,$authorization:TableAuthorization," +
	"$description:String,$color:Colors,$icon:String){ updateTable(input:{ id:$id, name:$name, " +
	"authorization:$authorization, description:$description, color:$color, icon:$icon }){ table{ " +
	tableSelection + " } } }"

const deleteTableMutation = "mutation DeleteTable_tf($id:ID!){ deleteTable(input:{id:$id}){ success } }"

// Table authorization levels.
const (
	AuthorizationRead  = "read"
	AuthorizationWrite = "write"
)

// AuthorizationValues lists the table authorization levels.
var AuthorizationValues = []string{AuthorizationRead, AuthorizationWrite}

// Table is a database table. OrganizationID comes from the organization
// sub-selection, which only the read requests.
type Table struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   *string `json:"description"`
	Authorization *string `json:"authorization"`
	Color         *string `json:"color"`
	Icon          *string `json:"icon"`

	OrganizationID string `json:"-"`
}

// TableWrites are the settings both table mutations accept. A nil field is left
// out of the request.
type TableWrites struct {
	Description   *string
	Authorization *string
	Color         *string
	Icon          *string
}

func (w TableWrites) addTo(vars map[string]any) {
	setIf(vars, "description", w.Description)
	setIf(vars, "authorization", w.Authorization)
	setIf(vars, "color", w.Color)
	setIf(vars, "icon", w.Icon)
}

// CreateTableInput is the argument set of createTable.
type CreateTableInput struct {
	Name           string
	OrganizationID string
	TableWrites
}

// UpdateTableInput is the argument set of updateTable.
type UpdateTableInput struct {
	ID   string
	Name string
	TableWrites
}

// TableService reads and writes database tables.
type TableService struct{ c *Client }

// Create makes a table.
func (s *TableService) Create(ctx context.Context, in CreateTableInput) (Table, error) {
	vars := map[string]any{"name": in.Name, "orgId": in.OrganizationID}
	in.TableWrites.addTo(vars)

	var out struct {
		CreateTable struct {
			Table Table `json:"table"`
		} `json:"createTable"`
	}
	if err := s.c.do(ctx, "CreateTable_tf", createTableMutation, vars, &out); err != nil {
		return Table{}, err
	}
	return out.CreateTable.Table, nil
}

// Get returns a table and the id of the organization owning it.
func (s *TableService) Get(ctx context.Context, id string) (Table, error) {
	var out struct {
		Table *struct {
			Table
			Organization *struct {
				ID string `json:"id"`
			} `json:"organization"`
		} `json:"table"`
	}
	if err := s.c.do(ctx, "GetTable_tf", getTableQuery, map[string]any{"id": id}, &out); err != nil {
		return Table{}, err
	}
	if out.Table == nil {
		return Table{}, ErrNotFound
	}
	table := out.Table.Table
	if out.Table.Organization != nil {
		table.OrganizationID = out.Table.Organization.ID
	}
	return table, nil
}

// Update renames a table and applies its settings.
func (s *TableService) Update(ctx context.Context, in UpdateTableInput) (Table, error) {
	vars := map[string]any{"id": in.ID, "name": in.Name}
	in.TableWrites.addTo(vars)

	var out struct {
		UpdateTable struct {
			Table Table `json:"table"`
		} `json:"updateTable"`
	}
	if err := s.c.do(ctx, "UpdateTable_tf", updateTableMutation, vars, &out); err != nil {
		return Table{}, err
	}
	return out.UpdateTable.Table, nil
}

// Delete removes a table.
func (s *TableService) Delete(ctx context.Context, id string) error {
	var out struct {
		DeleteTable struct {
			Success bool `json:"success"`
		} `json:"deleteTable"`
	}
	return s.c.do(ctx, "DeleteTable_tf", deleteTableMutation, map[string]any{"id": id}, &out)
}
