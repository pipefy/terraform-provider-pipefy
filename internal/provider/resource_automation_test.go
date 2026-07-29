// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"encoding/json"
	"fmt"
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
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

type automationState struct {
	ID                  string
	Name                string
	Active              bool
	EventID             string
	ActionID            string
	EventRepo           string
	ActionRepo          string
	SchedulerFrequency  string
	SchedulerCron       json.RawMessage
	SearchFor           json.RawMessage
	ResponseSchema      json.RawMessage
	EventParams         json.RawMessage
	Condition           json.RawMessage
	CreatedEventParams  any
	CreatedActionParams any
	UpdatedEventParams  any
	UpdatedActionParams any
	DeletedCt           int
}

// automationCapture records a mutation's input so the read handler can echo it,
// mirroring how the real API round-trips the settable attributes.
func automationCapture(st *automationState, in map[string]any) {
	if v, ok := in["name"].(string); ok {
		st.Name = v
	}
	if v, ok := in["active"].(bool); ok {
		st.Active = v
	}
	if v, ok := in["event_id"].(string); ok {
		st.EventID = v
	}
	if v, ok := in["action_id"].(string); ok {
		st.ActionID = v
	}
	if v, ok := in["event_repo_id"].(string); ok {
		st.EventRepo = v
	}
	if v, ok := in["action_repo_id"].(string); ok {
		st.ActionRepo = v
	}
	if v, ok := in["scheduler_frequency"].(string); ok {
		st.SchedulerFrequency = v
	}
	if v, ok := in["schedulerCron"]; ok {
		st.SchedulerCron, _ = json.Marshal(v)
	}
	if v, ok := in["searchFor"]; ok {
		st.SearchFor, _ = json.Marshal(v)
	}
	if v, ok := in["responseSchema"]; ok {
		st.ResponseSchema, _ = json.Marshal(v)
	}
	if v, ok := in["event_params"]; ok {
		st.EventParams, _ = json.Marshal(v)
	}
	if v, ok := in["condition"]; ok {
		st.Condition, _ = json.Marshal(v)
	}
}

// writeAutomationRead returns the automation read payload built from the mock
// state, matching the shape of the real automation(id:) query.
func writeAutomationRead(w http.ResponseWriter, st *automationState) {
	auto := map[string]any{
		"id":             st.ID,
		"name":           st.Name,
		"active":         st.Active,
		"event_id":       st.EventID,
		"action_id":      st.ActionID,
		"event_repo":     map[string]any{"id": st.EventRepo},
		"action_repo_v2": map[string]any{"id": st.ActionRepo},
	}
	if st.SchedulerFrequency != "" {
		auto["scheduler_frequency"] = st.SchedulerFrequency
	} else {
		auto["scheduler_frequency"] = nil
	}
	if len(st.SchedulerCron) > 0 {
		auto["schedulerCron"] = st.SchedulerCron
	} else {
		auto["schedulerCron"] = nil
	}
	if len(st.SearchFor) > 0 {
		auto["searchFor"] = st.SearchFor
	} else {
		auto["searchFor"] = json.RawMessage("[]")
	}
	if len(st.ResponseSchema) > 0 && string(st.ResponseSchema) != "null" {
		auto["responseSchema"] = st.ResponseSchema
	} else {
		auto["responseSchema"] = nil
	}
	if len(st.EventParams) > 0 {
		auto["event_params"] = st.EventParams
	} else {
		auto["event_params"] = nil
	}
	if len(st.Condition) > 0 {
		auto["condition"] = st.Condition
	} else {
		auto["condition"] = nil
	}
	body, _ := json.Marshal(map[string]any{"data": map[string]any{"automation": auto}})
	_, _ = w.Write(body)
}

