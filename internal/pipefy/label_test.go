// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestLabelsCreate(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"createLabel":{"label":{"id":"1","name":"Bug","color":"#FF0000"}}}}`)
	})

	label, err := c.Labels.Create(t.Context(), CreateLabelInput{PipeID: "301", Name: "Bug", Color: "#FF0000"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if label.ID != "1" || label.Name != "Bug" || label.Color != "#FF0000" {
		t.Errorf("label = %+v", label)
	}
	if !strings.Contains(got.Query, "mutation CreateLabel_tf") {
		t.Errorf("query = %q", got.Query)
	}
	if got.Variables["pipeId"] != "301" || got.Variables["name"] != "Bug" || got.Variables["color"] != "#FF0000" {
		t.Errorf("variables = %+v", got.Variables)
	}
}

func TestLabelsGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"labels":[
			{"id":"other","name":"X","color":"#000000"},
			{"id":"1","name":"Bug","color":"#FF0000"}
		]}}}`)
	})

	label, err := c.Labels.Get(t.Context(), "301", "1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if label.Name != "Bug" || label.Color != "#FF0000" {
		t.Errorf("label = %+v", label)
	}
}

func TestLabelsGetMissingLabelIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"labels":[{"id":"other","name":"X","color":"#000000"}]}}}`)
	})

	_, err := c.Labels.Get(t.Context(), "301", "1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLabelsGetMissingPipeIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":null}}`)
	})

	_, err := c.Labels.Get(t.Context(), "301", "1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// A GraphQL errors array is not a not-found signal. Resources hard-fail on it
// today and must keep doing so.
func TestLabelsGetGraphQLErrorIsNotNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"errors":[{"message":"Name can't be blank"},{"message":"nope"}]}`)
	})

	_, err := c.Labels.Get(t.Context(), "301", "1")
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want a plain error", err)
	}
	if err == nil || err.Error() != "graphql error: Name can't be blank; nope" {
		t.Fatalf("err = %v, want the transport's rendering unchanged", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Op != "GetPipeLabels_tf" {
		t.Fatalf("err = %v, want *APIError tagged GetPipeLabels_tf", err)
	}
}

func TestLabelsUpdate(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updateLabel":{"label":{"id":"1","name":"Defect","color":"#00FF00"}}}}`)
	})

	label, err := c.Labels.Update(t.Context(), UpdateLabelInput{ID: "1", Name: "Defect", Color: "#00FF00"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if label.Name != "Defect" || label.Color != "#00FF00" {
		t.Errorf("label = %+v", label)
	}
	if got.Variables["id"] != "1" {
		t.Errorf("variables = %+v", got.Variables)
	}
}

func TestLabelsDelete(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"deleteLabel":{"success":true}}}`)
	})

	if err := c.Labels.Delete(t.Context(), "1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got.Variables["id"] != "1" {
		t.Errorf("variables = %+v", got.Variables)
	}
}
