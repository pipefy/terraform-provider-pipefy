// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

// peakConcurrency runs call 8 times at once against a handler that holds each
// request briefly, and reports the highest number in flight together.
func peakConcurrency(t *testing.T, body string, call func(c *Client)) int {
	t.Helper()
	var mu sync.Mutex
	inFlight, peak := 0, 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		respondJSON(w, body)
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			call(c)
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	return peak
}

func TestTableFieldsGetByUUID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"table":{"table_fields":[
			{"id":"other","uuid":"u0"},
			{"id":"slug","internal_id":"i1","uuid":"u1","label":"Email","type":"email",
			 "required":true,"options":["a"],"description":"d","help":"h",
			 "minimal_view":false,"custom_validation":"rx","unique":true}
		]}}}`)
	})

	field, err := c.TableFields.GetByUUID(t.Context(), "tbl1", "u1")
	if err != nil {
		t.Fatalf("GetByUUID: %v", err)
	}
	if field.ID != "slug" || field.InternalID != "i1" || field.Label != "Email" || field.Type != "email" {
		t.Errorf("field = %+v", field)
	}
	if field.Unique == nil || !*field.Unique {
		t.Errorf("Unique = %v, want true", field.Unique)
	}
}

func TestTableFieldsGetByUUIDMissingFieldIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"table":{"table_fields":[{"id":"other","uuid":"u0"}]}}}`)
	})

	if _, err := c.TableFields.GetByUUID(t.Context(), "tbl1", "u1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestTableFieldsGetByUUIDMissingTableIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"table":null}}`)
	})

	if _, err := c.TableFields.GetByUUID(t.Context(), "tbl1", "u1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestTableFieldsCreateOmitsUnsetWrites(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"createTableField":{"table_field":{"id":"slug","uuid":"u1"}}}}`)
	})

	unique := true
	in := CreateTableFieldInput{
		TableID:          "tbl1",
		Type:             "email",
		Label:            "Email",
		TableFieldWrites: TableFieldWrites{Unique: &unique},
	}
	if _, err := c.TableFields.Create(t.Context(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Variables["tableId"] != "tbl1" || got.Variables["type"] != "email" || got.Variables["unique"] != true {
		t.Errorf("variables = %+v", got.Variables)
	}
	for _, absent := range []string{"required", "options", "description", "help", "minimalView", "customValidation"} {
		if _, present := got.Variables[absent]; present {
			t.Errorf("variables carries %q, want it omitted: %+v", absent, got.Variables)
		}
	}
}

func TestTableFieldsUpdateSendsTableID(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updateTableField":{"table_field":{"id":"slug","label":"Work email"}}}}`)
	})

	label := "Work email"
	field, err := c.TableFields.Update(t.Context(), UpdateTableFieldInput{TableID: "tbl1", ID: "slug", Label: &label})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if field.Label != "Work email" {
		t.Errorf("field = %+v", field)
	}
	if got.Variables["id"] != "slug" || got.Variables["tableId"] != "tbl1" || got.Variables["label"] != "Work email" {
		t.Errorf("variables = %+v", got.Variables)
	}
}

func TestTableFieldsCreateSerializesPerTable(t *testing.T) {
	peak := peakConcurrency(t, `{"data":{"createTableField":{"table_field":{"id":"slug"}}}}`, func(c *Client) {
		_, _ = c.TableFields.Create(t.Context(), CreateTableFieldInput{TableID: "tbl1", Type: "email", Label: "Email"})
	})
	if peak != 1 {
		t.Errorf("peak concurrent creates = %d, want 1", peak)
	}
}

func TestTableFieldsDeleteSerializesPerTable(t *testing.T) {
	peak := peakConcurrency(t, `{"data":{"deleteTableField":{"success":true}}}`, func(c *Client) {
		_ = c.TableFields.Delete(t.Context(), "tbl1", "slug")
	})
	if peak != 1 {
		t.Errorf("peak concurrent deletes = %d, want 1", peak)
	}
}

// Update does not serialize, unlike Create and Delete. The asymmetry is
// deliberate, so it is pinned here rather than left to be tidied away.
func TestTableFieldsUpdateDoesNotSerialize(t *testing.T) {
	peak := peakConcurrency(t, `{"data":{"updateTableField":{"table_field":{"id":"slug"}}}}`, func(c *Client) {
		_, _ = c.TableFields.Update(t.Context(), UpdateTableFieldInput{TableID: "tbl1", ID: "slug"})
	})
	if peak < 2 {
		t.Errorf("peak concurrent updates = %d, want more than 1", peak)
	}
}

func TestTableFieldsDeleteSendsTableID(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"deleteTableField":{"success":true}}}`)
	})

	if err := c.TableFields.Delete(t.Context(), "tbl1", "slug"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got.Variables["id"] != "slug" || got.Variables["tableId"] != "tbl1" {
		t.Errorf("variables = %+v", got.Variables)
	}
}
