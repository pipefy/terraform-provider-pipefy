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

func TestFieldConditionsGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"fieldCondition":{
			"id":"fc1","name":"Hide unless urgent","phase":{"id":"900","repo_id":123},
			"condition":{"expressions":[{"structure_id":"0","field_address":"1001","operation":"equals","value":"urgent"}],
			             "expressions_structure":[["0"]]},
			"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true},
			           {"actionId":"hide","phaseField":{"internal_id":"1002"},"whenEvaluator":false}]
		}}}`)
	})

	fc, err := c.FieldConditions.Get(t.Context(), "fc1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fc.ID != "fc1" || fc.Name != "Hide unless urgent" {
		t.Errorf("condition = %+v", fc)
	}
	if fc.Phase == nil || fc.Phase.ID != "900" || fc.Phase.PipeID() != "123" {
		t.Errorf("Phase = %+v", fc.Phase)
	}
	if len(fc.Actions) != 2 {
		t.Fatalf("Actions = %+v", fc.Actions)
	}
	if fc.Actions[0].ActionID != "show" || fc.Actions[0].PhaseField == nil || fc.Actions[0].PhaseField.InternalID != "1002" {
		t.Errorf("Actions[0] = %+v", fc.Actions[0])
	}
	if fc.Actions[1].WhenEvaluator == nil || *fc.Actions[1].WhenEvaluator {
		t.Errorf("Actions[1].WhenEvaluator = %v, want false", fc.Actions[1].WhenEvaluator)
	}
	if fc.Condition == nil || len(fc.Condition.Expressions) != 1 {
		t.Errorf("Condition = %+v", fc.Condition)
	}
}

func TestFieldConditionsGetMissingIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"fieldCondition":null}}`)
	})

	if _, err := c.FieldConditions.Get(t.Context(), "fc1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// A write that comes back without its object must not report ErrNotFound: a
// resource maps that to removing itself from state, which would discard a
// condition the API may well have created.
func TestFieldConditionsCreateNilPayloadIsNoFieldCondition(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"createFieldCondition":{"fieldCondition":null}}}`)
	})

	_, err := c.FieldConditions.Create(t.Context(), "123", map[string]any{"name": "X"})
	if !errors.Is(err, ErrNoFieldCondition) {
		t.Fatalf("err = %v, want ErrNoFieldCondition", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, must not match ErrNotFound", err)
	}
}

func TestFieldConditionsUpdateNilPayloadIsNoFieldCondition(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"updateFieldCondition":{"fieldCondition":null}}}`)
	})

	_, err := c.FieldConditions.Update(t.Context(), "123", map[string]any{"id": "fc1"})
	if !errors.Is(err, ErrNoFieldCondition) {
		t.Fatalf("err = %v, want ErrNoFieldCondition", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, must not match ErrNotFound", err)
	}
}

func TestFieldConditionsCreateEmptyPipeID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("Create with an empty pipe_id must not call the API")
	})
	_, err := c.FieldConditions.Create(t.Context(), "", map[string]any{"name": "X"})
	if err == nil || !strings.Contains(err.Error(), "empty pipe_id") {
		t.Fatalf("err = %v", err)
	}
}

// fieldConditionPeak runs 8 concurrent calls against a handler that holds the
// mutation, then reports the peak overlap.
func fieldConditionPeak(t *testing.T, body string, call func(c *Client)) int {
	t.Helper()
	var mu sync.Mutex
	inFlight, peak := 0, 0
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
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

func TestFieldConditionsCreateSerializesPerRepo(t *testing.T) {
	peak := fieldConditionPeak(t, `{"data":{"createFieldCondition":{"fieldCondition":{"id":"fc1"}}}}`, func(c *Client) {
		_, _ = c.FieldConditions.Create(t.Context(), "123", map[string]any{"name": "X"})
	})
	if peak != 1 {
		t.Errorf("peak concurrent creates = %d, want 1", peak)
	}
}

func TestFieldConditionsUpdateSerializesPerRepo(t *testing.T) {
	peak := fieldConditionPeak(t, `{"data":{"updateFieldCondition":{"fieldCondition":{"id":"fc1"}}}}`, func(c *Client) {
		_, _ = c.FieldConditions.Update(t.Context(), "123", map[string]any{"id": "fc1"})
	})
	if peak != 1 {
		t.Errorf("peak concurrent updates = %d, want 1", peak)
	}
}

func TestFieldConditionsDeleteSerializesPerRepo(t *testing.T) {
	peak := fieldConditionPeak(t, `{"data":{"deleteFieldCondition":{"success":true}}}`, func(c *Client) {
		_ = c.FieldConditions.Delete(t.Context(), "123", "fc1")
	})
	if peak != 1 {
		t.Errorf("peak concurrent deletes = %d, want 1", peak)
	}
}

func TestFieldConditionsDeleteEmptyPipeIDStillDeletes(t *testing.T) {
	deleted := false
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		deleted = true
		respondJSON(w, `{"data":{"deleteFieldCondition":{"success":true}}}`)
	})

	if err := c.FieldConditions.Delete(t.Context(), "", "fc1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Error("the delete mutation never went out")
	}
}

// The two mutations disagree on the phase key: create takes phaseId, update takes
// phase_id. The resource omits phase_id on update so the listing does not move,
// and this pins that a resource-shaped map is passed through without one.
func TestFieldConditionsPassesInputThrough(t *testing.T) {
	var mutation capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mutation = capture(t, r)
		respondJSON(w, `{"data":{"updateFieldCondition":{"fieldCondition":{"id":"fc1"}}}}`)
	})

	input := map[string]any{"id": "fc1", "name": "X"}
	if _, err := c.FieldConditions.Update(t.Context(), "123", input); err != nil {
		t.Fatalf("Update: %v", err)
	}
	sent, ok := mutation.Variables["input"].(map[string]any)
	if !ok || sent["id"] != "fc1" || sent["name"] != "X" {
		t.Errorf("variables = %+v", mutation.Variables)
	}
	if _, ok := sent["phase_id"]; ok {
		t.Errorf("Update sent phase_id: %+v", sent)
	}
}
