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
	Body      string
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

// fieldConditionOrderingBody echoes two expressions in the same order the config
// declares them (structure_id 1 then 0), and returns expressions_structure
// elements as strings. The API preserves the caller's expression order on both
// create and read, and returns structure elements as strings rather than
// integers.
func fieldConditionOrderingBody(name string) string {
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"phase_1"},` +
		`"condition":{"expressions":[` +
		`{"structure_id":"1","field_address":"2001","operation":"equals","value":"High"},` +
		`{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"}` +
		`],"expressions_structure":[["1"],["0"]]},` +
		`"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true}]}`
}

// TestUnit_FieldConditionResource_ExpressionOrdering declares expressions in
// descending structure_id order (1 then 0) and asserts the resource preserves
// that order through create and the follow-up plan. The API keeps the caller's
// order, so a stable round-trip here guards against a regression that would
// reorder expressions and produce a spurious diff or an inconsistent result
// after apply.
func TestUnit_FieldConditionResource_ExpressionOrdering(t *testing.T) {
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
			_, _ = io.WriteString(w, `{"data":{"createFieldCondition":{"fieldCondition":`+fieldConditionOrderingBody(st.Name)+`}}}`)
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
			_, _ = io.WriteString(w, `{"data":{"fieldCondition":`+fieldConditionOrderingBody(st.Name)+`}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_field_condition" "test" {
		phase_id = "phase_1"
		name     = "Ordering"

		condition = {
			expressions = [
				{
					structure_id  = "1"
					field_address = "2001"
					operation     = "equals"
					value         = "High"
				},
				{
					structure_id  = "0"
					field_address = "1001"
					operation     = "equals"
					value         = "Other"
				}
			]
			expressions_structure = [["1"], ["0"]]
		}

		actions = [
			{
				action_id      = "show"
				phase_field_id  = "1002"
			}
		]
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
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("expressions").AtSliceIndex(0).AtMapKey("structure_id"),
						knownvalue.StringExact("1"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("expressions").AtSliceIndex(1).AtMapKey("structure_id"),
						knownvalue.StringExact("0"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("expressions_structure"),
						knownvalue.ListExact([]knownvalue.Check{
							knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("1")}),
							knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("0")}),
						}),
					),
				},
			},
		},
	})
}

// fieldConditionValueBody echoes the first expression's operation and value from
// the request, so value is present in the response only when the provider sent
// it. This mimics an API that honors an omitted value, letting the test verify
// value clears cleanly when the operation no longer needs one.
func fieldConditionValueBody(name string, gr gqlReq) string {
	op, valJSON := "equals", "null"
	if input, ok := gr.Variables["input"].(map[string]any); ok {
		if cond, ok := input["condition"].(map[string]any); ok {
			if exprs, ok := cond["expressions"].([]any); ok && len(exprs) > 0 {
				if e0, ok := exprs[0].(map[string]any); ok {
					if o, ok := e0["operation"].(string); ok {
						op = o
					}
					if v, ok := e0["value"].(string); ok {
						valJSON = `"` + v + `"`
					}
				}
			}
		}
	}
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"phase_1"},` +
		`"condition":{"expressions":[{"structure_id":"0","field_address":"1001","operation":"` + op + `","value":` + valJSON + `}],"expressions_structure":[[0]]},` +
		`"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true}]}`
}

// TestUnit_FieldConditionResource_ValueClear creates an expression with a value,
// then updates it to a value-less operation (present) that drops value from
// config. It verifies value round-trips to null rather than lingering in state.
func TestUnit_FieldConditionResource_ValueClear(t *testing.T) {
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
			st.Body = fieldConditionValueBody("Value clear", gr)
			_, _ = io.WriteString(w, `{"data":{"createFieldCondition":{"fieldCondition":`+st.Body+`}}}`)
		case strings.Contains(q, "updateFieldCondition"):
			st.Body = fieldConditionValueBody("Value clear", gr)
			_, _ = io.WriteString(w, `{"data":{"updateFieldCondition":{"fieldCondition":`+st.Body+`}}}`)
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
			_, _ = io.WriteString(w, `{"data":{"fieldCondition":`+st.Body+`}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()

	config := func(operation, valueLine string) string {
		return `
		provider "pipefy" {
			endpoint = "` + srv.URL + `"
			token    = "testtoken"
		}

		resource "pipefy_field_condition" "test" {
			phase_id = "phase_1"
			name     = "Value clear"

			condition = {
				expressions = [
					{
						structure_id  = "0"
						field_address = "1001"
						operation     = "` + operation + `"
						` + valueLine + `
					}
				]
				expressions_structure = [["0"]]
			}

			actions = [
				{
					action_id      = "show"
					phase_field_id  = "1002"
				}
			]
		}
		`
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("equals", `value = "Other"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("expressions").AtSliceIndex(0).AtMapKey("value"),
						knownvalue.StringExact("Other"),
					),
				},
			},
			{
				Config: config("present", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("expressions").AtSliceIndex(0).AtMapKey("value"),
						knownvalue.Null(),
					),
				},
			},
		},
	})
}
