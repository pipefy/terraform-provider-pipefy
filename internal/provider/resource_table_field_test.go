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
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

type tableFieldState struct {
	id, internalID, uuid, label, fieldType, optionsJSON string
	required, unique                                    *bool
	description, help, customValidation                 *string
	minimalView                                         *bool
	created                                             bool
	deletedCt                                           int
}

func tableFieldObj(st *tableFieldState) string {
	return `{"id":"` + st.id + `","internal_id":"` + st.internalID +
		`","uuid":"` + st.uuid + `","label":"` + st.label +
		`","type":"` + st.fieldType +
		`","required":` + jsonBool(st.required) +
		`,"options":` + st.optionsJSON +
		`,"description":` + jsonStr(st.description) +
		`,"help":` + jsonStr(st.help) +
		`,"minimal_view":` + jsonBool(st.minimalView) +
		`,"custom_validation":` + jsonStr(st.customValidation) +
		`,"unique":` + jsonBool(st.unique) + `}`
}

func tableFieldMockHandler(st *tableFieldState) http.HandlerFunc {
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

		switch q := gr.Query; {
		case strings.Contains(q, "createTableField"):
			st.id, st.internalID, st.uuid = "tfield_123", "789", "tfield-uuid-1"
			st.label, _ = gr.Variables["label"].(string)
			st.fieldType, _ = gr.Variables["type"].(string)
			st.required = varBool(gr.Variables, "required")
			st.description = varStr(gr.Variables, "description")
			st.help = varStr(gr.Variables, "help")
			st.minimalView = varBool(gr.Variables, "minimalView")
			st.customValidation = varStr(gr.Variables, "customValidation")
			st.unique = varBool(gr.Variables, "unique")
			st.optionsJSON = optionsJSON(gr.Variables, "null")
			st.created = true
			_, _ = io.WriteString(w, `{"data":{"createTableField":{"table_field":`+tableFieldObj(st)+`}}}`)
		case strings.Contains(q, "updateTableField"):
			if v, ok := gr.Variables["label"].(string); ok {
				st.label = v
			}
			if p := varBool(gr.Variables, "required"); p != nil {
				st.required = p
			}
			if p := varStr(gr.Variables, "description"); p != nil {
				st.description = p
			}
			if p := varStr(gr.Variables, "help"); p != nil {
				st.help = p
			}
			if p := varBool(gr.Variables, "minimalView"); p != nil {
				st.minimalView = p
			}
			if p := varStr(gr.Variables, "customValidation"); p != nil {
				st.customValidation = p
			}
			if p := varBool(gr.Variables, "unique"); p != nil {
				st.unique = p
			}
			st.optionsJSON = optionsJSON(gr.Variables, st.optionsJSON)
			_, _ = io.WriteString(w, `{"data":{"updateTableField":{"table_field":`+tableFieldObj(st)+`}}}`)
		case strings.Contains(q, "deleteTableField"):
			st.deletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteTableField":{"success":true}}}`)
		case strings.Contains(q, "table("):
			fields := ""
			if st.created {
				fields = tableFieldObj(st)
			}
			_, _ = io.WriteString(w, `{"data":{"table":{"table_fields":[`+fields+`]}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}
}

func tableFieldConfig(srvURL, fieldBlock string) string {
	return `
provider "pipefy" {
  endpoint = "` + srvURL + `"
  token    = "testtoken"
}

resource "pipefy_table" "t" {
  name            = "My Table"
  organization_id = "org_1"
}
` + fieldBlock
}

func expectTableFieldStr(attr, val string) statecheck.StateCheck {
	return statecheck.ExpectKnownValue("pipefy_table_field.test", tfjsonpath.New(attr), knownvalue.StringExact(val))
}

func expectTableFieldList(attr string, vals ...string) statecheck.StateCheck {
	checks := make([]knownvalue.Check, len(vals))
	for i, v := range vals {
		checks[i] = knownvalue.StringExact(v)
	}
	return statecheck.ExpectKnownValue("pipefy_table_field.test", tfjsonpath.New(attr), knownvalue.ListExact(checks))
}

var planTableFieldUpdate = resource.ConfigPlanChecks{
	PreApply: []plancheck.PlanCheck{
		plancheck.ExpectResourceAction("pipefy_table_field.test", plancheck.ResourceActionUpdate),
	},
}

func TestUnit_TableFieldResource_CRUD(t *testing.T) {
	st := &tableFieldState{}
	srv := httptest.NewServer(tableFieldMockHandler(st))
	defer srv.Close()

	create := tableFieldConfig(srv.URL, `
resource "pipefy_table_field" "test" {
  table_id = pipefy_table.t.id
  type     = "checklist_vertical"
  label    = "Approved?"
  options  = ["Sim", "Não"]
}
`)
	update := strings.ReplaceAll(create, `["Sim", "Não"]`, `["Sim", "Não", "Talvez"]`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: create,
				ConfigStateChecks: []statecheck.StateCheck{
					expectTableFieldStr("internal_id", "789"),
					expectTableFieldStr("uuid", "tfield-uuid-1"),
					expectTableFieldList("options", "Sim", "Não"),
				},
			},
			{
				Config:           update,
				ConfigPlanChecks: planTableFieldUpdate,
				ConfigStateChecks: []statecheck.StateCheck{
					expectTableFieldStr("internal_id", "789"),
					expectTableFieldStr("uuid", "tfield-uuid-1"),
					expectTableFieldList("options", "Sim", "Não", "Talvez"),
				},
			},
			{Config: tableFieldConfig(srv.URL, "")},
		},
	})

	if st.deletedCt == 0 {
		t.Fatal("expected delete mutation to be called")
	}
}

func TestUnit_TableFieldResource_SchemaAttributes(t *testing.T) {
	st := &tableFieldState{}
	srv := httptest.NewServer(tableFieldMockHandler(st))
	defer srv.Close()

	create := tableFieldConfig(srv.URL, `
resource "pipefy_table_field" "test" {
  table_id          = pipefy_table.t.id
  type              = "short_text"
  label             = "Name"
  required          = true
  unique            = true
  description       = "The record's display name"
  help              = "Enter a short, clear name"
  minimal_view      = false
  custom_validation = "min:3"
}
`)
	update := strings.ReplaceAll(create, `"Enter a short, clear name"`, `"Give it a unique name"`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: create,
				ConfigStateChecks: []statecheck.StateCheck{
					expectTableFieldStr("description", "The record's display name"),
					expectTableFieldStr("help", "Enter a short, clear name"),
					expectTableFieldStr("custom_validation", "min:3"),
					statecheck.ExpectKnownValue("pipefy_table_field.test", tfjsonpath.New("required"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("pipefy_table_field.test", tfjsonpath.New("unique"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("pipefy_table_field.test", tfjsonpath.New("minimal_view"), knownvalue.Bool(false)),
				},
			},
			{
				Config:           update,
				ConfigPlanChecks: planTableFieldUpdate,
				ConfigStateChecks: []statecheck.StateCheck{
					expectTableFieldStr("help", "Give it a unique name"),
				},
			},
		},
	})
}

func TestUnit_TableFieldResource_ReadRefreshDetectsDrift(t *testing.T) {
	st := &tableFieldState{}
	srv := httptest.NewServer(tableFieldMockHandler(st))
	defer srv.Close()

	cfg := tableFieldConfig(srv.URL, `
resource "pipefy_table_field" "test" {
  table_id    = pipefy_table.t.id
  type        = "short_text"
  label       = "Name"
  required    = true
  description = "original"
}
`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					drift := "changed by someone else"
					st.description = &drift
					no := false
					st.required = &no
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// table_id is a literal here: the mock ignores it on read, so the import ID
// round-trips without depending on the table scaffolding (ids are empty here).
func TestUnit_TableFieldResource_ImportState(t *testing.T) {
	st := &tableFieldState{}
	srv := httptest.NewServer(tableFieldMockHandler(st))
	defer srv.Close()

	cfg := `
provider "pipefy" {
  endpoint = "` + srv.URL + `"
  token    = "testtoken"
}

resource "pipefy_table_field" "test" {
  table_id    = "table_1"
  type        = "short_text"
  label       = "Name"
  required    = true
  description = "desc"
}
`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				ResourceName: "pipefy_table_field.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["pipefy_table_field.test"]
					return rs.Primary.Attributes["table_id"] + "/" + rs.Primary.Attributes["uuid"], nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

type tableFieldCollision struct {
	id, internalID, uuid, label string
}

type tableFieldCollisionState struct {
	ghost, managed tableFieldCollision
	lastUpdateID   string
}

func tableFieldCollisionJSON(f tableFieldCollision) string {
	return `{"id":"` + f.id + `","internal_id":"` + f.internalID +
		`","uuid":"` + f.uuid + `","label":"` + f.label + `","type":"short_text","options":null}`
}

func tableFieldCollisionMockHandler(st *tableFieldCollisionState) http.HandlerFunc {
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

		switch q := gr.Query; {
		case strings.Contains(q, "createTableField"):
			st.managed.label, _ = gr.Variables["label"].(string)
			_, _ = io.WriteString(w, `{"data":{"createTableField":{"table_field":`+tableFieldCollisionJSON(st.managed)+`}}}`)
		case strings.Contains(q, "updateTableField"):
			id, _ := gr.Variables["id"].(string)
			st.lastUpdateID = id
			target := &st.ghost
			if id == st.managed.id {
				target = &st.managed
			}
			if v, ok := gr.Variables["label"].(string); ok {
				target.label = v
			}
			_, _ = io.WriteString(w, `{"data":{"updateTableField":{"table_field":`+tableFieldCollisionJSON(*target)+`}}}`)
		case strings.Contains(q, "deleteTableField"):
			_, _ = io.WriteString(w, `{"data":{"deleteTableField":{"success":true}}}`)
		case strings.Contains(q, "table("):
			// Table-scoped read returns only the managed field; the ghost is in another table.
			_, _ = io.WriteString(w, `{"data":{"table":{"table_fields":[`+tableFieldCollisionJSON(st.managed)+`]}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}
}

func TestUnit_TableFieldResource_UpdateTargetsByUuid(t *testing.T) {
	st := &tableFieldCollisionState{
		ghost:   tableFieldCollision{id: "ghost_id", internalID: "481", uuid: "uuid-ghost", label: "Trigger"},
		managed: tableFieldCollision{id: "managed_id", internalID: "485", uuid: "uuid-managed", label: "Trigger"},
	}
	srv := httptest.NewServer(tableFieldCollisionMockHandler(st))
	defer srv.Close()

	config := tableFieldConfig(srv.URL, `
resource "pipefy_table_field" "test" {
  table_id = pipefy_table.t.id
  type     = "short_text"
  label    = "Trigger"
}
`)
	renamed := strings.ReplaceAll(config, `"Trigger"`, `"Trigger renamed"`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					expectTableFieldStr("internal_id", "485"),
					expectTableFieldStr("uuid", "uuid-managed"),
				},
			},
			{
				Config:           renamed,
				ConfigPlanChecks: planTableFieldUpdate,
				ConfigStateChecks: []statecheck.StateCheck{
					expectTableFieldStr("internal_id", "485"), // the managed field, not the ghost's 481
				},
			},
		},
	})

	if st.lastUpdateID != "managed_id" {
		t.Fatalf("update must target the managed field by id, got %q", st.lastUpdateID)
	}
	if st.ghost.label != "Trigger" {
		t.Fatalf("update retargeted the colliding field, its label is now %q", st.ghost.label)
	}
}
