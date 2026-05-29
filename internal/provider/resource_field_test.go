// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

type fieldState struct {
	ID          string
	InternalId  string
	Label       string
	OptionsJSON string
	DeletedCt   int
}

func (s *fieldState) captureOptions(vars map[string]any) {
	if opts, ok := vars["options"]; ok {
		b, _ := json.Marshal(opts)
		s.OptionsJSON = string(b)
	}
}

func (s *fieldState) optionsJSON() string {
	if s.OptionsJSON == "" {
		return "null"
	}
	return s.OptionsJSON
}

func fieldMockHandler(st *fieldState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		q := gr.Query
		switch {
		case strings.Contains(q, "createPhaseField"):
			st.ID = "field_123"
			st.InternalId = "456"
			if v, ok := gr.Variables["label"].(string); ok {
				st.Label = v
			}
			st.captureOptions(gr.Variables)
			_, _ = io.WriteString(w, `{"data":{"createPhaseField":{"phase_field":{"id":"`+st.ID+`","internal_id":"`+st.InternalId+`","label":"`+st.Label+`","options":`+st.optionsJSON()+`}}}}`)
		case strings.Contains(q, "updatePhaseField"):
			if v, ok := gr.Variables["label"].(string); ok {
				st.Label = v
			}
			st.captureOptions(gr.Variables)
			_, _ = io.WriteString(w, `{"data":{"updatePhaseField":{"phase_field":{"id":"`+st.ID+`","internal_id":"`+st.InternalId+`","options":`+st.optionsJSON()+`}}}}`)
		case strings.Contains(q, "deletePhaseField"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deletePhaseField":{"success":true}}}`)
		case strings.Contains(q, "phase(") && strings.Contains(q, "repo_id"):
			_, _ = io.WriteString(w, `{"data":{"phase":{"repo_id":123}}}`)
		case strings.Contains(q, "pipe(") && strings.Contains(q, "uuid"):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"uuid":"pipe-uuid-1"}}}`)
		case strings.Contains(q, "phase("):
			_, _ = io.WriteString(w, `{"data":{"phase":{"fields":[{"id":"`+st.ID+`","internal_id":"`+st.InternalId+`","label":"`+st.Label+`","options":`+st.optionsJSON()+`}]}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}
}

func TestUnit_FieldResource_CRUD(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
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

	resource "pipefy_phase" "ph" {
		pipe_id = pipefy_pipe.p.id
		name    = "My Phase"
	}

	resource "pipefy_field" "test" {
		phase_id = pipefy_phase.ph.id
		type     = "short_text"
		label    = "My Field"
	}
	`

	configDestroy := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_phase" "ph" {
		pipe_id = pipefy_pipe.p.id
		name    = "My Phase"
	}

	resource "pipefy_field" "test" {
		count    = 0
		phase_id = pipefy_phase.ph.id
		type     = "short_text"
		label    = "My Field"
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("field_123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("internal_id"),
						knownvalue.StringExact("456"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("label"),
						knownvalue.StringExact("My Field"),
					),
				},
			},
			{
				Config: strings.ReplaceAll(config, "My Field", "Renamed Field"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("internal_id"),
						knownvalue.StringExact("456"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("label"),
						knownvalue.StringExact("Renamed Field"),
					),
				},
			},
			{
				Config: configDestroy,
			},
		},
	})

	if st.DeletedCt == 0 {
		t.Fatalf("expected delete mutation to be called")
	}
}

func TestUnit_FieldResource_Options_CRUD(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	base := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_phase" "ph" {
		pipe_id = pipefy_pipe.p.id
		name    = "My Phase"
	}
	`

	configCreate := base + `
	resource "pipefy_field" "test" {
		phase_id = pipefy_phase.ph.id
		type     = "checklist_vertical"
		label    = "Approved?"
		options  = ["Sim", "Não"]
	}
	`

	configUpdate := base + `
	resource "pipefy_field" "test" {
		phase_id = pipefy_phase.ph.id
		type     = "checklist_vertical"
		label    = "Approved?"
		options  = ["Sim", "Não", "Talvez"]
	}
	`

	// Same options, only the label changes — a label-only edit must keep options.
	configRelabel := base + `
	resource "pipefy_field" "test" {
		phase_id = pipefy_phase.ph.id
		type     = "checklist_vertical"
		label    = "Approved (final)?"
		options  = ["Sim", "Não", "Talvez"]
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configCreate,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("options"),
						knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("Sim"),
							knownvalue.StringExact("Não"),
						}),
					),
				},
			},
			{
				Config: configUpdate,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("pipefy_field.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("options"),
						knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("Sim"),
							knownvalue.StringExact("Não"),
							knownvalue.StringExact("Talvez"),
						}),
					),
				},
			},
			{
				// Label-only edit: options stay in config unchanged. The update
				// must apply in place and the options must round-trip intact, not
				// get clobbered to null.
				Config: configRelabel,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("pipefy_field.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("label"),
						knownvalue.StringExact("Approved (final)?"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("options"),
						knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("Sim"),
							knownvalue.StringExact("Não"),
							knownvalue.StringExact("Talvez"),
						}),
					),
				},
			},
		},
	})

	if st.OptionsJSON != `["Sim","Não","Talvez"]` {
		t.Fatalf("expected the label-only update to preserve options, got %q", st.OptionsJSON)
	}
}