// newAutomationServer is a stateful mock: it stores each mutation's input and
// echoes it on read, so a refresh reflects what was written (round-trip) unless
// a test mutates the state out of band (drift).
func newAutomationServer(st *automationState) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		in, _ := gr.Variables["input"].(map[string]any)
		switch q := gr.Query; {
		case strings.Contains(q, "createAutomation"):
			if st.ID == "" {
				st.ID = "auto_1"
			}
			if in != nil {
				automationCapture(st, in)
				st.CreatedEventParams = in["event_params"]
				st.CreatedActionParams = in["action_params"]
			}
			_, _ = io.WriteString(w, `{"data":{"createAutomation":{"automation":{"id":"`+st.ID+`"}}}}`)
		case strings.Contains(q, "updateAutomation"):
			if in != nil {
				automationCapture(st, in)
				st.UpdatedEventParams = in["event_params"]
				st.UpdatedActionParams = in["action_params"]
			}
			_, _ = io.WriteString(w, `{"data":{"updateAutomation":{"automation":{"id":"`+st.ID+`"}}}}`)
		case strings.Contains(q, "deleteAutomation"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteAutomation":{"success":true}}}`)
		case strings.Contains(q, "automation("):
			writeAutomationRead(w, st)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
}

func TestUnit_AutomationResource_CRUD(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "When Summary changes, generate AI output"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		event_params = {
			trigger_field_ids = ["420173505"]
		}

		action_params = jsonencode({
			aiParams = {
				value    = "Rewrite this more clearly:\n\n%%{420173505}"
				fieldIds = [420173432]
			}
		})
	}
	`

	configUpdate := strings.ReplaceAll(config, "When Summary changes, generate AI output", "Renamed Automation")
	configUpdate = strings.ReplaceAll(configUpdate, "420173505", "420173506")

	configDestroy := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		count          = 0
		name           = "When Summary changes, generate AI output"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
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
						"pipefy_automation.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("auto_1"),
					),
				},
			},
			{
				Config: configUpdate,
			},
			{
				Config: configDestroy,
			},
		},
	})

	if st.CreatedEventParams == nil {
		t.Fatalf("expected event_params to be sent on create")
	}
	if st.CreatedActionParams == nil {
		t.Fatalf("expected action_params to be sent on create")
	}
	if st.UpdatedEventParams == nil {
		t.Fatalf("expected event_params to be sent on update")
	}
	if st.UpdatedActionParams == nil {
		t.Fatalf("expected action_params to be sent on update")
	}
	if st.DeletedCt == 0 {
		t.Fatalf("expected delete mutation to be called")
	}
}

func TestUnit_AutomationResource_CreateSurfacesErrorDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(gr.Query, "createAutomation") {
			_, _ = io.WriteString(w, `{"errors":[{"message":"All fields must be filled properly."}],"data":{"createAutomation":{"automation":null,"error_details":[{"object_name":"field_map","object_key":"420173432","messages":["can't be blank"]}]}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Invalid automation"
		event_id       = "field_updated"
		action_id      = "update_card_field"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		action_params = jsonencode({
			field_map = [{ fieldId = "420173432", inputMode = "fixed_value", value = "" }]
		})
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`field_map.*can't be blank`),
			},
		},
	})
}

func TestUnit_AutomationResource_CreateSurfacesTopLevelErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(gr.Query, "createAutomation") {
			_, _ = io.WriteString(w, `{"errors":[{"message":"Name can't be blank"},{"message":"Event is invalid"}],"data":{"createAutomation":{"automation":null}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Invalid automation"
		event_id       = "field_updated"
		action_id      = "update_card_field"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`Name can't be blank; Event is invalid`),
			},
		},
	})
}

func TestUnit_AutomationResource_CreateJoinsErrorDetailMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(gr.Query, "createAutomation") {
			_, _ = io.WriteString(w, `{"errors":[{"message":"All fields must be filled properly."}],"data":{"createAutomation":{"automation":null,"error_details":[{"object_name":"field_map","object_key":"420173432","messages":["can't be blank","is invalid"]}]}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Invalid automation"
		event_id       = "field_updated"
		action_id      = "update_card_field"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`field_map \(420173432\): can't be blank; is invalid`),
			},
		},
	})
}

