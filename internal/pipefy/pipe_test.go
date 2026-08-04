// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

const pipePayloadJSON = `{
	"id":"301","name":"Ops","public":true,"icon":"pipefy","color":"blue",
	"only_admin_can_remove_cards":false,"only_assignees_can_edit_cards":true,
	"expiration_time_by_unit":3,"expiration_unit":3600,
	"startFormPhaseId":"900",
	"preferences":{"inboxEmailEnabled":true,"mainTabViews":["kanban","table"]},
	"organization":{"id":"28"}
}`

func TestPipesGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":`+pipePayloadJSON+`}}`)
	})

	pipe, err := c.Pipes.Get(t.Context(), "301")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if pipe.ID != "301" || pipe.Name != "Ops" || pipe.StartFormPhaseID != "900" {
		t.Errorf("pipe = %+v", pipe)
	}
	if pipe.OrganizationID != "28" {
		t.Errorf("OrganizationID = %q, want 28", pipe.OrganizationID)
	}
	if pipe.Public == nil || !*pipe.Public {
		t.Errorf("Public = %v, want true", pipe.Public)
	}
	if pipe.OnlyAdminCanRemoveCards == nil || *pipe.OnlyAdminCanRemoveCards {
		t.Errorf("OnlyAdminCanRemoveCards = %v, want false", pipe.OnlyAdminCanRemoveCards)
	}
	if pipe.Preferences == nil || len(pipe.Preferences.MainTabViews) != 2 {
		t.Errorf("Preferences = %+v", pipe.Preferences)
	}
	count, unit, ok := pipe.SLA()
	if !ok || count != 3 || unit != UnitHours {
		t.Errorf("SLA() = (%d,%q,%v), want (3,hours,true)", count, unit, ok)
	}
}

func TestPipesGetMissingIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":null}}`)
	})

	if _, err := c.Pipes.Get(t.Context(), "301"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPipesGetWithPhaseIDs(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"id":"301","name":"Ops","phases":[{"id":"1"},{"id":"2"},{"id":"3"}]}}}`)
	})

	pipe, phaseIDs, err := c.Pipes.GetWithPhaseIDs(t.Context(), "301")
	if err != nil {
		t.Fatalf("GetWithPhaseIDs: %v", err)
	}
	if pipe.Name != "Ops" {
		t.Errorf("pipe = %+v", pipe)
	}
	if strings.Join(phaseIDs, ",") != "1,2,3" {
		t.Errorf("phaseIDs = %v, want [1 2 3]", phaseIDs)
	}
}

func TestPipesGetWithPhaseIDsMissingIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":null}}`)
	})

	if _, _, err := c.Pipes.GetWithPhaseIDs(t.Context(), "301"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPipesCreateReturnsID(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"createPipe":{"pipe":{"id":"301","name":"Ops"}}}}`)
	})

	id, err := c.Pipes.Create(t.Context(), CreatePipeInput{Name: "Ops", OrganizationID: "28"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != "301" {
		t.Errorf("id = %q, want 301", id)
	}
	if got.Variables["name"] != "Ops" || got.Variables["orgId"] != "28" {
		t.Errorf("variables = %+v", got.Variables)
	}
}

// An omitted attribute must not reach the wire, so an unset Optional+Computed
// attribute keeps its server value instead of being cleared.
func TestPipesUpdateOmitsUnsetFields(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updatePipe":{"pipe":`+pipePayloadJSON+`}}}`)
	})

	name := "Ops"
	public := true
	if _, err := c.Pipes.Update(t.Context(), UpdatePipeInput{ID: "301", Name: &name, Public: &public}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Variables["id"] != "301" || got.Variables["name"] != "Ops" || got.Variables["public"] != true {
		t.Errorf("variables = %+v", got.Variables)
	}
	for _, absent := range []string{"icon", "color", "onlyAdminCanRemoveCards", "onlyAssigneesCanEditCards", "expirationTimeByUnit", "expirationUnit", "preferences"} {
		if _, present := got.Variables[absent]; present {
			t.Errorf("variables carries %q, want it omitted: %+v", absent, got.Variables)
		}
	}
}

