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
				groups = [
					{
						expressions = [
							{
								field_address = "1001"
								operation     = "equals"
								value         = "Other"
							}
						]
					}
				]
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
						tfjsonpath.New("condition").AtMapKey("groups").AtSliceIndex(0).AtMapKey("expressions").AtSliceIndex(0).AtMapKey("operation"),
						knownvalue.StringExact("equals"),
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

// fieldConditionOrderingBody returns two expressions, each in its own
// structure-group (["1"],["0"]), with structure elements as strings rather
// than integers. It exercises reconstructing condition.groups in
// expressions_structure order even when that doesn't match the order
// expressions themselves happen to appear in.
func fieldConditionOrderingBody(name string) string {
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"phase_1"},` +
		`"condition":{"expressions":[` +
		`{"structure_id":"1","field_address":"2001","operation":"equals","value":"High"},` +
		`{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"}` +
		`],"expressions_structure":[["1"],["0"]]},` +
		`"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true}]}`
}

// TestUnit_FieldConditionResource_ExpressionOrdering declares two groups and
// asserts the resource preserves both group order and within-group expression
// order through create and the follow-up plan. This guards against a
// regression that would reorder groups or expressions and produce a spurious
// diff or an inconsistent result after apply.
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
			groups = [
				{
					expressions = [
						{
							field_address = "2001"
							operation     = "equals"
							value         = "High"
						}
					]
				},
				{
					expressions = [
						{
							field_address = "1001"
							operation     = "equals"
							value         = "Other"
						}
					]
				}
			]
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
						tfjsonpath.New("condition").AtMapKey("groups").AtSliceIndex(0).AtMapKey("expressions").AtSliceIndex(0).AtMapKey("field_address"),
						knownvalue.StringExact("2001"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("groups").AtSliceIndex(1).AtMapKey("expressions").AtSliceIndex(0).AtMapKey("field_address"),
						knownvalue.StringExact("1001"),
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
				groups = [
					{
						expressions = [
							{
								field_address = "1001"
								operation     = "` + operation + `"
								` + valueLine + `
							}
						]
					}
				]
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
						tfjsonpath.New("condition").AtMapKey("groups").AtSliceIndex(0).AtMapKey("expressions").AtSliceIndex(0).AtMapKey("value"),
						knownvalue.StringExact("Other"),
					),
				},
			},
			{
				Config: config("present", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("groups").AtSliceIndex(0).AtMapKey("expressions").AtSliceIndex(0).AtMapKey("value"),
						knownvalue.Null(),
					),
				},
			},
		},
	})
}

// fieldConditionGroupFlatteningBody mirrors what the API would return for two
// groups: the first with two ANDed expressions, the second with one. It
// assumes the provider flattens groups into sequential integer structure_ids
// in declaration order (group 0's expressions first, then group 1's).
func fieldConditionGroupFlatteningBody(name string) string {
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"phase_1"},` +
		`"condition":{"expressions":[` +
		`{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"},` +
		`{"structure_id":"1","field_address":"2001","operation":"equals","value":"High"},` +
		`{"structure_id":"2","field_address":"3001","operation":"present"}` +
		`],"expressions_structure":[[0,1],[2]]},` +
		`"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true}]}`
}

// TestUnit_FieldConditionResource_GroupFlattening exercises a group with more
// than one ANDed expression alongside a second, single-expression group. It
// verifies the provider sends sequential integer structure_ids to the API in
// group-then-expression order, and that the response reconstructs into groups
// matching the original nesting.
func TestUnit_FieldConditionResource_GroupFlattening(t *testing.T) {
	st := &fieldConditionState{}
	var sentCondition map[string]any
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
				if c, ok := v["condition"].(map[string]any); ok {
					sentCondition = c
				}
			}
			_, _ = io.WriteString(w, `{"data":{"createFieldCondition":{"fieldCondition":`+fieldConditionGroupFlatteningBody(st.Name)+`}}}`)
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
			_, _ = io.WriteString(w, `{"data":{"fieldCondition":`+fieldConditionGroupFlatteningBody(st.Name)+`}}`)
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
		name     = "Group flattening"

		condition = {
			groups = [
				{
					expressions = [
						{
							field_address = "1001"
							operation     = "equals"
							value         = "Other"
						},
						{
							field_address = "2001"
							operation     = "equals"
							value         = "High"
						}
					]
				},
				{
					expressions = [
						{
							field_address = "3001"
							operation     = "present"
						}
					]
				}
			]
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
						tfjsonpath.New("condition").AtMapKey("groups").AtSliceIndex(0).AtMapKey("expressions").AtSliceIndex(1).AtMapKey("field_address"),
						knownvalue.StringExact("2001"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("groups").AtSliceIndex(1).AtMapKey("expressions").AtSliceIndex(0).AtMapKey("field_address"),
						knownvalue.StringExact("3001"),
					),
				},
			},
		},
	})

	if sentCondition == nil {
		t.Fatalf("createFieldCondition was never called")
	}
	exprs, ok := sentCondition["expressions"].([]any)
	if !ok || len(exprs) != 3 {
		t.Fatalf("expected 3 flattened expressions, got %#v", sentCondition["expressions"])
	}
	for i, e := range exprs {
		expr, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("expression %d is not an object: %#v", i, e)
		}
		id, ok := expr["structure_id"].(float64)
		if !ok || int(id) != i {
			t.Fatalf("expected expression %d to have structure_id %d, got %#v", i, i, expr["structure_id"])
		}
	}
	structure, ok := sentCondition["expressions_structure"].([]any)
	if !ok || len(structure) != 2 {
		t.Fatalf("expected 2 groups in expressions_structure, got %#v", sentCondition["expressions_structure"])
	}
	if g0, ok := structure[0].([]any); !ok || len(g0) != 2 {
		t.Fatalf("expected first group to reference 2 structure_ids, got %#v", structure[0])
	}
	if g1, ok := structure[1].([]any); !ok || len(g1) != 1 {
		t.Fatalf("expected second group to reference 1 structure_id, got %#v", structure[1])
	}
}
