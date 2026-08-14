// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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
// GraphQL selection: a single all_of comparison and one action with only its
// true branch set.
func fieldConditionBody(name string) string {
	return fieldConditionBodyOnPhase(name, "phase_1")
}

// fieldConditionBodyOnPhase is fieldConditionBody with an explicit owning phase.
func fieldConditionBodyOnPhase(name, phaseID string) string {
	return fieldConditionBodyOnPhaseRepo(name, phaseID, "123")
}

func fieldConditionBodyOnPhaseRepo(name, phaseID, repoID string) string {
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"` + phaseID + `","repo_id":` + repoID + `},` +
		`"condition":{"expressions":[{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"}],"expressions_structure":[[0]]},` +
		`"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true}]}`
}

func writeFieldConditionPipe(w http.ResponseWriter) {
	_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"123","name":"Ops","startFormPhaseId":"phase_1","organization":{"id":"1"}}}}`)
}

func TestUnit_FieldConditionResource_CRUD(t *testing.T) {
	st := &fieldConditionState{}
	var lastUpdate map[string]any
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
				lastUpdate = v
				if n, ok := v["name"].(string); ok {
					st.Name = n
				}
			}
			_, _ = io.WriteString(w, `{"data":{"updateFieldCondition":{"fieldCondition":`+fieldConditionBody(st.Name)+`}}}`)
		case strings.Contains(q, "deleteFieldCondition"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteFieldCondition":{"success":true}}}`)
		case strings.Contains(q, "GetPipe_tf"):
			writeFieldConditionPipe(w)
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
			pipe_id = "123"
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
					field      = "1002"
					when_true  = "show"
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
						tfjsonpath.New("pipe_id"),
						knownvalue.StringExact("123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("phase_id"),
						knownvalue.StringExact("phase_1"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("all_of").AtSliceIndex(0).AtMapKey("operation"),
						knownvalue.StringExact("equals"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("actions").AtSliceIndex(0).AtMapKey("when_true"),
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
				ResourceName:      "pipefy_field_condition.test",
				Config:            baseConfig("Renamed condition"),
				ImportState:       true,
				ImportStateId:     "fc_123",
				ImportStateVerify: true,
			},
			{
				Config: configDestroy,
			},
		},
	})

	if st.DeletedCt == 0 {
		t.Fatalf("expected deleteFieldCondition mutation to be called")
	}
	if lastUpdate == nil {
		t.Fatal("updateFieldCondition was never called")
	}
	if _, ok := lastUpdate["phase_id"]; ok {
		t.Errorf("Update sent phase_id: %+v", lastUpdate)
	}
}

// fieldConditionOrderingBody returns two expressions, each in its own
// structure-group (["1"],["0"]), with structure elements as strings rather
// than integers. It exercises reconstructing condition.any_of in
// expressions_structure order even when that doesn't match the order
// expressions themselves happen to appear in.
func fieldConditionOrderingBody(name string) string {
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"phase_1","repo_id":123},` +
		`"condition":{"expressions":[` +
		`{"structure_id":"1","field_address":"2001","operation":"equals","value":"High"},` +
		`{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"}` +
		`],"expressions_structure":[["1"],["0"]]},` +
		`"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true}]}`
}

// TestUnit_FieldConditionResource_AnyOfOrdering declares two any_of entries and
// asserts the resource preserves their order through create and the
// follow-up plan. This guards against a regression that would reorder
// entries and produce a spurious diff or an inconsistent result after apply.
func TestUnit_FieldConditionResource_AnyOfOrdering(t *testing.T) {
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
		case strings.Contains(q, "GetPipe_tf"):
			writeFieldConditionPipe(w)
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
		pipe_id = "123"
		name     = "Ordering"

		condition = {
			any_of = [
				{
					field     = "2001"
					operation = "equals"
					value     = "High"
				},
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
						tfjsonpath.New("condition").AtMapKey("any_of").AtSliceIndex(0).AtMapKey("field"),
						knownvalue.StringExact("2001"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("any_of").AtSliceIndex(1).AtMapKey("field"),
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
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"phase_1","repo_id":123},` +
		`"condition":{"expressions":[{"structure_id":"0","field_address":"1001","operation":"` + op + `","value":` + valJSON + `}],"expressions_structure":[[0]]},` +
		`"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true}]}`
}

// TestUnit_FieldConditionResource_ValueClear creates a comparison with a value,
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
		case strings.Contains(q, "GetPipe_tf"):
			writeFieldConditionPipe(w)
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
			pipe_id = "123"
			name     = "Value clear"

			condition = {
				all_of = [
					{
						field     = "1001"
						operation = "` + operation + `"
						` + valueLine + `
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
						tfjsonpath.New("condition").AtMapKey("all_of").AtSliceIndex(0).AtMapKey("value"),
						knownvalue.StringExact("Other"),
					),
				},
			},
			{
				Config: config("present", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("all_of").AtSliceIndex(0).AtMapKey("value"),
						knownvalue.Null(),
					),
				},
			},
		},
	})
}