func TestUnit_AutomationResource_CreateFallsBackToTopLevelErrorWhenDetailsBlank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(gr.Query, "createAutomation") {
			_, _ = io.WriteString(w, `{"errors":[{"message":"All fields must be filled properly."}],"data":{"createAutomation":{"automation":null,"error_details":[{"object_name":"","object_key":"","messages":[]}]}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Invalid automation"
		event_id       = "field_updated"
		action_id      = "update_card_field"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`All fields must be filled properly`),
			},
		},
	})
}

func TestUnit_AutomationResource_CreateReportsGenericErrorWhenNoAutomationOrDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(gr.Query, "createAutomation") {
			_, _ = io.WriteString(w, `{"data":{"createAutomation":{"automation":null}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Invalid automation"
		event_id       = "field_updated"
		action_id      = "update_card_field"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`the API returned no automation and no error_details`),
			},
		},
	})
}

func TestUnit_AutomationResource_CreatePersistsStateWhenAutomationReturnedWithErrorDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")

		switch q := gr.Query; {
		case strings.Contains(q, "createAutomation"):
			_, _ = io.WriteString(w, `{"data":{"createAutomation":{"automation":{"id":"auto_1","name":"Auto","action_id":"generate_with_ai","event_id":"field_updated","active":true},"error_details":[{"object_name":"field_map","object_key":"420173432","messages":["can't be blank"]}]}}}`)
		case strings.Contains(q, "automation("):
			_, _ = io.WriteString(w, `{"data":{"automation":{"id":"auto_1","name":"Auto","action_id":"generate_with_ai","event_id":"field_updated","active":true,"event_repo":{"id":"306729113"},"action_repo_v2":{"id":"306729113"}}}}`)
		case strings.Contains(q, "deleteAutomation"):
			_, _ = io.WriteString(w, `{"data":{"deleteAutomation":{"success":true}}}`)
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

	resource "pipefy_automation" "test" {
		name           = "Auto"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
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
						"pipefy_automation.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("auto_1"),
					),
				},
			},
		},
	})
}

func TestUnit_AutomationResource_CreateRejectsInvalidActionParamsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Auto"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
		action_params  = "{not valid json"
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`invalid action_params JSON`),
			},
		},
	})
}

func TestUnit_AutomationResource_UpdateSurfacesTopLevelErrors(t *testing.T) {
	failUpdate := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")

		switch q := gr.Query; {
		case strings.Contains(q, "createAutomation"):
			_, _ = io.WriteString(w, `{"data":{"createAutomation":{"automation":{"id":"auto_1","name":"Auto","action_id":"generate_with_ai","event_id":"field_updated","active":true}}}}`)
		case strings.Contains(q, "updateAutomation"):
			if failUpdate {
				_, _ = io.WriteString(w, `{"errors":[{"message":"Name can't be blank"},{"message":"Event is invalid"}],"data":{"updateAutomation":{"automation":null}}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"updateAutomation":{"automation":{"id":"auto_1"}}}}`)
		case strings.Contains(q, "deleteAutomation"):
			_, _ = io.WriteString(w, `{"data":{"deleteAutomation":{"success":true}}}`)
		case strings.Contains(q, "automation("):
			_, _ = io.WriteString(w, `{"data":{"automation":{"id":"auto_1","name":"Auto","action_id":"generate_with_ai","event_id":"field_updated","active":true,"event_repo":{"id":"306729113"},"action_repo_v2":{"id":"306729113"}}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()

	base := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "NAME"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
	}
	`
	config := strings.ReplaceAll(base, "NAME", "Auto")
	configUpdate := strings.ReplaceAll(base, "NAME", "Renamed Auto")

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				PreConfig:   func() { failUpdate = true },
				Config:      configUpdate,
				ExpectError: regexp.MustCompile(`Name can't be blank; Event is invalid`),
			},
		},
	})
}