func TestPipesUpdateSendsPreferencesAndSLA(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updatePipe":{"pipe":`+pipePayloadJSON+`}}}`)
	})

	count := int64(3)
	seconds := int64(3600)
	in := UpdatePipeInput{
		ID:                   "301",
		ExpirationTimeByUnit: &count,
		ExpirationUnit:       &seconds,
		Preferences:          map[string]any{"inboxEmailEnabled": true},
	}
	if _, err := c.Pipes.Update(t.Context(), in); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Variables["expirationTimeByUnit"] != float64(3) || got.Variables["expirationUnit"] != float64(3600) {
		t.Errorf("variables = %+v", got.Variables)
	}
	prefs, ok := got.Variables["preferences"].(map[string]any)
	if !ok || prefs["inboxEmailEnabled"] != true {
		t.Errorf("preferences = %+v", got.Variables["preferences"])
	}
}

func TestPipesDelete(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"deletePipe":{"success":true}}}`)
	})

	if err := c.Pipes.Delete(t.Context(), "301"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestPipesUUID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"uuid":"abc-123"}}}`)
	})

	uuid, err := c.Pipes.UUID(t.Context(), "301")
	if err != nil {
		t.Fatalf("UUID: %v", err)
	}
	if uuid != "abc-123" {
		t.Errorf("uuid = %q, want abc-123", uuid)
	}
}

// The two UUID failure messages reach Terraform diagnostics that
// resource_ai_agent_failures_test.go matches on, so they are pinned here.
func TestPipesUUIDEmptyKeepsMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"uuid":""}}}`)
	})

	_, err := c.Pipes.UUID(t.Context(), "301")
	if err == nil || err.Error() != `resolve pipe "301" UUID: expected a non-empty pipe.uuid` {
		t.Fatalf("err = %v", err)
	}
}

func TestPipesUUIDErrorKeepsMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"errors":[{"message":"boom"}]}`)
	})

	_, err := c.Pipes.UUID(t.Context(), "301")
	if err == nil || err.Error() != `resolve pipe "301" UUID: graphql error: boom` {
		t.Fatalf("err = %v", err)
	}
}

func TestPipeSLAUnitRoundTrip(t *testing.T) {
	cases := map[string]int64{UnitMinutes: 60, UnitHours: 3600, UnitDays: 86400}
	for name, seconds := range cases {
		got, ok := UnitNameToSeconds(name)
		if !ok || got != seconds {
			t.Errorf("UnitNameToSeconds(%q) = (%d,%v), want (%d,true)", name, got, ok, seconds)
		}
		back, ok := UnitSecondsToName(seconds)
		if !ok || back != name {
			t.Errorf("UnitSecondsToName(%d) = (%q,%v), want (%q,true)", seconds, back, ok, name)
		}
	}
	if _, ok := UnitNameToSeconds("weeks"); ok {
		t.Error(`UnitNameToSeconds("weeks") reported known`)
	}
	if _, ok := UnitSecondsToName(1); ok {
		t.Error("UnitSecondsToName(1) reported known")
	}
}

func TestPipeValidDuration(t *testing.T) {
	cases := []struct {
		unit  string
		count int64
		want  bool
	}{
		{UnitMinutes, 1, true}, {UnitMinutes, 59, true}, {UnitMinutes, 60, false},
		{UnitHours, 1, true}, {UnitHours, 23, true}, {UnitHours, 24, false},
		{UnitDays, 1, true}, {UnitDays, 3650, true},
		{UnitDays, 0, false}, {UnitMinutes, 0, false}, {"weeks", 1, false},
	}
	for _, c := range cases {
		if got := ValidDuration(c.unit, c.count); got != c.want {
			t.Errorf("ValidDuration(%q,%d) = %v, want %v", c.unit, c.count, got, c.want)
		}
	}
}

func TestPipeSLAUnsetIsNotOK(t *testing.T) {
	if _, _, ok := (Pipe{}).SLA(); ok {
		t.Error("SLA() on a zero pipe reported ok")
	}
	seconds := int64(7)
	count := int64(1)
	if _, _, ok := (Pipe{ExpirationUnit: &seconds, ExpirationTimeByUnit: &count}).SLA(); ok {
		t.Error("SLA() with an unmappable unit reported ok")
	}
}
