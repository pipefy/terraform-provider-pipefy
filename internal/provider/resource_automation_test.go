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

type automationState struct {
	ID                  string
	CreatedEventParams  any
	CreatedActionParams any
	UpdatedEventParams  any
	UpdatedActionParams any
	DeletedCt           int
}

func TestUnit_AutomationResource_CRUD(t *testing.T) {
	st := &automationState{}
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
		case strings.Contains(q, "createAutomation"):
			// input is nested under variables["input"]
			if in, ok := gr.Variables["input"].(map[string]any); ok {
				if v, ok2 := in["event_params"]; ok2 {
					st.CreatedEventParams = v
				}
				if v, ok2 := in["action_params"]; ok2 {
					st.CreatedActionParams = v
				}
			}
			st.ID = "auto_1"
			_, _ = io.WriteString(w, `{"data":{"createAutomation":{"automation":{"id":"`+st.ID+`","name":"When Summary changes, generate AI output","action_id":"generate_with_ai","event_id":"field_updated","active":true}}}}`)
		case strings.Contains(q, "updateAutomation"):
			if in, ok := gr.Variables["input"].(map[string]any); ok {
				if v, ok2 := in["event_params"]; ok2 {
					st.UpdatedEventParams = v
				}
				if v, ok2 := in["action_params"]; ok2 {
					st.UpdatedActionParams = v
				}
			}
			_, _ = io.WriteString(w, `{"data":{"updateAutomation":{"automation":{"id":"`+st.ID+`"}}}}`)
		case strings.Contains(q, "deleteAutomation"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteAutomation":{"success":true}}}`)
		case strings.Contains(q, "automation("):
			// Read path
			_, _ = io.WriteString(w, `{"data":{"automation":{"id":"`+st.ID+`","name":"When Summary changes, generate AI output","action_id":"generate_with_ai","event_id":"field_updated","active":true,"event_repo":{"id":"repo"},"action_repo_v2":{"id":"repo"}}}}`)
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
		name           = "When Summary changes, generate AI output"
		event_id       = "field_updated"
		action_id      = "generate_with_ai"
		event_repo_id  = "306729113"
		action_repo_id = "306729113"
		active         = true

		event_params = jsonencode({
			triggerFieldIds = [420173505]
		})

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