func TestUnit_AutomationResource_UpdateSurfacesErrorDetails(t *testing.T) {
	failUpdate := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")

		switch q := gr.Query; {
		case strings.Contains(q, "createAutomation"):
			_, _ = io.WriteString(w, `{"data":{"createAutomation":{"automation":{"id":"auto_1","name":"Auto","action_id":"generate_with_ai","event_id":"field_updated","active":true}}}}`)
		case strings.Contains(q, "updateAutomation"):
			if failUpdate {
				_, _ = io.WriteString(w, `{"errors":[{"message":"All fields must be filled properly."}],"data":{"updateAutomation":{"automation":null,"error_details":[{"object_name":"field_map","object_key":"420173432","messages":["can't be blank"]}]}}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"updateAutomation":{"automation":{"id":"auto_1"}}}}`)
		case strings.Contains(q, "deleteAutomation"):
			_, _ = io.WriteString(w, `{"data":{"deleteAutomation":{"success":true}}}`)
		case strings.Contains(q, "automation("):
			_, _ = io.WriteString(w, `{"data":{"automation":{"id":"auto_1","name":"Auto","action_id":"generate_with_ai","event_id":"field_updated","active":true,"event_repo":{"id":"306729113"},"action_repo_v2":{"id":"306729113"}}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()

	base := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "NAME"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
	}
	`
	config := strings.ReplaceAll(base, "NAME", "Auto")
	configUpdate := strings.ReplaceAll(base, "NAME", "Renamed Auto")

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				PreConfig:   func() { failUpdate = true },
				Config:      configUpdate,
				ExpectError: regexp.MustCompile(`field_map.*can't be blank`),
			},
		},
	})
}

func TestUnit_AutomationResource_RoundTripSchedulerSearchResponse(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Recurring move"
		event_id       = "scheduler"
		action_id      = "move_multiple_cards"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		scheduler_frequency = "daily"

		scheduler_cron = {
			minute       = "30"
			hour         = "9"
			day_of_month = "*"
			month        = "*"
			day_of_week  = "*"
		}

		search_for = [
			{ field = "title", id = "tf-cond-A", operation = "eq", value = "alpha" },
			{ field = "title", id = "tf-cond-B", operation = "eq", value = "bravo" },
			{ field = "title", id = "tf-cond-C", operation = "eq", value = "charlie" },
		]

		response_schema = jsonencode({
			type       = "object"
			properties = { foo = { type = "string" } }
		})
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
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("scheduler_frequency"), knownvalue.StringExact("daily")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("scheduler_cron").AtMapKey("minute"), knownvalue.StringExact("30")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("scheduler_cron").AtMapKey("day_of_week"), knownvalue.StringExact("*")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("search_for").AtSliceIndex(0).AtMapKey("id"), knownvalue.StringExact("tf-cond-A")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("search_for").AtSliceIndex(1).AtMapKey("id"), knownvalue.StringExact("tf-cond-B")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("search_for").AtSliceIndex(2).AtMapKey("id"), knownvalue.StringExact("tf-cond-C")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("search_for").AtSliceIndex(0).AtMapKey("value"), knownvalue.StringExact("alpha")),
				},
			},
			{
				// The four new attributes must round-trip: a re-plan is empty.
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestUnit_AutomationResource_ReadDetectsDrift(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Original name"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
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
			},
			{
				// Someone changes the automation outside Terraform; Read must
				// refresh state so the next plan shows a diff.
				PreConfig:          func() { st.Name = "Changed outside Terraform" },
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestUnit_AutomationResource_EventParamsWriteOnly(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Field updated"
		event_id       = "field_updated"
		action_id      = "move_single_card"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		event_params = {
			trigger_field_ids = ["427453916", "427453917"]
		}
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
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("event_params").AtMapKey("trigger_field_ids").AtSliceIndex(0), knownvalue.StringExact("427453916")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("event_params").AtMapKey("trigger_field_ids").AtSliceIndex(1), knownvalue.StringExact("427453917")),
				},
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// event_params is write-only: an out-of-band change is not read
				// back, so a refresh surfaces no diff (the plan stays empty).
				PreConfig:    func() { st.EventParams = json.RawMessage(`{"triggerFieldIds":["999"]}`) },
				RefreshState: true,
			},
		},
	})
}

