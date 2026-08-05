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

func TestPhasesGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"phase":{
			"id":"900","name":"Inbox","done":false,"description":"first stop",
			"index":1.5,"lateness_time":3600,"can_receive_card_directly_from_draft":true,
			"repo_id":302825965
		}}}`)
	})

	phase, err := c.Phases.Get(t.Context(), "900")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if phase.ID != "900" || phase.Name != "Inbox" || phase.Done {
		t.Errorf("phase = %+v", phase)
	}
	if phase.RepoID != 302825965 {
		t.Errorf("RepoID = %d, want 302825965", phase.RepoID)
	}
	if phase.Index == nil || *phase.Index != 1.5 {
		t.Errorf("Index = %v, want 1.5", phase.Index)
	}
	if phase.LatenessTime == nil || *phase.LatenessTime != 3600 {
		t.Errorf("LatenessTime = %v, want 3600", phase.LatenessTime)
	}
	if phase.Description == nil || *phase.Description != "first stop" {
		t.Errorf("Description = %v", phase.Description)
	}
}

func TestPhasesGetMissingIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"phase":null}}`)
	})

	if _, err := c.Phases.Get(t.Context(), "900"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPhasesCreateOmitsUnsetFields(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"createPhase":{"phase":{"id":"900","name":"Inbox"}}}}`)
	})

	done := true
	in := CreatePhaseInput{PipeID: "301", Name: "Inbox", PhaseWrites: PhaseWrites{Done: &done}}
	if _, err := c.Phases.Create(t.Context(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Variables["pipeId"] != "301" || got.Variables["name"] != "Inbox" || got.Variables["done"] != true {
		t.Errorf("variables = %+v", got.Variables)
	}
	for _, absent := range []string{"description", "index", "latenessTime", "canReceiveCardDirectlyFromDraft"} {
		if _, present := got.Variables[absent]; present {
			t.Errorf("variables carries %q, want it omitted: %+v", absent, got.Variables)
		}
	}
}

// The API rejects concurrent phase creates for the same pipe. This asserts the
// observable contract rather than the mutex.
func TestPhasesCreateSerializesPerPipe(t *testing.T) {
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
		respondJSON(w, `{"data":{"createPhase":{"phase":{"id":"900","name":"Inbox"}}}}`)
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Phases.Create(t.Context(), CreatePhaseInput{PipeID: "301", Name: "Inbox"})
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if peak != 1 {
		t.Errorf("peak concurrent creates = %d, want 1", peak)
	}
}

// Deletes do not take the lock their creates take. The asymmetry is deliberate,
// so it is pinned here rather than left to be tidied away.
func TestPhasesDeleteDoesNotSerialize(t *testing.T) {
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
		respondJSON(w, `{"data":{"deletePhase":{"success":true}}}`)
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Phases.Delete(t.Context(), "900")
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if peak < 2 {
		t.Errorf("peak concurrent deletes = %d, want more than 1", peak)
	}
}

func TestPhasesDeleteReportsSuccessFlag(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"deletePhase":{"success":false}}}`)
	})

	ok, err := c.Phases.Delete(t.Context(), "900")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok {
		t.Error("Delete reported success, want false")
	}
}

func TestPhasesUpdate(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updatePhase":{"phase":{"id":"900","name":"Triage"}}}}`)
	})

	phase, err := c.Phases.Update(t.Context(), UpdatePhaseInput{ID: "900", Name: "Triage"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if phase.Name != "Triage" {
		t.Errorf("phase = %+v", phase)
	}
	if got.Variables["id"] != "900" || got.Variables["name"] != "Triage" {
		t.Errorf("variables = %+v", got.Variables)
	}
	// updatePhase takes no index: the API only accepts it at creation.
	if strings.Contains(got.Query, "$index") {
		t.Errorf("update document declares $index: %s", got.Query)
	}
}

func TestPhaseRepoID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"phase":{"repo_id":302825965}}}`)
	})

	repoID, err := c.phaseRepoID(t.Context(), "901")
	if err != nil {
		t.Fatalf("phaseRepoID: %v", err)
	}
	if repoID != "302825965" {
		t.Errorf("repoID = %q, want 302825965", repoID)
	}
}

// The three phaseRepoID outcomes are distinct because three callers render them
// differently. See the table in the plan's field condition task.
func TestPhaseRepoIDNilPhase(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"phase":null}}`)
	})

	_, err := c.phaseRepoID(t.Context(), "902")
	if !errors.Is(err, errPhaseUnresolved) {
		t.Fatalf("err = %v, want errPhaseUnresolved", err)
	}
	if err.Error() != "could not resolve phase from phase query" {
		t.Errorf("err.Error() = %q", err.Error())
	}
}

func TestPhaseRepoIDZeroIsDistinct(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"phase":{"repo_id":0}}}`)
	})

	_, err := c.phaseRepoID(t.Context(), "903")
	if !errors.Is(err, errPhaseRepoIDUnresolved) {
		t.Fatalf("err = %v, want errPhaseRepoIDUnresolved", err)
	}
	if err.Error() != "could not resolve valid phase repo_id from phase query" {
		t.Errorf("err.Error() = %q", err.Error())
	}
}

func TestPhaseRepoIDQueryFailureKeepsMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"errors":[{"message":"boom"}]}`)
	})

	_, err := c.phaseRepoID(t.Context(), "904")
	if errors.Is(err, errPhaseUnresolved) || errors.Is(err, errPhaseRepoIDUnresolved) {
		t.Fatalf("err = %v, want a query failure", err)
	}
	if err == nil || err.Error() != "failed to fetch phase repo_id: graphql error: boom" {
		t.Fatalf("err = %v", err)
	}
}
