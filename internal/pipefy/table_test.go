// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"errors"
	"net/http"
	"testing"
)

func TestTablesGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"table":{
			"id":"tbl1","name":"Vendors","description":"d","authorization":"write",
			"color":"blue","icon":"table","organization":{"id":"28"}
		}}}`)
	})

	table, err := c.Tables.Get(t.Context(), "tbl1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if table.ID != "tbl1" || table.Name != "Vendors" || table.OrganizationID != "28" {
		t.Errorf("table = %+v", table)
	}
	if table.Authorization == nil || *table.Authorization != AuthorizationWrite {
		t.Errorf("Authorization = %v", table.Authorization)
	}
	if table.Description == nil || *table.Description != "d" {
		t.Errorf("Description = %v", table.Description)
	}
}

func TestTablesGetMissingIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"table":null}}`)
	})

	if _, err := c.Tables.Get(t.Context(), "tbl1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestTablesCreateOmitsUnsetFields(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"createTable":{"table":{"id":"tbl1","name":"Vendors"}}}}`)
	})

	auth := AuthorizationRead
	in := CreateTableInput{Name: "Vendors", OrganizationID: "28", TableWrites: TableWrites{Authorization: &auth}}
	if _, err := c.Tables.Create(t.Context(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Variables["name"] != "Vendors" || got.Variables["orgId"] != "28" || got.Variables["authorization"] != "read" {
		t.Errorf("variables = %+v", got.Variables)
	}
	for _, absent := range []string{"description", "color", "icon"} {
		if _, present := got.Variables[absent]; present {
			t.Errorf("variables carries %q, want it omitted: %+v", absent, got.Variables)
		}
	}
}

func TestTablesUpdateSendsName(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updateTable":{"table":{"id":"tbl1","name":"Suppliers"}}}}`)
	})

	table, err := c.Tables.Update(t.Context(), UpdateTableInput{ID: "tbl1", Name: "Suppliers"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if table.Name != "Suppliers" {
		t.Errorf("table = %+v", table)
	}
	if got.Variables["id"] != "tbl1" || got.Variables["name"] != "Suppliers" {
		t.Errorf("variables = %+v", got.Variables)
	}
}

func TestTablesDelete(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"deleteTable":{"success":true}}}`)
	})

	if err := c.Tables.Delete(t.Context(), "tbl1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