func TestUnit_AutomationResource_EventParamsPhaseBasedKeys(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Move between phases"
		event_id       = "card_left_phase"
		action_id      = "move_single_card"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		event_params = {
			from_phase_id         = "111"
			in_phase_id           = "222"
			to_phase_id           = "333"
			trigger_automation_id = "444"
			kind_of_sla           = "expiration"
		}
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
		},
	})

	got := string(st.EventParams)
	for _, want := range []string{
		`"fromPhaseId":"111"`,
		`"inPhaseId":"222"`,
		`"to_phase_id":"333"`,
		`"triggerAutomationId":"444"`,
		`"kindOfSla":"expiration"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("event_params missing %s; got %s", want, got)
		}
	}
}

func TestUnit_AutomationResource_EmptyEventParamsNotSent(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "No event params"
		event_id       = "field_updated"
		action_id      = "move_single_card"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		event_params = {}
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
		},
	})

	if len(st.EventParams) != 0 {
		t.Fatalf("expected no event_params to be sent, got: %s", st.EventParams)
	}
}

func TestUnit_AutomationResource_ConditionRoundTripDriftAndClear(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	base := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Field updated with condition"
		event_id       = "field_updated"
		action_id      = "move_single_card"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		event_params = {
			trigger_field_ids = ["427453916"]
		}
		CONDITION
	}
	`
	withCond := strings.ReplaceAll(base, "CONDITION", `condition = {
			all_of = [
				{ field = "427453916", operation = "equals", value = "a" },
				{ field = "427453917", operation = "equals", value = "b" },
			]
		}`)
	cleared := strings.ReplaceAll(base, "CONDITION", ``)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: withCond,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition").AtMapKey("all_of").AtSliceIndex(0).AtMapKey("field"), knownvalue.StringExact("427453916")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition").AtMapKey("all_of").AtSliceIndex(1).AtMapKey("value"), knownvalue.StringExact("b")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition").AtMapKey("any_of"), knownvalue.Null()),
				},
			},
			{
				Config: withCond,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// ExpectNonEmptyPlan only asserts the refresh noticed something.
				// What Read reconstructs is pinned by
				// TestUnit_AutomationResource_ConditionReadsServerShape.
				PreConfig: func() {
					st.Condition = json.RawMessage(`{"expressions":[{"field_address":"427453916","operation":"equals","value":"CHANGED","structure_id":"5"}],"expressions_structure":[["5"]]}`)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: cleared,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition"), knownvalue.Null()),
				},
			},
		},
	})

	if !strings.Contains(string(st.Condition), `"expressions":[]`) {
		t.Fatalf("expected clear step to send an empty condition, got: %s", st.Condition)
	}
}

// TestUnit_AutomationResource_ConditionReadsServerShape reads back a condition
// the way the server can report it for an unchanged configuration: structure ids
// the provider never assigned, and a "" value on a value-less comparison. Both
// have to settle into the same all_of with an empty plan.
func TestUnit_AutomationResource_ConditionReadsServerShape(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Value-less condition"
		event_id       = "field_updated"
		action_id      = "move_single_card"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		condition = {
			all_of = [
				{ field = "427453916", operation = "equals", value = "a" },
				{ field = "427453917", operation = "present" },
			]
		}
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					st.Condition = json.RawMessage(`{"expressions":[` +
						`{"field_address":"427453916","operation":"equals","structure_id":"31","value":"a"},` +
						`{"field_address":"427453917","operation":"present","structure_id":"32","value":""}` +
						`],"expressions_structure":[["31","32"]]}`)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition").AtMapKey("all_of"), knownvalue.ListSizeExact(2)),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition").AtMapKey("all_of").AtSliceIndex(1).AtMapKey("value"), knownvalue.Null()),
				},
			},
		},
	})
}

