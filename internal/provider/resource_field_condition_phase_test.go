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

type fieldConditionPhaseState struct {
	name          string
	createPhase   string
	startForm     string
	createPhaseId string
	created       int
	pipeID        string
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
			if p, ok := v["phaseId"].(string); ok {
				st.createPhaseId = p
			}
		}

		q := gr.Query
		switch {
		case strings.Contains(q, "GetPipe_tf"):
			pipeID := "123"
			if id, ok := gr.Variables["id"].(string); ok && id != "" {
				pipeID = id
			}
			st.pipeID = pipeID
			start := st.startForm
			if start == "" {
				start = "phase_1"
			}
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"`+pipeID+`","name":"Ops","startFormPhaseId":"`+start+`","organization":{"id":"1"}}}}`)
		case strings.Contains(q, "createFieldCondition"):
			st.created++
			phase := st.createPhase
			if phase == "" {
				phase = "phase_1"
			}
			_, _ = io.WriteString(w, `{"data":{"createFieldCondition":{"fieldCondition":`+st.body(phase)+`}}}`)
		case strings.Contains(q, "updateFieldCondition"):
			_, _ = io.WriteString(w, `{"data":{"updateFieldCondition":{"fieldCondition":`+st.body(st.createPhase)+`}}}`)
		case strings.Contains(q, "deleteFieldCondition"):
			_, _ = io.WriteString(w, `{"data":{"deleteFieldCondition":{"success":true}}}`)
		case strings.Contains(q, "fieldCondition("):
			_, _ = io.WriteString(w, `{"data":{"fieldCondition":`+st.body(st.createPhase)+`}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}
}

func (st *fieldConditionPhaseState) body(phase string) string {
	repo := st.pipeID
	if repo == "" {
		repo = "123"
	}
	if phase == "" {
		phase = "phase_1"
	}
	return fieldConditionBodyOnPhaseRepo(st.name, phase, repo)
}

func fieldConditionPipeConfig(endpoint, pipeID string) string {
	return `
	provider "pipefy" {
		endpoint = "` + endpoint + `"
		token    = "testtoken"
	}

	resource "pipefy_field_condition" "test" {
		pipe_id = "` + pipeID + `"
		name    = "Show details when type is Other"

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

// TestUnit_FieldConditionResource_CreateRecordsListedPhase: create sends the
// pipe's start form as phaseId and state records the phase the API lists.
func TestUnit_FieldConditionResource_CreateRecordsListedPhase(t *testing.T) {
	st := &fieldConditionPhaseState{startForm: "phase_start", createPhase: "phase_start"}
	srv := httptest.NewServer(fieldConditionPhaseHandler(st))
	defer srv.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fieldConditionPipeConfig(srv.URL, "123"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("pipe_id"),
						knownvalue.StringExact("123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("phase_id"),
						knownvalue.StringExact("phase_start"),
					),
				},
			},
		},
	})

	if st.createPhaseId != "phase_start" {
		t.Fatalf("expected create to send the start form phase, got %q", st.createPhaseId)
	}
}

// TestUnit_FieldConditionResource_RelocationIsRecorded: createFieldCondition
// attaches the condition to a different phase than the start form; Terraform
// records that phase instead of failing the apply.
func TestUnit_FieldConditionResource_RelocationIsRecorded(t *testing.T) {
	st := &fieldConditionPhaseState{startForm: "phase_start", createPhase: "phase_listed"}
	srv := httptest.NewServer(fieldConditionPhaseHandler(st))
	defer srv.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fieldConditionPipeConfig(srv.URL, "123"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("phase_id"),
						knownvalue.StringExact("phase_listed"),
					),
				},
			},
		},
	})

	if st.created != 1 {
		t.Fatalf("expected exactly 1 createFieldCondition, got %d", st.created)
	}
	if st.createPhaseId != "phase_start" {
		t.Fatalf("expected create to send the start form phase, got %q", st.createPhaseId)
	}
}

func TestUnit_FieldConditionResource_PipeIdChangeIsReplace(t *testing.T) {
	st := &fieldConditionPhaseState{startForm: "phase_1", createPhase: "phase_1"}
	srv := httptest.NewServer(fieldConditionPhaseHandler(st))
	defer srv.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fieldConditionPipeConfig(srv.URL, "123"),
			},
			{
				Config: fieldConditionPipeConfig(srv.URL, "456"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("pipefy_field_condition.test", plancheck.ResourceActionReplace),
					},
				},
			},
		},
	})
}
