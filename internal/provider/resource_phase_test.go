// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

type phaseState struct {
	ID              string
	Name            string
	Done            bool
	Description     *string
	Index           *float64
	Color           *string
	LatenessTime    *int64
	CanReceiveDraft *bool
	Deleted         bool
	FailUpdate      bool
	DeletedCt       int
	CreateSawColor  bool
	UpdateSawIndex  bool
}

func (st *phaseState) toJSON() string {
	b, _ := json.Marshal(map[string]any{
		"id":                                   st.ID,
		"name":                                 st.Name,
		"done":                                 st.Done,
		"description":                          st.Description,
		"index":                                st.Index,
		"color":                                st.Color,
		"lateness_time":                        st.LatenessTime,
		"can_receive_card_directly_from_draft": st.CanReceiveDraft,
		"repo_id":                              301,
	})
	return string(b)
}

func newPhaseServer(st *phaseState) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testtoken" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"errors":[{"message":"unauthorized"}]}`)
			return
		}
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")

		merge := func() {
			if v, ok := gr.Variables["name"].(string); ok {
				st.Name = v
			}
			if v, ok := gr.Variables["done"].(bool); ok {
				st.Done = v
			}
			if v, ok := gr.Variables["description"].(string); ok {
				st.Description = &v
			}
			if v, ok := gr.Variables["latenessTime"].(float64); ok {
				lt := int64(v)
				st.LatenessTime = &lt
			}
			if v, ok := gr.Variables["canReceiveCardDirectlyFromDraft"].(bool); ok {
				st.CanReceiveDraft = &v
			}
		}

		switch q := gr.Query; {
		case strings.Contains(q, "createPhase"):
			st.ID = "phase_123"
			st.Color = nil
			st.Deleted = false
			if _, ok := gr.Variables["color"]; ok {
				st.CreateSawColor = true
			}
			if v, ok := gr.Variables["index"].(float64); ok {
				st.Index = &v
			} else {
				// The real API always assigns a position.
				serverAssigned := 5.0
				st.Index = &serverAssigned
			}
			merge()
			_, _ = io.WriteString(w, `{"data":{"createPhase":{"phase":`+st.toJSON()+`}}}`)
		case strings.Contains(q, "updatePhase"):
			if st.FailUpdate {
				_, _ = io.WriteString(w, `{"errors":[{"message":"color update failed"}]}`)
				return
			}
			if _, ok := gr.Variables["index"]; ok {
				st.UpdateSawIndex = true
			}
			if v, ok := gr.Variables["color"].(string); ok {
				st.Color = &v
			}
			merge()
			_, _ = io.WriteString(w, `{"data":{"updatePhase":{"phase":`+st.toJSON()+`}}}`)
		case strings.Contains(q, "deletePhase"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deletePhase":{"success":true}}}`)
		case strings.Contains(q, "phase("):
			if st.Deleted {
				_, _ = io.WriteString(w, `{"data":{"phase":null}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"phase":`+st.toJSON()+`}}`)
		case strings.Contains(q, "createPipe"):
			_, _ = io.WriteString(w, `{"data":{"createPipe":{"pipe":{"id":"301","name":"My Pipe"}}}}`)
		case strings.Contains(q, "deletePipe"):
			_, _ = io.WriteString(w, `{"data":{"deletePipe":{"success":true}}}`)
		case strings.Contains(q, "pipe("):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"301","name":"My Pipe","startFormPhaseId":"","phases":[]}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
}

func TestUnit_PhaseResource_CRUD(t *testing.T) {
	st := &phaseState{}
	srv := newPhaseServer(st)
	defer srv.Close()

	config := func(attrs string) string {
		return `
		provider "pipefy" {
			endpoint = "` + srv.URL + `"
			token    = "testtoken"
		}

		resource "pipefy_pipe" "p" {
			name            = "My Pipe"
			organization_id = "org_1"
		}

		resource "pipefy_phase" "test" {
			pipe_id = pipefy_pipe.p.id
` + attrs + `
		}
		`
	}

	fullAttrsInitial := `
			name                                 = "Renamed Phase"
			done                                 = true
			description                          = "First description"
			index                                = 1
			color                                = "blue"
			lateness_time                        = 3600
			can_receive_card_directly_from_draft = true
	`
	fullAttrsUpdated := `
			name                                 = "Renamed Phase"
			done                                 = false
			description                          = "Second description"
			index                                = 1
			color                                = "red"
			lateness_time                        = 7200
			can_receive_card_directly_from_draft = false
	`

	phaseValue := func(attr string, v knownvalue.Check) statecheck.StateCheck {
		return statecheck.ExpectKnownValue("pipefy_phase.test", tfjsonpath.New(attr), v)
	}
	expectAction := func(action plancheck.ResourceActionType) resource.ConfigPlanChecks {
		return resource.ConfigPlanChecks{
			PreApply: []plancheck.PlanCheck{
				plancheck.ExpectResourceAction("pipefy_phase.test", action),
			},
		}
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Minimal create: computed attributes resolve from API defaults.
				Config: config(`name = "My Phase"`),
				ConfigStateChecks: []statecheck.StateCheck{
					phaseValue("id", knownvalue.StringExact("phase_123")),
					phaseValue("name", knownvalue.StringExact("My Phase")),
					phaseValue("done", knownvalue.Bool(false)),
					phaseValue("description", knownvalue.Null()),
					phaseValue("color", knownvalue.Null()),
					phaseValue("index", knownvalue.Float64Exact(5)),
				},
			},
			{
				Config:           config(`name = "Renamed Phase"`),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionUpdate),
				ConfigStateChecks: []statecheck.StateCheck{
					phaseValue("name", knownvalue.StringExact("Renamed Phase")),
				},
			},
			{
				// index is create-only, so setting it forces replacement; color
				// must reach the API through the follow-up update after create.
				Config:           config(fullAttrsInitial),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionReplace),
				ConfigStateChecks: []statecheck.StateCheck{
					phaseValue("done", knownvalue.Bool(true)),
					phaseValue("description", knownvalue.StringExact("First description")),
					phaseValue("index", knownvalue.Float64Exact(1)),
					phaseValue("color", knownvalue.StringExact("blue")),
					phaseValue("lateness_time", knownvalue.Int64Exact(3600)),
					phaseValue("can_receive_card_directly_from_draft", knownvalue.Bool(true)),
				},
			},
			{
				Config:           config(fullAttrsUpdated),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionUpdate),
				ConfigStateChecks: []statecheck.StateCheck{
					phaseValue("done", knownvalue.Bool(false)),
					phaseValue("description", knownvalue.StringExact("Second description")),
					phaseValue("color", knownvalue.StringExact("red")),
					phaseValue("lateness_time", knownvalue.Int64Exact(7200)),
					phaseValue("can_receive_card_directly_from_draft", knownvalue.Bool(false)),
				},
			},
			{
				// Read must catch server-side drift so the apply writes it back.
				PreConfig: func() {
					drifted := "drifted outside terraform"
					st.Description = &drifted
				},
				Config:           config(fullAttrsUpdated),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionUpdate),
				ConfigStateChecks: []statecheck.StateCheck{
					phaseValue("description", knownvalue.StringExact("Second description")),
				},
			},
			{
				// A phase deleted outside terraform must be recreated.
				PreConfig:        func() { st.Deleted = true },
				Config:           config(fullAttrsUpdated),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionCreate),
			},
			{
				ResourceName:      "pipefy_phase.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})

	if st.CreateSawColor {
		t.Errorf("createPhase must not receive a color variable; the API only accepts it on updatePhase")
	}
	if st.UpdateSawIndex {
		t.Errorf("updatePhase must not receive an index variable; the API only accepts it on createPhase")
	}
	if st.Description == nil || *st.Description != "Second description" {
		t.Errorf("drifted description was not written back to the API: got %v", st.Description)
	}
	if st.DeletedCt < 2 {
		t.Fatalf("expected deletes from the index replacement and the final destroy, got %d", st.DeletedCt)
	}
}

// A failed follow-up color update must keep the created phase in state so
// terraform taints and replaces it instead of leaking it.
func TestUnit_PhaseResource_CreateColorFailureKeepsPhaseInState(t *testing.T) {
	st := &phaseState{FailUpdate: true}
	srv := newPhaseServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_phase" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "My Phase"
		color   = "blue"
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`setting its color failed`),
			},
		},
	})

	if st.DeletedCt != 1 {
		t.Fatalf("expected the half-created phase to be destroyed during cleanup, got %d deletes", st.DeletedCt)
	}
}