// TestUnit_AutomationResource_ConditionAnyOf covers the OR shape: a nested
// all_of of two comparisons, ORed against a plain entry. The mock echoes what
// the provider sent, so the write flattening and the read canonicalization are
// both under test.
func TestUnit_AutomationResource_ConditionAnyOf(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Field updated with any_of condition"
		event_id       = "field_updated"
		action_id      = "move_single_card"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		condition = {
			any_of = [
				{
					all_of = [
						{ field = "427453916", operation = "equals", value = "a" },
						{ field = "427453917", operation = "equals", value = "b" },
					]
				},
				{ field = "427453918", operation = "present" },
			]
		}
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
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition").AtMapKey("any_of").AtSliceIndex(0).AtMapKey("all_of").AtSliceIndex(1).AtMapKey("field"), knownvalue.StringExact("427453917")),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition").AtMapKey("any_of").AtSliceIndex(1).AtMapKey("field"), knownvalue.StringExact("427453918")),
					// present takes no value, so it stays null rather than "".
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition").AtMapKey("any_of").AtSliceIndex(1).AtMapKey("value"), knownvalue.Null()),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("condition").AtMapKey("all_of"), knownvalue.Null()),
				},
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})

	if !strings.Contains(string(st.Condition), `"expressions_structure":[["0","1"],["2"]]`) {
		t.Fatalf("expected the nested all_of and the plain entry to flatten into two groups, got: %s", st.Condition)
	}
}

// TestUnit_AutomationResource_ConditionRejectsUnusableShapes covers what plan
// time rejects: an empty condition, both branches at once, and the shapes that
// come back as something simpler than they went in.
func TestUnit_AutomationResource_ConditionRejectsUnusableShapes(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	base := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Unusable condition"
		event_id       = "field_updated"
		action_id      = "move_single_card"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		condition = {CONDITION}
	}
	`
	with := func(condition string) string { return strings.ReplaceAll(base, "CONDITION", condition) }

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      with(``),
				ExpectError: regexp.MustCompile(`(?i)and only one`),
			},
			{
				Config:      with(`all_of = []`),
				ExpectError: regexp.MustCompile(`(?i)at least 1`),
			},
			{
				// Counts as set for ExactlyOneOf, so it would otherwise reach
				// the API as a condition-clearing payload.
				Config:      with(`any_of = []`),
				ExpectError: regexp.MustCompile(`(?i)Invalid any_of`),
			},
			{
				Config: with(`
			all_of = [{ field = "427453916", operation = "equals" }]
			any_of = [
				{ field = "427453917", operation = "equals" },
				{ field = "427453918", operation = "present" },
			]`),
				ExpectError: regexp.MustCompile(`(?i)and only one`),
			},
			{
				Config:      with(`any_of = [{ field = "427453916", operation = "equals" }]`),
				ExpectError: regexp.MustCompile(`(?i)Invalid any_of`),
			},
			{
				Config: with(`
			any_of = [
				{ all_of = [{ field = "427453916", operation = "equals" }] },
				{ field = "427453918", operation = "present" },
			]`),
				ExpectError: regexp.MustCompile(`(?i)at least 2|redundant`),
			},
			{
				Config: with(`
			any_of = [
				{
					field     = "427453916"
					operation = "equals"
					all_of = [
						{ field = "427453917", operation = "equals" },
						{ field = "427453918", operation = "present" },
					]
				},
				{ field = "427453919", operation = "present" },
			]`),
				ExpectError: regexp.MustCompile(`(?i)Invalid any_of entry`),
			},
		},
	})
}

func TestUnit_AutomationResource_ConditionRejectsBlankComparisonFields(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Blank condition field"
		event_id       = "field_updated"
		action_id      = "move_single_card"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		condition = {
			all_of = [COMPARISON]
		}
	}
	`
	blankField := strings.ReplaceAll(config, "COMPARISON", `{ field = "", operation = "equals" }`)
	blankOperation := strings.ReplaceAll(config, "COMPARISON", `{ field = "427453916", operation = "  " }`)
	// A blank value is rejected rather than sent, because Read normalizes the
	// "" the API returns for value-less operations back to null.
	blankValue := strings.ReplaceAll(config, "COMPARISON", `{ field = "427453916", operation = "equals", value = "" }`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      blankField,
				ExpectError: regexp.MustCompile(`(?i)non-empty|blank`),
			},
			{
				Config:      blankOperation,
				ExpectError: regexp.MustCompile(`(?i)non-empty|blank`),
			},
			{
				Config:      blankValue,
				ExpectError: regexp.MustCompile(`(?i)non-empty|blank`),
			},
		},
	})
}

