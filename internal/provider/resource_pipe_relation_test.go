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

type pipeRelationState struct {
	input       map[string]any
	id          string
	deletedCt   int
	reverseMaps bool
}

func (st *pipeRelationState) relationObj() map[string]any {
	in := st.input
	maps := in["ownFieldMaps"]
	if st.reverseMaps {
		if s, ok := maps.([]any); ok {
			rev := make([]any, len(s))
			for i, v := range s {
				rev[len(s)-1-i] = v
			}
			maps = rev
		}
	}
	return map[string]any{
		"id":                                  st.id,
		"name":                                in["name"],
		"canCreateNewItems":                   in["canCreateNewItems"],
		"canConnectExistingItems":             in["canConnectExistingItems"],
		"canConnectMultipleItems":             in["canConnectMultipleItems"],
		"allChildrenMustBeDoneToFinishParent": in["allChildrenMustBeDoneToFinishParent"],
		"allChildrenMustBeDoneToMoveParent":   in["allChildrenMustBeDoneToMoveParent"],
		"childMustExistToFinishParent":        in["childMustExistToFinishParent"],
		"childMustExistToMoveParent":          in["childMustExistToMoveParent"],
		"autoFillFieldEnabled":                in["autoFillFieldEnabled"],
		"parent":                              map[string]any{"id": in["parentId"]},
		"child":                               map[string]any{"id": in["childId"]},
		"ownFieldMaps":                        maps,
	}
}

func pipeRelationMockHandler(st *pipeRelationState) http.HandlerFunc {
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

		input, _ := gr.Variables["input"].(map[string]any)
		q := gr.Query
		switch {
		case strings.Contains(q, "createPipeRelation"):
			st.id = "rel_1"
			st.input = input
			_, _ = io.WriteString(w, `{"data":{"createPipeRelation":{"pipeRelation":{"id":"rel_1"}}}}`)
		case strings.Contains(q, "updatePipeRelation"):
			for k, v := range input {
				st.input[k] = v
			}
			_, _ = io.WriteString(w, `{"data":{"updatePipeRelation":{"pipeRelation":{"id":"rel_1"}}}}`)
		case strings.Contains(q, "deletePipeRelation"):
			st.deletedCt++
			st.input = nil
			_, _ = io.WriteString(w, `{"data":{"deletePipeRelation":{"success":true}}}`)
		case strings.Contains(q, "childrenRelations"):
			var rels []any
			if st.input != nil {
				rels = append(rels, st.relationObj())
			}
			out, _ := json.Marshal(map[string]any{"data": map[string]any{"pipe": map[string]any{"childrenRelations": rels}}})
			_, _ = w.Write(out)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}
}

func TestUnit_PipeRelationResource_CRUD(t *testing.T) {
	st := &pipeRelationState{}
	srv := httptest.NewServer(pipeRelationMockHandler(st))
	defer srv.Close()

	provider := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}
	`

	create := provider + `
	resource "pipefy_pipe_relation" "test" {
		parent_id = "pipe_parent"
		child_id  = "pipe_child"
		name      = "Sub-tasks"
	}
	`

	update := provider + `
	resource "pipefy_pipe_relation" "test" {
		parent_id            = "pipe_parent"
		child_id             = "pipe_child"
		name                 = "Approvals"
		can_create_new_items = false

		all_children_must_be_done_to_finish_parent = true

		own_field_maps = [{
			field_id   = "431323022"
			input_mode = "fixed_value"
			value      = "hello"
		}]
	}
	`

	destroy := provider + `
	resource "pipefy_pipe_relation" "test" {
		count     = 0
		parent_id = "pipe_parent"
		child_id  = "pipe_child"
		name      = "Approvals"
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: create,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("id"), knownvalue.StringExact("rel_1")),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("name"), knownvalue.StringExact("Sub-tasks")),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("parent_id"), knownvalue.StringExact("pipe_parent")),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("child_id"), knownvalue.StringExact("pipe_child")),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("can_create_new_items"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("can_connect_existing_items"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("can_connect_multiple_items"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("all_children_must_be_done_to_finish_parent"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("auto_fill_field_enabled"), knownvalue.Bool(false)),
				},
			},
			{
				Config: update,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("name"), knownvalue.StringExact("Approvals")),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("can_create_new_items"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("all_children_must_be_done_to_finish_parent"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("own_field_maps").AtSliceIndex(0).AtMapKey("field_id"), knownvalue.StringExact("431323022")),
					statecheck.ExpectKnownValue("pipefy_pipe_relation.test", tfjsonpath.New("own_field_maps").AtSliceIndex(0).AtMapKey("value"), knownvalue.StringExact("hello")),
				},
			},
			{
				ResourceName:            "pipefy_pipe_relation.test",
				ImportState:             true,
				ImportStateId:           "pipe_parent/rel_1",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"own_field_maps"},
			},
			{
				Config: destroy,
			},
		},
	})

	if st.deletedCt == 0 {
		t.Fatalf("expected deletePipeRelation mutation to be called")
	}
}

func TestUnit_PipeRelationResource_OwnFieldMapsSetOrderInsensitive(t *testing.T) {
	st := &pipeRelationState{reverseMaps: true}
	srv := httptest.NewServer(pipeRelationMockHandler(st))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe_relation" "test" {
		parent_id               = "pipe_parent"
		child_id                = "pipe_child"
		name                    = "Maps"
		auto_fill_field_enabled = true

		own_field_maps = [
			{ field_id = "111", input_mode = "fixed_value", value = "A" },
			{ field_id = "222", input_mode = "fixed_value", value = "B" },
		]
	}
	`

	fieldMap := func(id, value string) knownvalue.Check {
		return knownvalue.ObjectExact(map[string]knownvalue.Check{
			"field_id":   knownvalue.StringExact(id),
			"input_mode": knownvalue.StringExact("fixed_value"),
			"value":      knownvalue.StringExact(value),
		})
	}

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
						"pipefy_pipe_relation.test",
						tfjsonpath.New("own_field_maps"),
						knownvalue.SetExact([]knownvalue.Check{
							fieldMap("111", "A"),
							fieldMap("222", "B"),
						}),
					),
				},
			},
		},
	})
}

func TestUnit_PipeRelationResource_OwnFieldMapsClearedConverges(t *testing.T) {
	st := &pipeRelationState{}
	srv := httptest.NewServer(pipeRelationMockHandler(st))
	defer srv.Close()

	provider := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}
	`
	withMaps := provider + `
	resource "pipefy_pipe_relation" "test" {
		parent_id               = "pipe_parent"
		child_id                = "pipe_child"
		name                    = "Maps"
		auto_fill_field_enabled = true

		own_field_maps = [
			{ field_id = "111", input_mode = "fixed_value", value = "A" },
		]
	}
	`
	withoutMaps := provider + `
	resource "pipefy_pipe_relation" "test" {
		parent_id               = "pipe_parent"
		child_id                = "pipe_child"
		name                    = "Maps"
		auto_fill_field_enabled = true
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: withMaps,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_pipe_relation.test",
						tfjsonpath.New("own_field_maps"),
						knownvalue.SetSizeExact(1),
					),
				},
			},
			{
				Config: withoutMaps,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_pipe_relation.test",
						tfjsonpath.New("own_field_maps"),
						knownvalue.Null(),
					),
				},
			},
		},
	})
}
