// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// fieldHandler answers the phase repo lookup and the pipe UUID lookup, then
// hands anything else to next. Every field mutation needs at least one of the
// two lookups first.
func fieldHandler(t *testing.T, next func(w http.ResponseWriter, q string)) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		q := capture(t, r).Query
		switch {
		case strings.Contains(q, "GetPhaseRepoId_tf"):
			respondJSON(w, `{"data":{"phase":{"repo_id":302825965}}}`)
		case strings.Contains(q, "GetPipeUuid_tf"):
			respondJSON(w, `{"data":{"pipe":{"uuid":"pipe-uuid"}}}`)
		default:
			next(w, q)
		}
	}
}

func TestFieldsGetByUUID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"phase":{"fields":[
			{"id":"other","internal_id":"i0","uuid":"u0","label":"X","type":"short_text"},
			{"id":"slug","internal_id":"i1","uuid":"u1","label":"Amount","type":"number",
			 "required":true,"options":["a","b"],"description":"d","help":"h",
			 "editable":false,"minimal_view":true,"custom_validation":"","index":2.5}
		]}}}`)
	})

	field, err := c.Fields.GetByUUID(t.Context(), "900", "u1")
	if err != nil {
		t.Fatalf("GetByUUID: %v", err)
	}
	if field.ID != "slug" || field.InternalID != "i1" || field.Label != "Amount" || field.Type != "number" {
		t.Errorf("field = %+v", field)
	}
	if field.Index == nil || *field.Index != 2.5 {
		t.Errorf("Index = %v, want 2.5", field.Index)
	}
	if strings.Join(field.Options, ",") != "a,b" {
		t.Errorf("Options = %v", field.Options)
	}
	// An empty custom_validation is distinct from an absent one, so it stays a
	// pointer to the empty string rather than becoming nil.
	if field.CustomValidation == nil || *field.CustomValidation != "" {
		t.Errorf("CustomValidation = %v, want a pointer to the empty string", field.CustomValidation)
	}
}

func TestFieldsGetByUUIDMissingFieldIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"phase":{"fields":[{"id":"other","uuid":"u0"}]}}}`)
	})

	if _, err := c.Fields.GetByUUID(t.Context(), "900", "u1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestFieldsGetByUUIDMissingPhaseIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"phase":null}}`)
	})

	if _, err := c.Fields.GetByUUID(t.Context(), "900", "u1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestFieldsCreateOmitsUnsetWrites(t *testing.T) {
	var mutation capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got := capture(t, r)
		if strings.Contains(got.Query, "GetPhaseRepoId_tf") {
			respondJSON(w, `{"data":{"phase":{"repo_id":302825965}}}`)
			return
		}
		mutation = got
		respondJSON(w, `{"data":{"createPhaseField":{"phase_field":{"id":"slug","uuid":"u1"}}}}`)
	})

	required := true
	in := CreateFieldInput{
		PhaseID:     "900",
		Type:        "number",
		Label:       "Amount",
		FieldWrites: FieldWrites{Required: &required},
	}
	if _, err := c.Fields.Create(t.Context(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if mutation.Variables["phaseId"] != "900" || mutation.Variables["type"] != "number" || mutation.Variables["required"] != true {
		t.Errorf("variables = %+v", mutation.Variables)
	}
	for _, absent := range []string{"options", "description", "help", "editable", "minimalView", "customValidation", "index"} {
		if _, present := mutation.Variables[absent]; present {
			t.Errorf("variables carries %q, want it omitted: %+v", absent, mutation.Variables)
		}
	}
}

// An empty options list is sent, unlike an unset one, so a field's choices can be
// cleared.
func TestFieldsCreateSendsEmptyOptions(t *testing.T) {
	var mutation capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got := capture(t, r)
		if strings.Contains(got.Query, "GetPhaseRepoId_tf") {
			respondJSON(w, `{"data":{"phase":{"repo_id":302825965}}}`)
			return
		}
		mutation = got
		respondJSON(w, `{"data":{"createPhaseField":{"phase_field":{"id":"slug"}}}}`)
	})

	empty := []string{}
	in := CreateFieldInput{PhaseID: "900", Type: "select", Label: "Pick", FieldWrites: FieldWrites{Options: &empty}}
	if _, err := c.Fields.Create(t.Context(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	options, present := mutation.Variables["options"]
	if !present {
		t.Fatalf("variables omitted options: %+v", mutation.Variables)
	}
	if list, ok := options.([]any); !ok || len(list) != 0 {
		t.Errorf("options = %#v, want an empty list", options)
	}
}

func TestFieldsCreateSerializesPerRepo(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	c := newTestClient(t, fieldHandler(t, func(w http.ResponseWriter, q string) {
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
		respondJSON(w, `{"data":{"createPhaseField":{"phase_field":{"id":"slug"}}}}`)
	}))

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Fields.Create(t.Context(), CreateFieldInput{PhaseID: "900", Type: "number", Label: "Amount"})
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if peak != 1 {
		t.Errorf("peak concurrent creates = %d, want 1", peak)
	}
}

// Create renders one message whether the phase is missing or its repo id is
// zero, which is what the resource did.
func TestFieldsCreatePhaseLookupCollapsesMessages(t *testing.T) {
	for name, body := range map[string]string{
		"nil phase":    `{"data":{"phase":null}}`,
		"zero repo id": `{"data":{"phase":{"repo_id":0}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, body)
			})
			_, err := c.Fields.Create(t.Context(), CreateFieldInput{PhaseID: "900", Type: "number", Label: "Amount"})
			if err == nil || err.Error() != "could not resolve valid phase repo_id from phase query" {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestFieldsCreatePhaseQueryFailureKeepsMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"errors":[{"message":"boom"}]}`)
	})

	_, err := c.Fields.Create(t.Context(), CreateFieldInput{PhaseID: "900", Type: "number", Label: "Amount"})
	if err == nil || !strings.HasPrefix(err.Error(), "failed to fetch phase repo_id: ") {
		t.Fatalf("err = %v", err)
	}
}