func TestUnit_AutomationResource_SearchForRejectsBlankField(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Blank search field"
		event_id       = "scheduler"
		action_id      = "move_multiple_cards"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		scheduler_frequency = "daily"

		scheduler_cron = {
			minute       = "30"
			hour         = "9"
			day_of_month = "*"
			month        = "*"
			day_of_week  = "*"
		}

		search_for = [CONDITION]
	}
	`
	blankField := strings.ReplaceAll(config, "CONDITION", `{ field = "", id = "c1", operation = "eq" }`)
	blankOperation := strings.ReplaceAll(config, "CONDITION", `{ field = "title", id = "c1", operation = "\t" }`)
	blankId := strings.ReplaceAll(config, "CONDITION", `{ field = "title", id = "", operation = "eq" }`)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      blankField,
				ExpectError: regexp.MustCompile(`(?i)non-empty|blank`),
			},
			{
				Config:      blankOperation,
				ExpectError: regexp.MustCompile(`(?i)non-empty|blank`),
			},
			{
				Config:      blankId,
				ExpectError: regexp.MustCompile(`(?i)non-empty|blank`),
			},
		},
	})
}

func TestUnit_AutomationResource_SearchForManagedInFull(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	base := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Recurring"
		event_id       = "scheduler"
		action_id      = "move_multiple_cards"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		scheduler_frequency = "daily"

		scheduler_cron = {
			minute       = "30"
			hour         = "9"
			day_of_month = "*"
			month        = "*"
			day_of_week  = "*"
		}
		SEARCH_FOR
	}
	`
	withConditions := strings.ReplaceAll(base, "SEARCH_FOR", `search_for = [
			{ field = "title", id = "c1", operation = "eq", value = "a" },
			{ field = "title", id = "c2", operation = "eq", value = "b" },
		]`)
	cleared := strings.ReplaceAll(base, "SEARCH_FOR", ``)

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: withConditions,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("search_for"), knownvalue.ListSizeExact(2)),
				},
			},
			{
				// Omitting the block clears the conditions on the server: the
				// list is managed in full and settles to null, matching how an
				// unset condition block settles.
				Config: cleared,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("search_for"), knownvalue.Null()),
				},
			},
		},
	})
}

