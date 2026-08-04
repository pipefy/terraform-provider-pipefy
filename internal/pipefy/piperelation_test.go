// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"errors"
	"net/http"
	"testing"
)

func TestPipeRelationsGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"childrenRelations":[
			{"id":"other","name":"X"},
			{"id":"rel1","name":"Tasks","canCreateNewItems":true,"canConnectExistingItems":false,
			 "autoFillFieldEnabled":true,
			 "parent":{"id":"301"},"child":{"id":"tbl1"},
			 "ownFieldMaps":[{"fieldId":"f1","inputMode":"copy","value":"v1"},
			                 {"fieldId":"f2","inputMode":"fixed","value":"v2"}]}
		]}}}`)
	})

	rel, err := c.PipeRelations.Get(t.Context(), "301", "rel1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rel.Name != "Tasks" {
		t.Errorf("relation = %+v", rel)
	}
	// The inline fragments have to resolve to plain ids on both ends.
	if rel.Parent == nil || rel.Parent.ID != "301" {
		t.Errorf("Parent = %+v, want id 301", rel.Parent)
	}
	if rel.Child == nil || rel.Child.ID != "tbl1" {
		t.Errorf("Child = %+v, want id tbl1", rel.Child)
	}
	if len(rel.OwnFieldMaps) != 2 || rel.OwnFieldMaps[0].FieldID != "f1" || rel.OwnFieldMaps[1].InputMode != "fixed" {
		t.Errorf("OwnFieldMaps = %+v", rel.OwnFieldMaps)
	}
	if rel.CanCreateNewItems == nil || !*rel.CanCreateNewItems {
		t.Errorf("CanCreateNewItems = %v, want true", rel.CanCreateNewItems)
	}
	if rel.CanConnectExistingItems == nil || *rel.CanConnectExistingItems {
		t.Errorf("CanConnectExistingItems = %v, want false", rel.CanConnectExistingItems)
	}
}

func TestPipeRelationsGetMissingRelationIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"childrenRelations":[{"id":"other"}]}}}`)
	})

	if _, err := c.PipeRelations.Get(t.Context(), "301", "rel1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPipeRelationsGetMissingPipeIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":null}}`)
	})

	if _, err := c.PipeRelations.Get(t.Context(), "301", "rel1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPipeRelationsCreateReturnsID(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"createPipeRelation":{"pipeRelation":{"id":"rel1"}}}}`)
	})

	input := map[string]any{"parentId": "301", "childId": "tbl1", "name": "Tasks"}
	id, err := c.PipeRelations.Create(t.Context(), input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != "rel1" {
		t.Errorf("id = %q, want rel1", id)
	}
	sent, ok := got.Variables["input"].(map[string]any)
	if !ok || sent["parentId"] != "301" || sent["childId"] != "tbl1" {
		t.Errorf("variables = %+v", got.Variables)
	}
}

// updatePipeRelation replaces ownFieldMaps wholesale, so the full list has to
// reach the wire rather than a diff of it.
func TestPipeRelationsUpdateSendsFullFieldMaps(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updatePipeRelation":{"pipeRelation":{"id":"rel1"}}}}`)
	})

	maps := []map[string]any{
		{"fieldId": "f1", "inputMode": "copy", "value": "v1"},
		{"fieldId": "f2", "inputMode": "fixed", "value": "v2"},
	}
	input := map[string]any{"id": "rel1", "ownFieldMaps": maps}
	if err := c.PipeRelations.Update(t.Context(), input); err != nil {
		t.Fatalf("Update: %v", err)
	}
	sent, ok := got.Variables["input"].(map[string]any)
	if !ok {
		t.Fatalf("variables = %+v", got.Variables)
	}
	list, ok := sent["ownFieldMaps"].([]any)
	if !ok || len(list) != 2 {
		t.Errorf("ownFieldMaps = %#v, want both entries", sent["ownFieldMaps"])
	}
}

func TestPipeRelationsDelete(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"deletePipeRelation":{"success":true}}}`)
	})

	if err := c.PipeRelations.Delete(t.Context(), "rel1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
