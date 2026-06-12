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

type webhookState struct {
	ID        string
	Name      string
	URL       string
	Actions   []string
	DeletedCt int
}

func TestUnit_WebhookResource_CRUD(t *testing.T) {
	st := &webhookState{}
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
		case strings.Contains(q, "createWebhook"):
			st.ID = "wh_123"
			if v, ok := gr.Variables["name"].(string); ok {
				st.Name = v
			}
			if v, ok := gr.Variables["url"].(string); ok {
				st.URL = v
			}
			if v, ok := gr.Variables["actions"].([]any); ok {
				st.Actions = toStringSlice(v)
			}
			actionsJSON, _ := json.Marshal(st.Actions)
			_, _ = io.WriteString(w, `{"data":{"createWebhook":{"webhook":{"id":"`+st.ID+`","name":"`+st.Name+`","url":"`+st.URL+`","actions":`+string(actionsJSON)+`}}}}`)
		case strings.Contains(q, "updateWebhook"):
			if v, ok := gr.Variables["name"].(string); ok {
				st.Name = v
			}
			if v, ok := gr.Variables["url"].(string); ok {
				st.URL = v
			}
			if v, ok := gr.Variables["actions"].([]any); ok {
				st.Actions = toStringSlice(v)
			}
			actionsJSON, _ := json.Marshal(st.Actions)
			_, _ = io.WriteString(w, `{"data":{"updateWebhook":{"webhook":{"id":"`+st.ID+`","name":"`+st.Name+`","url":"`+st.URL+`","actions":`+string(actionsJSON)+`}}}}`)
		case strings.Contains(q, "deleteWebhook"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteWebhook":{"success":true}}}`)
		case strings.Contains(q, "webhooks"):
			if st.ID == "" {
				_, _ = io.WriteString(w, `{"data":{"pipe":{"webhooks":[]}}}`)
				return
			}
			actionsJSON, _ := json.Marshal(st.Actions)
			_, _ = io.WriteString(w, `{"data":{"pipe":{"webhooks":[{"id":"`+st.ID+`","name":"`+st.Name+`","url":"`+st.URL+`","actions":`+string(actionsJSON)+`}]}}}`)
		case strings.Contains(q, "createPipe"):
			_, _ = io.WriteString(w, `{"data":{"createPipe":{"pipe":{"id":"pipe_1","name":"My Pipe"}}}}`)
		case strings.Contains(q, "updatePipe"):
			_, _ = io.WriteString(w, `{"data":{"updatePipe":{"pipe":{"id":"pipe_1"}}}}`)
		case strings.Contains(q, "deletePipe"):
			_, _ = io.WriteString(w, `{"data":{"deletePipe":{"success":true}}}`)
		case strings.Contains(q, "deletePhase"):
			_, _ = io.WriteString(w, `{"data":{"deletePhase":{"clientMutationId":"","success":true}}}`)
		case strings.Contains(q, "phases"):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"pipe_1","startFormPhaseId":"sfp_1","phases":[]}}}`)
		case strings.Contains(q, "pipe("):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"pipe_1","name":"My Pipe","startFormPhaseId":"sfp_1"}}}`)
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

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Test Webhook"
		url     = "https://example.com/hook"
		actions = ["card.create", "card.move"]
	}
	`

	configUpdated := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Updated Webhook"
		url     = "https://example.com/hook2"
		actions = ["card.create", "card.done"]
	}
	`

	configDestroy := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_webhook" "test" {
		count   = 0
		pipe_id = pipefy_pipe.p.id
		name    = "Updated Webhook"
		url     = "https://example.com/hook2"
		actions = ["card.create", "card.done"]
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
						"pipefy_webhook.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("wh_123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Test Webhook"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("url"),
						knownvalue.StringExact("https://example.com/hook"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("actions"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("card.create"),
							knownvalue.StringExact("card.move"),
						}),
					),
				},
			},
			{
				Config: configUpdated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Updated Webhook"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("url"),
						knownvalue.StringExact("https://example.com/hook2"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("actions"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("card.create"),
							knownvalue.StringExact("card.done"),
						}),
					),
				},
			},
			{
				Config: configDestroy,
			},
		},
	})

	if st.DeletedCt == 0 {
		t.Fatalf("expected deleteWebhook mutation to be called")
	}
}

func toStringSlice(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
