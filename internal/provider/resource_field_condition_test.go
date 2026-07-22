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
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

type fieldConditionState struct {
	ID        string
	Name      string
	DeletedCt int
}

// fieldConditionBody renders a fieldCondition payload matching the resource's
// GraphQL selection. expressions_structure is returned as integers to exercise
// the numeric-to-string normalization on read.
func fieldConditionBody(name string) string {
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"phase_1"},` +
		`"condition":{"expressions":[{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"}],"expressions_structure":[[0]]},` +
		`"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true}]}`
}

func TestUnit_FieldConditionResource_CRUD(t *testing.T) {
	st := &fieldConditionState{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case strings.Contains(q, "createFieldCondition"):
			st.ID = "fc_123"
			if v, ok := gr.Variables["input"].(map[string]any); ok {
				if n, ok := v["name"].(string); ok {
					st.Name = n
				}
			}
			_, _ = io.WriteString(w, `{"data":{"createFieldCondition":{"fieldCondition":`+fieldConditionBody(st.Name)+`}}}`)
		case strings.Contains(q, "updateFieldCondition"):
			if v, ok := gr.Variables["input"].(map[string]any); ok {
				if n, ok := v["name"].(string); ok {
					st.Name = n
				}
			}
			_, _ = io.WriteString(w, `{"data":{"updateFieldCondition":{"fieldCondition":`+fieldConditionBody(st.Name)+`}}}`)
		case strings.Contains(q, "deleteFieldCondition"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteFieldCondition":{"success":true}}}`)
		case strings.Contains(q, "repo_id"):
			_, _ = io.WriteString(w, `{"data":{"phase":{"repo_id":123}}}`)
		case strings.Contains(q, "fieldCondition("):
			if st.ID == "" {
				_, _ = io.WriteString(w, `{"data":{"fieldCondition":null}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"fieldCondition":`+fieldConditionBody(st.Name)+`}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()

	baseConfig := func(name string) string {
		return `
		provider "pipefy" {
			endpoint = "` + srv.URL + `"
			token    = "testtoken"
		}

		resource "pipefy_field_condition" "test" {
			phase_id = "phase_1"
			name     = "` + name + `"

			condition = {
				expressions = [
					{
						structure_id  = "0"
						field_address = "1001"
						operation     = "equals"
						value         = "Other"
					}
				]
				expressions_structure = [["0"]]
			}

			actions = [
				{
					action_id      = "show"
					phase_field_id  = "1002"
					when_evaluator  = true
				}
			]
		}
		`
	}

	configDestroy := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: baseConfig("Show details when type is Other"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("fc_123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("expressions").AtSliceIndex(0).AtMapKey("operation"),
						knownvalue.StringExact("equals"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("expressions_structure"),
						knownvalue.ListExact([]knownvalue.Check{
							knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("0")}),
						}),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("actions").AtSliceIndex(0).AtMapKey("action_id"),
						knownvalue.StringExact("show"),
					),
				},
			},
			{
				Config: baseConfig("Renamed condition"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Renamed condition"),
					),
				},
			},
			{
				Config: configDestroy,
			},
		},
	})

	if st.DeletedCt == 0 {
		t.Fatalf("expected deleteFieldCondition mutation to be called")
	}
}