// Delete keeps the two lookup failures distinct, unlike Create.
func TestFieldsDeleteKeepsPhaseMessagesDistinct(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"nil phase":    {`{"data":{"phase":null}}`, "could not resolve phase from phase query"},
		"zero repo id": {`{"data":{"phase":{"repo_id":0}}}`, "could not resolve valid phase repo_id from phase query"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			api := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, c.body)
			})
			err := api.Fields.Delete(t.Context(), "900", "slug")
			if err == nil || err.Error() != c.want {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}

func TestFieldsDeleteResolvesPipeUUID(t *testing.T) {
	var order []string
	var mutation capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got := capture(t, r)
		switch {
		case strings.Contains(got.Query, "GetPhaseRepoId_tf"):
			order = append(order, "repo")
			respondJSON(w, `{"data":{"phase":{"repo_id":302825965}}}`)
		case strings.Contains(got.Query, "GetPipeUuid_tf"):
			order = append(order, "uuid")
			if got.Variables["id"] != "302825965" {
				t.Errorf("pipe UUID lookup got id %v, want the phase's repo_id", got.Variables["id"])
			}
			respondJSON(w, `{"data":{"pipe":{"uuid":"pipe-uuid"}}}`)
		default:
			order = append(order, "delete")
			mutation = got
			respondJSON(w, `{"data":{"deletePhaseField":{"success":true}}}`)
		}
	})

	if err := c.Fields.Delete(t.Context(), "900", "slug"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if strings.Join(order, ",") != "repo,uuid,delete" {
		t.Errorf("request order = %v, want repo,uuid,delete", order)
	}
	if mutation.Variables["id"] != "slug" || mutation.Variables["pipeUuid"] != "pipe-uuid" {
		t.Errorf("variables = %+v", mutation.Variables)
	}
}

func TestFieldsUpdate(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updatePhaseField":{"phase_field":{"id":"slug","uuid":"u1","label":"Total"}}}}`)
	})

	label := "Total"
	field, err := c.Fields.Update(t.Context(), UpdateFieldInput{ID: "slug", UUID: "u1", Label: &label})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if field.Label != "Total" {
		t.Errorf("field = %+v", field)
	}
	if got.Variables["id"] != "slug" || got.Variables["uuid"] != "u1" || got.Variables["label"] != "Total" {
		t.Errorf("variables = %+v", got.Variables)
	}
}

// Update takes no lock, so concurrent updates are not serialized. Pinned so a
// later change to that has to be deliberate.
func TestFieldsUpdateDoesNotSerialize(t *testing.T) {
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
		respondJSON(w, `{"data":{"updatePhaseField":{"phase_field":{"id":"slug"}}}}`)
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Fields.Update(t.Context(), UpdateFieldInput{ID: "slug", UUID: "u1"})
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if peak < 2 {
		t.Errorf("peak concurrent updates = %d, want more than 1", peak)
	}
}
