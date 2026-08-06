// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestAutomationsGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"automation":{
			"id":"a1","name":"Nightly","active":true,
			"event_id":"card_scheduler","action_id":"create_card",
			"event_repo":{"id":"301"},"action_repo_v2":{"id":"tbl1"},
			"scheduler_frequency":"daily",
			"schedulerCron":{"minute":"0","hour":"9","dayOfMonth":"*","month":"*","dayOfWeek":"*"},
			"searchFor":[{"field":"status","id":"s1","operation":"equals","value":"open"}],
			"responseSchema":{"type":"object"},
			"condition":{"expressions":[{"structure_id":"0","field_address":"1001","operation":"equals","value":"x"}],
			             "expressions_structure":[["0"]]}
		}}}`)
	})

	a, err := c.Automations.Get(t.Context(), "a1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if a.ID != "a1" || a.Name != "Nightly" || a.EventID != "card_scheduler" || a.ActionID != "create_card" {
		t.Errorf("automation = %+v", a)
	}
	if a.EventRepo == nil || a.EventRepo.ID != "301" {
		t.Errorf("EventRepo = %+v", a.EventRepo)
	}
	if a.ActionRepoV2 == nil || a.ActionRepoV2.ID != "tbl1" {
		t.Errorf("ActionRepoV2 = %+v", a.ActionRepoV2)
	}
	if a.SchedulerCron == nil || a.SchedulerCron.Hour == nil || *a.SchedulerCron.Hour != "9" {
		t.Errorf("SchedulerCron = %+v", a.SchedulerCron)
	}
	if len(a.SearchFor) != 1 || a.SearchFor[0].ID != "s1" {
		t.Errorf("SearchFor = %+v", a.SearchFor)
	}
	if !strings.Contains(string(a.ResponseSchema), `"type"`) {
		t.Errorf("ResponseSchema = %s, want the raw bytes", a.ResponseSchema)
	}
	if a.Condition == nil || len(a.Condition.Expressions) != 1 {
		t.Errorf("Condition = %+v", a.Condition)
	}
}

// A non-scheduler automation comes back with an all-null cron and no search
// conditions, which the caller normalizes.
func TestAutomationsGetNonSchedulerShape(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"automation":{"id":"a1","name":"X",
			"scheduler_frequency":null,
			"schedulerCron":{"minute":null,"hour":null,"dayOfMonth":null,"month":null,"dayOfWeek":null},
			"searchFor":[],"responseSchema":null,"condition":null}}}`)
	})

	a, err := c.Automations.Get(t.Context(), "a1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if a.SchedulerFrequency != nil {
		t.Errorf("SchedulerFrequency = %v, want nil", a.SchedulerFrequency)
	}
	if a.SchedulerCron == nil || a.SchedulerCron.Minute != nil {
		t.Errorf("SchedulerCron = %+v, want an all-null cron object", a.SchedulerCron)
	}
	if len(a.SearchFor) != 0 {
		t.Errorf("SearchFor = %+v, want empty", a.SearchFor)
	}
}

// structure_id comes back as a number or a string depending on how the condition
// was written, so both have to decode.
func TestAutomationsConditionStructureIDStaysUntyped(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"automation":{"id":"a1","condition":{
			"expressions":[{"structure_id":0,"field_address":"1001","operation":"equals"},
			               {"structure_id":"1","field_address":"1002","operation":"equals"}],
			"expressions_structure":[[0,"1"]]}}}}`)
	})

	a, err := c.Automations.Get(t.Context(), "a1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(a.Condition.Expressions) != 2 {
		t.Fatalf("Expressions = %+v", a.Condition.Expressions)
	}
	if a.Condition.Expressions[0].StructureID != float64(0) {
		t.Errorf("numeric structure_id = %#v", a.Condition.Expressions[0].StructureID)
	}
	if a.Condition.Expressions[1].StructureID != "1" {
		t.Errorf("string structure_id = %#v", a.Condition.Expressions[1].StructureID)
	}
	if len(a.Condition.ExpressionsStructure) != 1 || len(a.Condition.ExpressionsStructure[0]) != 2 {
		t.Errorf("ExpressionsStructure = %#v", a.Condition.ExpressionsStructure)
	}
}

func TestAutomationsGetMissingIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"automation":null}}`)
	})

	if _, err := c.Automations.Get(t.Context(), "a1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAutomationsCreateReturnsValidationError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"createAutomation":{"automation":null,"error_details":[
			{"object_name":"field_map","object_key":"420173432","messages":["can't be blank","is invalid"]}
		]}}}`)
	})

	_, err := c.Automations.Create(t.Context(), map[string]any{"name": "X"})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if len(validationErr.Details) != 1 {
		t.Fatalf("Details = %+v", validationErr.Details)
	}
	d := validationErr.Details[0]
	// object_key has to survive as its own field: the rendered diagnostic reads
	// "field_map (420173432): can't be blank; is invalid".
	if d.ObjectName != "field_map" || d.ObjectKey != "420173432" || len(d.Messages) != 2 {
		t.Errorf("detail = %+v", d)
	}
}

// An automation in the payload wins even when the response also carried a
// top-level error. Reordering this changes which diagnostic a user sees.
func TestAutomationsCreatePrefersAutomationOverError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"createAutomation":{"automation":{"id":"a1","name":"X"},
			"error_details":[{"object_name":"noise","messages":["ignored"]}]}},
			"errors":[{"message":"also ignored"}]}`)
	})

	created, err := c.Automations.Create(t.Context(), map[string]any{"name": "X"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != "a1" {
		t.Errorf("created = %+v", created)
	}
}

func TestAutomationsCreateNoAutomationNoDetails(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"createAutomation":{"automation":null,"error_details":[]}}}`)
	})

	_, err := c.Automations.Create(t.Context(), map[string]any{"name": "X"})
	if !errors.Is(err, ErrNoAutomation) {
		t.Fatalf("err = %v, want ErrNoAutomation", err)
	}
	if err.Error() != "the API returned no automation and no error_details" {
		t.Errorf("err.Error() = %q", err.Error())
	}
}

func TestAutomationsCreateTransportErrorPassesThrough(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"errors":[{"message":"boom"}]}`)
	})

	_, err := c.Automations.Create(t.Context(), map[string]any{"name": "X"})
	if errors.Is(err, ErrNoAutomation) {
		t.Fatalf("err = %v, want the transport error", err)
	}
	if err == nil || err.Error() != "graphql error: boom" {
		t.Fatalf("err = %v", err)
	}
}

func TestAutomationsUpdateSucceedsOnAutomation(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"updateAutomation":{"automation":{"id":"a1"},"error_details":[]}}}`)
	})

	if err := c.Automations.Update(t.Context(), map[string]any{"id": "a1"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func TestAutomationsUpdateReturnsValidationError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"updateAutomation":{"automation":null,"error_details":[
			{"object_name":"condition","messages":["is invalid"]}
		]}}}`)
	})

	err := c.Automations.Update(t.Context(), map[string]any{"id": "a1"})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Op != "UpdateAutomation_tf" {
		t.Fatalf("err = %v, want *ValidationError for UpdateAutomation_tf", err)
	}
}

func TestAutomationsDelete(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"deleteAutomation":{"success":true}}}`)
	})

	if err := c.Automations.Delete(t.Context(), "a1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
