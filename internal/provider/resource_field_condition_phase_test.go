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

// fieldConditionPhaseState lets each GraphQL op report its own owning phase.
type fieldConditionPhaseState struct {
	name        string
	createPhase string
	updatePhase string
	readPhase   string
	echoPhase   bool
	deleteFails bool
	created     int
	deleted     int
}

func fieldConditionRequestedPhase(gr gqlReq) string {
	v, ok := gr.Variables["input"].(map[string]any)
	if !ok {
		return ""
	}
	if p, ok := v["phaseId"].(string); ok {
		return p
	}
	if p, ok := v["phase_id"].(string); ok {
		return p
	}
	return ""
}

func fieldConditionPhaseHandler(st *fieldConditionPhaseState) http.HandlerFunc {
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

		if v, ok := gr.Variables["input"].(map[string]any); ok {
			if n, ok := v["name"].(string); ok {
				st.name = n
			}
		}

		q := gr.Query
		switch {
		case strings.Contains(q, "createFieldCondition"):
			st.created++
			phase := st.createPhase
			if st.echoPhase {
				if requested := fieldConditionRequestedPhase(gr); requested != "" {
					phase = requested
				}
				st.readPhase = phase
				st.updatePhase = phase
			}
			_, _ = io.WriteString(w, `{"data":{"createFieldCondition":{"fieldCondition":`+fieldConditionBodyOnPhase(st.name, phase)+`}}}`)
		case strings.Contains(q, "updateFieldCondition"):
			phase := st.updatePhase
			if st.echoPhase {
				if requested := fieldConditionRequestedPhase(gr); requested != "" {
					phase = requested
				}
				st.readPhase = phase
			}
			_, _ = io.WriteString(w, `{"data":{"updateFieldCondition":{"fieldCondition":`+fieldConditionBodyOnPhase(st.name, phase)+`}}}`)
		case strings.Contains(q, "deleteFieldCondition"):
			st.deleted++
			if st.deleteFails {
				_, _ = io.WriteString(w, `{"errors":[{"message":"the phase is locked"}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"deleteFieldCondition":{"success":true}}}`)
		case strings.Contains(q, "repo_id"):
			_, _ = io.WriteString(w, `{"data":{"phase":{"repo_id":123}}}`)
		case strings.Contains(q, "fieldCondition("):
			_, _ = io.WriteString(w, `{"data":{"fieldCondition":`+fieldConditionBodyOnPhase(st.name, st.readPhase)+`}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}
}

// fieldConditionPhaseConfig requests phase_2 (relocation mocks may answer phase_1).
func fieldConditionPhaseConfig(endpoint, name string) string {
	return fieldConditionPhaseConfigOn(endpoint, name, "phase_2")
}

func fieldConditionPhaseConfigOn(endpoint, name, phaseID string) string {
	return `
	provider "pipefy" {
		endpoint = "` + endpoint + `"
		token    = "testtoken"
	}

	resource "pipefy_field_condition" "test" {
		phase_id = "` + phaseID + `"
		name     = "` + name + `"

		condition = {
			all_of = [
				{
					field     = "1001"
					operation = "equals"
					value     = "Other"
				}
			]
		}

		actions = [
			{
				field     = "1002"
				when_true = "show"
			}
		]
	}
	`
}

// TestUnit_FieldConditionResource_CreatePhaseMismatchRollsBack: create relocates
// to the start form; provider deletes the condition and fails with both phases.
func TestUnit_FieldConditionResource_CreatePhaseMismatchRollsBack(t *testing.T) {
	st := &fieldConditionPhaseState{createPhase: "phase_1", updatePhase: "phase_1", readPhase: "phase_1"}
	srv := httptest.NewServer(fieldConditionPhaseHandler(st))
	defer srv.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fieldConditionPhaseConfig(srv.URL, "Show details when type is Other"),
				ExpectError: regexp.MustCompile(`(?s)phase_2.*phase_1.*start form`),
			},
		},
	})

	if st.created != 1 {
		t.Fatalf("expected exactly 1 createFieldCondition, got %d", st.created)
	}
	if st.deleted != 1 {
		t.Fatalf("expected the rollback to delete the condition exactly once, got %d deletes", st.deleted)
	}
}

// TestUnit_FieldConditionResource_CreatePhaseMismatchRollbackFails: orphan error
// names the phase mismatch and the failed delete.
func TestUnit_FieldConditionResource_CreatePhaseMismatchRollbackFails(t *testing.T) {
	st := &fieldConditionPhaseState{createPhase: "phase_1", updatePhase: "phase_1", readPhase: "phase_1", deleteFails: true}
	srv := httptest.NewServer(fieldConditionPhaseHandler(st))
	defer srv.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fieldConditionPhaseConfig(srv.URL, "Show details when type is Other"),
				ExpectError: regexp.MustCompile(`(?s)orphaned.*phase_2.*rollback failed.*the phase is locked`),
			},
		},
	})

	if st.deleted != 1 {
		t.Fatalf("expected exactly 1 rollback delete attempt, got %d", st.deleted)
	}
}

// TestUnit_FieldConditionResource_UpdatePhaseMismatchFails: update relocates the
// phase; apply fails without rollback and the same resource stays managed.
func TestUnit_FieldConditionResource_UpdatePhaseMismatchFails(t *testing.T) {
	st := &fieldConditionPhaseState{createPhase: "phase_2", updatePhase: "phase_2", readPhase: "phase_2"}
	srv := httptest.NewServer(fieldConditionPhaseHandler(st))
	defer srv.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fieldConditionPhaseConfig(srv.URL, "Show details when type is Other"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("phase_id"),
						knownvalue.StringExact("phase_2"),
					),
				},
			},
			{
				PreConfig:   func() { st.updatePhase = "phase_1" },
				Config:      fieldConditionPhaseConfig(srv.URL, "Renamed condition"),
				ExpectError: regexp.MustCompile(`(?s)phase_2.*phase_1.*start form`),
			},
			{
				PreConfig: func() { st.updatePhase = "phase_2" },
				Config:    fieldConditionPhaseConfig(srv.URL, "Renamed condition"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Renamed condition"),
					),
				},
			},
		},
	})

	if st.created != 1 {
		t.Fatalf("expected the failed update to keep the condition under management, got %d creates", st.created)
	}
}

// TestUnit_FieldConditionResource_PhaseIdChangeIsUpdate: changing phase_id
// plans an update, not a replacement, so a later mismatch can fail in place.
func TestUnit_FieldConditionResource_PhaseIdChangeIsUpdate(t *testing.T) {
	st := &fieldConditionPhaseState{echoPhase: true}
	srv := httptest.NewServer(fieldConditionPhaseHandler(st))
	defer srv.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fieldConditionPhaseConfigOn(srv.URL, "Show details when type is Other", "phase_2"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("phase_id"),
						knownvalue.StringExact("phase_2"),
					),
				},
			},
			{
				Config: fieldConditionPhaseConfigOn(srv.URL, "Show details when type is Other", "phase_1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("pipefy_field_condition.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("phase_id"),
						knownvalue.StringExact("phase_1"),
					),
				},
			},
		},
	})

	if st.created != 1 {
		t.Fatalf("expected the phase_id change to update in place, got %d creates", st.created)
	}
}
