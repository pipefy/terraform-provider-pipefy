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
			"id":"fc1","name":"Hide unless urgent","phase":{"id":"900"},
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
	if fc.Phase == nil || fc.Phase.ID != "900" {
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
		q := capture(t, r).Query
		if strings.Contains(q, "GetPhaseRepoId_tf") {
			respondJSON(w, `{"data":{"phase":{"repo_id":302825965}}}`)
			return
		}
		respondJSON(w, `{"data":{"createFieldCondition":{"fieldCondition":null}}}`)
	})

	_, err := c.FieldConditions.Create(t.Context(), "900", map[string]any{"name": "X"})
	if !errors.Is(err, ErrNoFieldCondition) {
		t.Fatalf("err = %v, want ErrNoFieldCondition", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, must not match ErrNotFound", err)
	}
}

func TestFieldConditionsUpdateNilPayloadIsNoFieldCondition(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := capture(t, r).Query
		if strings.Contains(q, "GetPhaseRepoId_tf") {
			respondJSON(w, `{"data":{"phase":{"repo_id":302825965}}}`)
			return
		}
		respondJSON(w, `{"data":{"updateFieldCondition":{"fieldCondition":null}}}`)
	})

	_, err := c.FieldConditions.Update(t.Context(), "900", map[string]any{"id": "fc1"})
	if !errors.Is(err, ErrNoFieldCondition) {
		t.Fatalf("err = %v, want ErrNoFieldCondition", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, must not match ErrNotFound", err)
	}
}

// fieldConditionPeak runs 8 concurrent calls against a handler that answers the
// repo lookup instantly and holds the mutation, then reports the peak overlap.
func fieldConditionPeak(t *testing.T, body string, call func(c *Client)) int {
	t.Helper()
	var mu sync.Mutex
	inFlight, peak := 0, 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(capture(t, r).Query, "GetPhaseRepoId_tf") {
			respondJSON(w, `{"data":{"phase":{"repo_id":302825965}}}`)
			return
		}
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
		_, _ = c.FieldConditions.Create(t.Context(), "900", map[string]any{"name": "X"})
	})
	if peak != 1 {
		t.Errorf("peak concurrent creates = %d, want 1", peak)
	}
}

// Unlike a phase field, a field condition's Update takes the lock too.
func TestFieldConditionsUpdateSerializesPerRepo(t *testing.T) {
	peak := fieldConditionPeak(t, `{"data":{"updateFieldCondition":{"fieldCondition":{"id":"fc1"}}}}`, func(c *Client) {
		_, _ = c.FieldConditions.Update(t.Context(), "900", map[string]any{"id": "fc1"})
	})
	if peak != 1 {
		t.Errorf("peak concurrent updates = %d, want 1", peak)
	}
}

func TestFieldConditionsDeleteSerializesPerRepo(t *testing.T) {
	peak := fieldConditionPeak(t, `{"data":{"deleteFieldCondition":{"success":true}}}`, func(c *Client) {
		_ = c.FieldConditions.Delete(t.Context(), "900", "fc1")
	})
	if peak != 1 {
		t.Errorf("peak concurrent deletes = %d, want 1", peak)
	}
}

// A condition whose phase was deleted out of band has nothing to lock, and must
// still be removable rather than stuck in state.
func TestFieldConditionsDeleteMissingPhaseStillDeletes(t *testing.T) {
	for name, repoBody := range map[string]string{
		"nil phase":    `{"data":{"phase":null}}`,
		"zero repo id": `{"data":{"phase":{"repo_id":0}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			deleted := false
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(capture(t, r).Query, "GetPhaseRepoId_tf") {
					respondJSON(w, repoBody)
					return
				}
				deleted = true
				respondJSON(w, `{"data":{"deleteFieldCondition":{"success":true}}}`)
			})

			if err := c.FieldConditions.Delete(t.Context(), "900", "fc1"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if !deleted {
				t.Error("the delete mutation never went out")
			}
		})
	}
}

// A failure of the lookup query itself is still an error, unlike an unresolvable
// phase.
func TestFieldConditionsDeletePhaseQueryFailureIsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"errors":[{"message":"boom"}]}`)
	})

	err := c.FieldConditions.Delete(t.Context(), "900", "fc1")
	if err == nil || !strings.HasPrefix(err.Error(), "failed to fetch phase repo_id: ") {
		t.Fatalf("err = %v", err)
	}
}

// Create and Update collapse both unresolvable-phase cases into one message,
// matching the resource they replace.
func TestFieldConditionsCreateCollapsesPhaseMessages(t *testing.T) {
	for name, body := range map[string]string{
		"nil phase":    `{"data":{"phase":null}}`,
		"zero repo id": `{"data":{"phase":{"repo_id":0}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, body)
			})
			_, err := c.FieldConditions.Create(t.Context(), "900", map[string]any{"name": "X"})
			if err == nil || err.Error() != "could not resolve valid phase repo_id from phase query" {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// The two mutations disagree on the phase key: create takes phaseId, update takes
// phase_id. The caller spells each, so this only pins that the SDK passes the map
// through untouched.
func TestFieldConditionsPassesInputThrough(t *testing.T) {
	var mutation capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got := capture(t, r)
		if strings.Contains(got.Query, "GetPhaseRepoId_tf") {
			respondJSON(w, `{"data":{"phase":{"repo_id":302825965}}}`)
			return
		}
		mutation = got
		respondJSON(w, `{"data":{"updateFieldCondition":{"fieldCondition":{"id":"fc1"}}}}`)
	})

	input := map[string]any{"id": "fc1", "phase_id": "900", "name": "X"}
	if _, err := c.FieldConditions.Update(t.Context(), "900", input); err != nil {
		t.Fatalf("Update: %v", err)
	}
	sent, ok := mutation.Variables["input"].(map[string]any)
	if !ok || sent["phase_id"] != "900" || sent["id"] != "fc1" {
		t.Errorf("variables = %+v", mutation.Variables)
	}
}