// fieldConditionMixedAnyOfBody mirrors what the API would return for a mixed
// any_of: the first entry is a nested all_of of two comparisons, the second
// is a plain comparison. It assumes the provider flattens all_of/any_of into
// sequential integer structure_ids in declaration order.
func fieldConditionMixedAnyOfBody(name string) string {
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"phase_1","repo_id":123},` +
		`"condition":{"expressions":[` +
		`{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"},` +
		`{"structure_id":"1","field_address":"2001","operation":"equals","value":"High"},` +
		`{"structure_id":"2","field_address":"3001","operation":"present"}` +
		`],"expressions_structure":[[0,1],[2]]},` +
		`"actions":[{"actionId":"show","phaseField":{"internal_id":"1002"},"whenEvaluator":true}]}`
}

// TestUnit_FieldConditionResource_MixedAnyOf exercises an any_of entry that is
// itself a nested all_of of two comparisons, alongside a second, plain
// any_of entry. It verifies the provider sends sequential structure_ids to the
// API in group-then-comparison order, and that the response reconstructs into
// all_of/any_of matching the original nesting: not just the same shape, but the
// same comparisons in the same slots.
func TestUnit_FieldConditionResource_MixedAnyOf(t *testing.T) {
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
			_, _ = io.WriteString(w, `{"data":{"createFieldCondition":{"fieldCondition":`+fieldConditionMixedAnyOfBody(st.Name)+`}}}`)
		case strings.Contains(q, "deleteFieldCondition"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteFieldCondition":{"success":true}}}`)
		case strings.Contains(q, "GetPipe_tf"):
			writeFieldConditionPipe(w)
		case strings.Contains(q, "fieldCondition("):
			if st.ID == "" {
				_, _ = io.WriteString(w, `{"data":{"fieldCondition":null}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"fieldCondition":`+fieldConditionMixedAnyOfBody(st.Name)+`}}`)
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
		pipe_id = "123"
		name     = "Mixed any_of"

		condition = {
			any_of = [
				{
					all_of = [
						{
							field     = "1001"
							operation = "equals"
							value     = "Other"
						},
						{
							field     = "2001"
							operation = "equals"
							value     = "High"
						}
					]
				},
				{
					field     = "3001"
					operation = "present"
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
						tfjsonpath.New("condition").AtMapKey("any_of").AtSliceIndex(0).AtMapKey("all_of").AtSliceIndex(1).AtMapKey("field"),
						knownvalue.StringExact("2001"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("condition").AtMapKey("any_of").AtSliceIndex(1).AtMapKey("field"),
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
		id, ok := expr["structure_id"].(string)
		if !ok || id != strconv.Itoa(i) {
			t.Fatalf("expected expression %d to have structure_id %q, got %#v", i, strconv.Itoa(i), expr["structure_id"])
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

// renderActionsResponse turns the actions array the provider sent on the wire
// (actionId/phaseFieldId/whenEvaluator) into the shape the API's read/write
// response uses (actionId/phaseField.internal_id/whenEvaluator), simulating
// an API that fully replaces the actions collection on every mutation.
func renderActionsResponse(sentActions []any) string {
	parts := make([]string, 0, len(sentActions))
	for _, a := range sentActions {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		actionId, _ := m["actionId"].(string)
		phaseFieldId, _ := m["phaseFieldId"].(string)
		whenEvaluator, _ := m["whenEvaluator"].(bool)
		parts = append(parts, fmt.Sprintf(`{"actionId":%q,"phaseField":{"internal_id":%q},"whenEvaluator":%v}`, actionId, phaseFieldId, whenEvaluator))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func fieldConditionActionsBody(name, actionsJSON string) string {
	return `{"id":"fc_123","name":"` + name + `","phase":{"id":"phase_1","repo_id":123},` +
		`"condition":{"expressions":[{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"}],"expressions_structure":[[0]]},` +
		`"actions":` + actionsJSON + `}`
}

// TestUnit_FieldConditionResource_ActionBranchRemoval creates an action with
// both when_true and when_false set on the same field, then updates it to
// drop when_false. It verifies the update sends only the surviving branch on
// the wire (actionsInput never re-sends a removed branch) and that the
// removed branch reads back null rather than retaining its old verb — the
// hazard the old whenEvaluator Optional+Computed attribute used to hide.
func TestUnit_FieldConditionResource_ActionBranchRemoval(t *testing.T) {
	st := &fieldConditionState{}
	var sentActions []any
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
				if a, ok := v["actions"].([]any); ok {
					st.Body = renderActionsResponse(a)
				}
			}
			_, _ = io.WriteString(w, `{"data":{"createFieldCondition":{"fieldCondition":`+fieldConditionActionsBody("Branch removal", st.Body)+`}}}`)
		case strings.Contains(q, "updateFieldCondition"):
			if v, ok := gr.Variables["input"].(map[string]any); ok {
				if a, ok := v["actions"].([]any); ok {
					sentActions = a
					st.Body = renderActionsResponse(a)
				}
			}
			_, _ = io.WriteString(w, `{"data":{"updateFieldCondition":{"fieldCondition":`+fieldConditionActionsBody("Branch removal", st.Body)+`}}}`)
		case strings.Contains(q, "deleteFieldCondition"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteFieldCondition":{"success":true}}}`)
		case strings.Contains(q, "GetPipe_tf"):
			writeFieldConditionPipe(w)
		case strings.Contains(q, "fieldCondition("):
			if st.ID == "" {
				_, _ = io.WriteString(w, `{"data":{"fieldCondition":null}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"fieldCondition":`+fieldConditionActionsBody("Branch removal", st.Body)+`}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()

	baseConfig := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_field_condition" "test" {
		pipe_id = "123"
		name     = "Branch removal"

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
				field      = "1002"
				when_true  = "show"
				when_false = "hide"
			}
		]
	}
	`

	updatedConfig := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_field_condition" "test" {
		pipe_id = "123"
		name     = "Branch removal"

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

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: baseConfig,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("actions").AtSliceIndex(0).AtMapKey("when_true"),
						knownvalue.StringExact("show"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("actions").AtSliceIndex(0).AtMapKey("when_false"),
						knownvalue.StringExact("hide"),
					),
				},
			},
			{
				Config: updatedConfig,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("actions").AtSliceIndex(0).AtMapKey("when_true"),
						knownvalue.StringExact("show"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field_condition.test",
						tfjsonpath.New("actions").AtSliceIndex(0).AtMapKey("when_false"),
						knownvalue.Null(),
					),
				},
			},
		},
	})

	if len(sentActions) != 1 {
		t.Fatalf("expected update to send exactly 1 action after removing when_false, got %d: %#v", len(sentActions), sentActions)
	}
	sent, ok := sentActions[0].(map[string]any)
	if !ok {
		t.Fatalf("sent action is not an object: %#v", sentActions[0])
	}
	if actionId, _ := sent["actionId"].(string); actionId != "show" {
		t.Fatalf("expected surviving action to be %q, got %#v", "show", sent["actionId"])
	}
	if whenEvaluator, _ := sent["whenEvaluator"].(bool); !whenEvaluator {
		t.Fatalf("expected surviving action's whenEvaluator to be true, got %#v", sent["whenEvaluator"])
	}
}