// response_schema is compared semantically: the API echoes the schema back with
// its object keys sorted (the mock marshals the decoded value, mirroring that),
// which reorders them relative to the config. That read-back reordering must not
// surface as a diff. The config's keys are deliberately not in sorted order so
// the read-back differs textually.
func TestUnit_AutomationResource_ResponseSchemaSemanticEquality(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "AI with schema"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		response_schema = "{\"type\":\"object\",\"properties\":{\"foo\":{\"type\":\"string\"},\"bar\":{\"type\":\"number\"}}}"
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				// The refresh reads back the same schema with keys sorted; the
				// semantic comparison must keep the plan empty.
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// scheduler_cron is refreshed on read, so an out-of-band change to the schedule
// is detected as drift.
func TestUnit_AutomationResource_SchedulerCronDetectsDrift(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Recurring move"
		event_id       = "scheduler"
		action_id      = "move_multiple_cards"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		scheduler_frequency = "daily"

		scheduler_cron = {
			minute       = "30"
			hour         = "9"
			day_of_month = "*"
			month        = "*"
			day_of_week  = "*"
		}
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
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("scheduler_cron").AtMapKey("hour"), knownvalue.StringExact("9")),
				},
			},
			{
				// The schedule is changed outside Terraform; a refresh must
				// surface the drift so the next plan is non-empty.
				PreConfig: func() {
					st.SchedulerCron = json.RawMessage(`{"minute":"0","hour":"18","dayOfMonth":"*","month":"*","dayOfWeek":"*"}`)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestUnit_AutomationResource_NonSchedulerSchedulerFieldsPlanEmpty(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "No schedule"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
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
				// Create must not send responseSchema at all when it is unset.
				// The explicit null that clears it belongs to Update only.
				Check: func(*terraform.State) error {
					if len(st.ResponseSchema) != 0 {
						return fmt.Errorf("create sent responseSchema, got %q", string(st.ResponseSchema))
					}
					return nil
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("scheduler_frequency"), knownvalue.Null()),
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("scheduler_cron"), knownvalue.Null()),
				},
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestUnit_AutomationResource_SchedulerFieldsRequiredForSchedulerEvent(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	head := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Recurring move"
		event_id       = "scheduler"
		action_id      = "move_multiple_cards"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
	`
	withSchedule := head + `
		scheduler_frequency = "daily"

		scheduler_cron = {
			minute       = "30"
			hour         = "9"
			day_of_month = "*"
			month        = "*"
			day_of_week  = "*"
		}
	}
	`
	omitted := head + `
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: withSchedule},
			{
				Config:      omitted,
				ExpectError: regexp.MustCompile(`(?s)Missing scheduler_frequency.*Missing scheduler_cron`),
			},
			{
				Config: withSchedule,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestUnit_AutomationResource_ResponseSchemaOmitAfterSetClears(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	head := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "HTTP with schema"
		event_id       = "field_updated"
		action_id      = "send_http_request"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
	`
	withSchema := head + `
		response_schema = jsonencode({ type = "object" })
	}
	`
	omitted := head + `
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: withSchema},
			{
				Config: omitted,
				Check: func(*terraform.State) error {
					if got := string(st.ResponseSchema); got != "null" {
						return fmt.Errorf("update did not clear responseSchema on the server, got %q", got)
					}
					return nil
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_automation.test", tfjsonpath.New("response_schema"), knownvalue.Null()),
				},
			},
			{
				Config: omitted,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// A blank scheduler_frequency is rejected at plan time. ValidateConfig only
// checks for a null (missing) frequency on scheduler automations, so an empty
// string would otherwise slip through to an apply-time API error.
func TestUnit_AutomationResource_SchedulerFrequencyRejectsBlank(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Frequency validation check"
		event_id       = "scheduler"
		action_id      = "move_multiple_cards"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		scheduler_frequency = ""

		scheduler_cron = {
			minute       = "30"
			hour         = "9"
			day_of_month = "*"
			month        = "*"
			day_of_week  = "*"
		}
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`(?i)non-empty, non-whitespace`),
			},
		},
	})
}

// The Read mapper sets name/event_id/action_id unconditionally, so a server-side
// clear is detected as drift rather than masked. Reinstating an `if a.Name != ""`
// guard would make this test fail (the empty read would not reach state).
func TestUnit_AutomationResource_ReadDetectsDriftToEmpty(t *testing.T) {
	st := &automationState{}
	srv := newAutomationServer(st)
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_automation" "test" {
		name           = "Original name"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				// The name is cleared outside Terraform; the refresh maps the
				// empty value into state, so a plan against the unchanged config
				// proposes restoring it. ExpectNonEmptyPlan (the plan check, which
				// asserts rather than tolerates) fails if drift is masked.
				PreConfig: func() { st.Name = "" },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectNonEmptyPlan()},
				},
			},
		},
	})
}
