// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

type fieldState struct {
	id, internalID, uuid, label, fieldType, optionsJSON string
	required                                            *bool
	description, help, customValidation                 *string
	editable, minimalView                               *bool
	index                                               *float64
	created                                             bool
	deletedCt                                           int
}

func optionsJSON(vars map[string]any, fallback string) string {
	if opts, ok := vars["options"]; ok {
		b, _ := json.Marshal(opts)
		return string(b)
	}
	return fallback
}

func jsonStr(p *string) string {
	if p == nil {
		return "null"
	}
	b, _ := json.Marshal(*p)
	return string(b)
}

func jsonBool(p *bool) string {
	if p == nil {
		return "null"
	}
	if *p {
		return "true"
	}
	return "false"
}

func jsonNum(p *float64) string {
	if p == nil {
		return "null"
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

func varBool(vars map[string]any, k string) *bool {
	if v, ok := vars[k].(bool); ok {
		return &v
	}
	return nil
}

func varStr(vars map[string]any, k string) *string {
	if v, ok := vars[k].(string); ok {
		return &v
	}
	return nil
}

// varCustomValidation models a written "" coming back as null. A non-empty
// rule on a number field is dropped: that type does not honour the attribute.
func varCustomValidation(vars map[string]any, fieldType string) *string {
	v, ok := vars["customValidation"].(string)
	if !ok || v == "" {
		return nil
	}
	if fieldType == "number" {
		return nil
	}
	return &v
}

func varNum(vars map[string]any, k string) *float64 {
	if v, ok := vars[k].(float64); ok {
		return &v
	}
	return nil
}

func fieldObj(st *fieldState) string {
	return `{"id":"` + st.id + `","internal_id":"` + st.internalID +
		`","uuid":"` + st.uuid + `","label":"` + st.label +
		`","type":"` + st.fieldType +
		`","required":` + jsonBool(st.required) +
		`,"options":` + st.optionsJSON +
		`,"description":` + jsonStr(st.description) +
		`,"help":` + jsonStr(st.help) +
		`,"editable":` + jsonBool(st.editable) +
		`,"minimal_view":` + jsonBool(st.minimalView) +
		`,"custom_validation":` + jsonStr(st.customValidation) +
		`,"index":` + jsonNum(st.index) + `}`
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
			st.fieldType, _ = gr.Variables["type"].(string)
			st.required = varBool(gr.Variables, "required")
			st.description = varStr(gr.Variables, "description")
			st.help = varStr(gr.Variables, "help")
			st.editable = varBool(gr.Variables, "editable")
			st.minimalView = varBool(gr.Variables, "minimalView")
			st.customValidation = varCustomValidation(gr.Variables, st.fieldType)
			st.index = varNum(gr.Variables, "index")
			if st.index == nil {
				def := 1.5
				st.index = &def
			}
			st.optionsJSON = optionsJSON(gr.Variables, "null")
			st.created = true
			_, _ = io.WriteString(w, `{"data":{"createPhaseField":{"phase_field":`+fieldObj(st)+`}}}`)
		case strings.Contains(q, "updatePhaseField"):
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
			if p := varBool(gr.Variables, "editable"); p != nil {
				st.editable = p
			}
			if p := varBool(gr.Variables, "minimalView"); p != nil {
				st.minimalView = p
			}
			if _, sent := gr.Variables["customValidation"]; sent {
				st.customValidation = varCustomValidation(gr.Variables, st.fieldType)
			}
			if p := varNum(gr.Variables, "index"); p != nil {
				st.index = p
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

func TestUnit_FieldResource_SchemaAttributes(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	create := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id          = pipefy_phase.ph.id
  type              = "short_text"
  label             = "Title"
  required          = true
  description       = "The card title"
  help              = "Enter a short title"
  editable          = true
  minimal_view      = false
  custom_validation = "min:3"
  index             = 2.5
}
`)
	update := strings.ReplaceAll(create, `"Enter a short title"`, `"Give it a clear name"`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: create,
				ConfigStateChecks: []statecheck.StateCheck{
					expectStr("description", "The card title"),
					expectStr("help", "Enter a short title"),
					expectStr("custom_validation", "min:3"),
					statecheck.ExpectKnownValue("pipefy_field.test", tfjsonpath.New("editable"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("pipefy_field.test", tfjsonpath.New("minimal_view"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue("pipefy_field.test", tfjsonpath.New("required"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("pipefy_field.test", tfjsonpath.New("index"), knownvalue.Float64Exact(2.5)),
				},
			},
			{
				Config:           update,
				ConfigPlanChecks: planUpdate,
				ConfigStateChecks: []statecheck.StateCheck{
					expectStr("help", "Give it a clear name"),
				},
			},
		},
	})
}

func TestUnit_FieldResource_ReadRefreshDetectsDrift(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	cfg := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id    = pipefy_phase.ph.id
  type        = "short_text"
  label       = "Title"
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

// The web UI writes "" for custom_validation when someone edits a field, but a
// write of "" is stored as NULL. Refresh therefore seeds "" into state, the next
// plan carries it forward, and the update response comes back null. Both values
// mean "no rule", so neither the refresh nor the apply may report a change.
func TestUnit_FieldResource_EmptyCustomValidationFromUIEdit(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	cfg := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id = pipefy_phase.ph.id
  type     = "radio_vertical"
  label    = "Budget confirmed"
  options  = ["Yes", "No"]
}
`)
	withOption := strings.ReplaceAll(cfg, `["Yes", "No"]`, `["Yes", "No", "Unknown"]`)

	uiEdit := func() {
		empty := ""
		st.customValidation = &empty
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig:         uiEdit,
				Config:            cfg,
				ConfigPlanChecks:  resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				ConfigStateChecks: []statecheck.StateCheck{statecheck.ExpectKnownValue("pipefy_field.test", tfjsonpath.New("custom_validation"), knownvalue.Null())},
			},
			{
				PreConfig:         uiEdit,
				Config:            withOption,
				ConfigPlanChecks:  planUpdate,
				ConfigStateChecks: []statecheck.StateCheck{expectList("options", "Yes", "No", "Unknown")},
			},
		},
	})
}

