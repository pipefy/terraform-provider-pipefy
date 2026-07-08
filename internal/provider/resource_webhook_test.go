// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"encoding/json"
	"fmt"
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
	ID          string
	Name        string
	URL         string
	Actions     []string
	HeadersType string // Go type of the headers input on the last create/update
	DeletedCt   int
}

func webhookInputStrings(input map[string]any) (name, url string, actions []string) {
	if v, ok := input["name"].(string); ok {
		name = v
	}
	if v, ok := input["url"].(string); ok {
		url = v
	}
	if raw, ok := input["actions"].([]any); ok {
		for _, a := range raw {
			if s, ok := a.(string); ok {
				actions = append(actions, s)
			}
		}
	}
	return name, url, actions
}

func writeWebhook(w http.ResponseWriter, root string, st *webhookState) {
	payload, _ := json.Marshal(map[string]any{
		"id":      st.ID,
		"name":    st.Name,
		"url":     st.URL,
		"actions": st.Actions,
	})
	_, _ = io.WriteString(w, `{"data":{"`+root+`":{"webhook":`+string(payload)+`}}}`)
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

		input, _ := gr.Variables["input"].(map[string]any)
		q := gr.Query
		switch {
		case strings.Contains(q, "createWebhook"):
			st.ID = "webhook_123"
			st.Name, st.URL, st.Actions = webhookInputStrings(input)
			if h, ok := input["headers"]; ok {
				st.HeadersType = fmt.Sprintf("%T", h)
			}
			writeWebhook(w, "createWebhook", st)
		case strings.Contains(q, "updateWebhook"):
			name, url, actions := webhookInputStrings(input)
			if name != "" {
				st.Name = name
			}
			if url != "" {
				st.URL = url
			}
			if actions != nil {
				st.Actions = actions
			}
			writeWebhook(w, "updateWebhook", st)
		case strings.Contains(q, "deleteWebhook"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteWebhook":{"success":true}}}`)
		case strings.Contains(q, "webhooks"):
			if st.ID == "" {
				_, _ = io.WriteString(w, `{"data":{"pipe":{"webhooks":[]}}}`)
				return
			}
			actions, _ := json.Marshal(st.Actions)
			_, _ = io.WriteString(w, `{"data":{"pipe":{"webhooks":[{"id":"`+st.ID+`","name":"`+st.Name+`","url":"`+st.URL+`","actions":`+string(actions)+`}]}}}`)
		case strings.Contains(q, "createPipe"):
			_, _ = io.WriteString(w, `{"data":{"createPipe":{"pipe":{"id":"pipe_1","name":"My Pipe"}}}}`)
		case strings.Contains(q, "updatePipe"):
			_, _ = io.WriteString(w, `{"data":{"updatePipe":{"pipe":{"id":"pipe_1"}}}}`)
		case strings.Contains(q, "deletePipe"):
			_, _ = io.WriteString(w, `{"data":{"deletePipe":{"success":true}}}`)
		case strings.Contains(q, "phases"):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"pipe_1","startFormPhaseId":"sfp_1","phases":[]}}}`)
		case strings.Contains(q, "pipe("):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"pipe_1","name":"My Pipe","startFormPhaseId":"sfp_1"}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()

	providerBlock := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}
	`

	config := providerBlock + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Card events"
		url     = "https://example.com/hook"
		actions = ["card.create", "card.move"]
		headers = jsonencode({ Authorization = "Bearer secret" })
	}
	`

	configUpdated := providerBlock + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Card events renamed"
		url     = "https://example.com/hook2"
		actions = ["card.done"]
		headers = jsonencode({ Authorization = "Bearer secret" })
	}
	`

	configDestroy := providerBlock + `
	resource "pipefy_webhook" "test" {
		count   = 0
		pipe_id = pipefy_pipe.p.id
		name    = "Card events renamed"
		url     = "https://example.com/hook2"
		actions = ["card.done"]
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
						knownvalue.StringExact("webhook_123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Card events"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("actions"),
						knownvalue.ListExact([]knownvalue.Check{
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
						knownvalue.StringExact("Card events renamed"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("url"),
						knownvalue.StringExact("https://example.com/hook2"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_webhook.test",
						tfjsonpath.New("actions"),
						knownvalue.ListExact([]knownvalue.Check{
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
	// The API's headers field is the Json scalar and must be sent as a JSON
	// string, not an object. Sending an object is rejected by the backend.
	if st.HeadersType != "string" {
		t.Fatalf("expected headers input to be a string, got %q", st.HeadersType)
	}
}
