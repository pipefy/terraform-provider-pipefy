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
	id, internalID, uuid, label, optionsJSON string
	created                                  bool
	deletedCt                                int
}

func optionsJSON(vars map[string]any, fallback string) string {
	if opts, ok := vars["options"]; ok {
		b, _ := json.Marshal(opts)
		return string(b)
	}
	return fallback
}

func fieldObj(st *fieldState) string {
	return `{"id":"` + st.id + `","internal_id":"` + st.internalID +
		`","uuid":"` + st.uuid + `","label":"` + st.label +
		`","options":` + st.optionsJSON + `}`
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

		switch q := gr.Query; {
		case strings.Contains(q, "createPhaseField"):
			st.id, st.internalID, st.uuid = "field_123", "456", "field-uuid-1"
			st.label, _ = gr.Variables["label"].(string)
			st.optionsJSON = optionsJSON(gr.Variables, "null")
			st.created = true
			_, _ = io.WriteString(w, `{"data":{"createPhaseField":{"phase_field":`+fieldObj(st)+`}}}`)
		case strings.Contains(q, "updatePhaseField"):
			if v, ok := gr.Variables["label"].(string); ok {
				st.label = v
			}
			st.optionsJSON = optionsJSON(gr.Variables, st.optionsJSON)
			_, _ = io.WriteString(w, `{"data":{"updatePhaseField":{"phase_field":`+fieldObj(st)+`}}}`)
		case strings.Contains(q, "deletePhaseField"):
			st.deletedCt++
			_, _ = io.WriteString(w, `{"data":{"deletePhaseField":{"success":true}}}`)
		case strings.Contains(q, "repo_id"):
			_, _ = io.WriteString(w, `{"data":{"phase":{"repo_id":123}}}`)
		case strings.Contains(q, "pipe("):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"uuid":"pipe-uuid-1"}}}`)
		case strings.Contains(q, "phase("):
			fields := ""
			if st.created {
				fields = fieldObj(st)
			}
			_, _ = io.WriteString(w, `{"data":{"phase":{"fields":[`+fields+`]}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}
}

func fieldConfig(srvURL, fieldBlock string) string {
	return `
provider "pipefy" {
  endpoint = "` + srvURL + `"
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
` + fieldBlock
}

var skipBelow18 = []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)}

var planUpdate = resource.ConfigPlanChecks{
	PreApply: []plancheck.PlanCheck{
		plancheck.ExpectResourceAction("pipefy_field.test", plancheck.ResourceActionUpdate),
	},
}

func expectStr(attr, val string) statecheck.StateCheck {
	return statecheck.ExpectKnownValue("pipefy_field.test", tfjsonpath.New(attr), knownvalue.StringExact(val))
}

func expectList(attr string, vals ...string) statecheck.StateCheck {
	checks := make([]knownvalue.Check, len(vals))
	for i, v := range vals {
		checks[i] = knownvalue.StringExact(v)
	}
	return statecheck.ExpectKnownValue("pipefy_field.test", tfjsonpath.New(attr), knownvalue.ListExact(checks))
}

func TestUnit_FieldResource_CRUD(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	create := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id = pipefy_phase.ph.id
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
					expectStr("internal_id", "456"),
					expectStr("uuid", "field-uuid-1"),
					expectList("options", "Sim", "Não"),
				},
			},
			{
				Config:           update,
				ConfigPlanChecks: planUpdate,
				ConfigStateChecks: []statecheck.StateCheck{
					expectStr("internal_id", "456"),
					expectStr("uuid", "field-uuid-1"),
					expectList("options", "Sim", "Não", "Talvez"),
				},
			},
			{Config: fieldConfig(srv.URL, "")},
		},
	})

	if st.deletedCt == 0 {
		t.Fatal("expected delete mutation to be called")
	}
}

type collisionField struct {
	id, internalID, uuid, label string
}

type collisionState struct {
	ghost, managed collisionField
	lastUpdateUUID string
}

func collisionFieldJSON(f collisionField) string {
	return `{"id":"` + f.id + `","internal_id":"` + f.internalID +
		`","uuid":"` + f.uuid + `","label":"` + f.label + `","options":null}`
}

func collisionMockHandler(st *collisionState) http.HandlerFunc {
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
		case strings.Contains(q, "createPhaseField"):
			st.managed.label, _ = gr.Variables["label"].(string)
			_, _ = io.WriteString(w, `{"data":{"createPhaseField":{"phase_field":`+collisionFieldJSON(st.managed)+`}}}`)
		case strings.Contains(q, "updatePhaseField"):
			uuid, _ := gr.Variables["uuid"].(string)
			st.lastUpdateUUID = uuid
			target := &st.ghost
			if uuid == st.managed.uuid {
				target = &st.managed
			}
			if v, ok := gr.Variables["label"].(string); ok {
				target.label = v
			}
			_, _ = io.WriteString(w, `{"data":{"updatePhaseField":{"phase_field":`+collisionFieldJSON(*target)+`}}}`)
		case strings.Contains(q, "deletePhaseField"):
			_, _ = io.WriteString(w, `{"data":{"deletePhaseField":{"success":true}}}`)
		case strings.Contains(q, "repo_id"):
			_, _ = io.WriteString(w, `{"data":{"phase":{"repo_id":123}}}`)
		case strings.Contains(q, "pipe("):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"uuid":"pipe-uuid-1"}}}`)
		case strings.Contains(q, "phase("):
			// Phase-scoped read returns only the managed field; the ghost is in another pipe.
			_, _ = io.WriteString(w, `{"data":{"phase":{"fields":[`+collisionFieldJSON(st.managed)+`]}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}
}

func TestUnit_FieldResource_UpdateTargetsByUuid(t *testing.T) {
	st := &collisionState{
		ghost:   collisionField{id: "trigger", internalID: "481", uuid: "uuid-ghost", label: "Trigger"},
		managed: collisionField{id: "trigger", internalID: "485", uuid: "uuid-managed", label: "Trigger"},
	}
	srv := httptest.NewServer(collisionMockHandler(st))
	defer srv.Close()

	config := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id = pipefy_phase.ph.id
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
					expectStr("internal_id", "485"),
					expectStr("uuid", "uuid-managed"),
				},
			},
			{
				Config:           renamed,
				ConfigPlanChecks: planUpdate,
				ConfigStateChecks: []statecheck.StateCheck{
					expectStr("internal_id", "485"), // the managed field, not the ghost's 481
				},
			},
		},
	})

	if st.lastUpdateUUID != "uuid-managed" {
		t.Fatalf("update must target the managed field by uuid, got %q", st.lastUpdateUUID)
	}
	if st.ghost.label != "Trigger" {
		t.Fatalf("update retargeted the colliding field, its label is now %q", st.ghost.label)
	}
}