// The empty-or-null merge is scoped to custom_validation. description and help
// store "" verbatim on the API side, so a UI edit that blanks them is a real
// value the refresh has to reflect. This fails the moment someone widens the
// merge into a general "empty string means null" rule.
func TestUnit_FieldResource_EmptyDescriptionAndHelpSurviveRefresh(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	cfg := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id = pipefy_phase.ph.id
  type     = "short_text"
  label    = "Title"
}
`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					empty := ""
					st.description = &empty
					st.help = &empty
				},
				Config:           cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				ConfigStateChecks: []statecheck.StateCheck{
					expectStr("description", ""),
					expectStr("help", ""),
				},
			},
		},
	})
}

// The merge keeps empty and null interchangeable, and nothing else: a rule that
// appears on the server side is drift the refresh must report. Config holds a
// different rule so the plan is non-empty; omitting the attribute would let
// Optional+Computed swallow the API value with an empty plan.
func TestUnit_FieldResource_CustomValidationDriftDetected(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	cfg := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id          = pipefy_phase.ph.id
  type              = "short_text"
  label             = "Title"
  custom_validation = "min:1"
}
`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:            cfg,
				ConfigStateChecks: []statecheck.StateCheck{expectStr("custom_validation", "min:1")},
			},
			{
				PreConfig: func() {
					rule := "min:3"
					st.customValidation = &rule
				},
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigStateChecks:  []statecheck.StateCheck{expectStr("custom_validation", "min:3")},
			},
		},
	})
}

func TestUnit_FieldResource_EmptyCustomValidationFromConfig(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	cfg := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id          = pipefy_phase.ph.id
  type              = "short_text"
  label             = "Title"
  custom_validation = ""
}
`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:            cfg,
				ConfigStateChecks: []statecheck.StateCheck{expectStr("custom_validation", "")},
			},
			{
				Config:           cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

// Keeping the planned value would convert a dropped rule into a perpetual plan
// with no diagnostic.
func TestUnit_FieldResource_UnsupportedCustomValidationRejectedAfterApply(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	cfg := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id          = pipefy_phase.ph.id
  type              = "number"
  label             = "Amount"
  custom_validation = "min:3"
}
`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      cfg,
				ExpectError: regexp.MustCompile(`inconsistent result after apply`),
			},
		},
	})
}

func TestUnit_FieldResource_ComputedIndexNoPerpetualDiff(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	cfg := fieldConfig(srv.URL, `
resource "pipefy_field" "test" {
  phase_id = pipefy_phase.ph.id
  type     = "short_text"
  label    = "Title"
}
`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   skipBelow18,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_field.test", tfjsonpath.New("index"), knownvalue.Float64Exact(1.5)),
				},
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// phase_id is a literal here: the mock ignores it on read, so the import ID
// round-trips without depending on the pipe/phase scaffolding (ids are empty here).
func TestUnit_FieldResource_ImportState(t *testing.T) {
	st := &fieldState{}
	srv := httptest.NewServer(fieldMockHandler(st))
	defer srv.Close()

	cfg := `
provider "pipefy" {
  endpoint = "` + srv.URL + `"
  token    = "testtoken"
}

resource "pipefy_field" "test" {
  phase_id    = "phase_1"
  type        = "short_text"
  label       = "Title"
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
				ResourceName: "pipefy_field.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["pipefy_field.test"]
					return rs.Primary.Attributes["phase_id"] + "/" + rs.Primary.Attributes["uuid"], nil
				},
				ImportStateVerify: true,
			},
		},
	})
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
		`","uuid":"` + f.uuid + `","label":"` + f.label + `","type":"short_text","options":null}`
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
