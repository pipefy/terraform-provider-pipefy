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
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

type webhookState struct {
	ID      string
	Name    string
	URL     string
	Actions []string
	Filters string // stored filters as raw JSON; "" means none

	// Serialization of the headers/filters inputs on the last create and update,
	// so tests can lock the Json-string vs JSON-object contract and the
	// clear-on-remove behavior.
	HeadersType       string
	FiltersType       string
	UpdateHeadersType string
	UpdateFiltersType string
	UpdateHeadersNull bool
	UpdateFiltersNull bool

	DeletedCt int
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

func webhookJSON(st *webhookState) string {
	filters := st.Filters
	if filters == "" {
		filters = "{}"
	}
	b, _ := json.Marshal(map[string]any{
		"id":      st.ID,
		"name":    st.Name,
		"url":     st.URL,
		"actions": st.Actions,
		"filters": json.RawMessage(filters),
	})
	return string(b)
}

// newWebhookServer returns a mock Pipefy GraphQL endpoint that tracks a single
// webhook's state, stores its filters, and records how headers/filters were
// serialized so tests can assert the wire contract.
func newWebhookServer(st *webhookState) *httptest.Server {
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

		input, _ := gr.Variables["input"].(map[string]any)
		q := gr.Query
		switch {
		case strings.Contains(q, "createWebhook"):
			st.ID = "webhook_123"
			st.Name, st.URL, st.Actions = webhookInputStrings(input)
			if h, ok := input["headers"]; ok {
				st.HeadersType = fmt.Sprintf("%T", h)
			}
			if f, ok := input["filters"]; ok {
				st.FiltersType = fmt.Sprintf("%T", f)
				if f != nil {
					fb, _ := json.Marshal(f)
					st.Filters = string(fb)
				}
			}
			_, _ = io.WriteString(w, `{"data":{"createWebhook":{"webhook":`+webhookJSON(st)+`}}}`)
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
			if h, ok := input["headers"]; ok {
				st.UpdateHeadersType = fmt.Sprintf("%T", h)
				st.UpdateHeadersNull = h == nil
			}
			if f, ok := input["filters"]; ok {
				st.UpdateFiltersType = fmt.Sprintf("%T", f)
				st.UpdateFiltersNull = f == nil
				if f == nil {
					st.Filters = ""
				} else {
					fb, _ := json.Marshal(f)
					st.Filters = string(fb)
				}
			}
			_, _ = io.WriteString(w, `{"data":{"updateWebhook":{"webhook":`+webhookJSON(st)+`}}}`)
		case strings.Contains(q, "deleteWebhook"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteWebhook":{"success":true}}}`)
		case strings.Contains(q, "webhooks"):
			if st.ID == "" {
				_, _ = io.WriteString(w, `{"data":{"pipe":{"webhooks":[]}}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"pipe":{"webhooks":[`+webhookJSON(st)+`]}}}`)
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
}

func webhookProviderBlock(endpoint string) string {
	return `
	provider "pipefy" {
		endpoint = "` + endpoint + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}
	`
}

func TestUnit_WebhookResource_CRUD(t *testing.T) {
	st := &webhookState{}
	srv := newWebhookServer(st)
	defer srv.Close()
	provider := webhookProviderBlock(srv.URL)

	config := provider + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Card events"
		url     = "https://example.com/hook"
		actions = ["card.create", "card.move"]
		headers = jsonencode({ Authorization = "Bearer secret" })
	}
	`

	configUpdated := provider + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Card events renamed"
		url     = "https://example.com/hook2"
		actions = ["card.done"]
		headers = jsonencode({ Authorization = "Bearer rotated" })
	}
	`

	configDestroy := provider + `
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
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("id"), knownvalue.StringExact("webhook_123")),
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("name"), knownvalue.StringExact("Card events")),
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("actions"), knownvalue.ListExact([]knownvalue.Check{
						knownvalue.StringExact("card.create"),
						knownvalue.StringExact("card.move"),
					})),
				},
			},
			{
				Config: configUpdated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("name"), knownvalue.StringExact("Card events renamed")),
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("url"), knownvalue.StringExact("https://example.com/hook2")),
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("actions"), knownvalue.ListExact([]knownvalue.Check{
						knownvalue.StringExact("card.done"),
					})),
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
	// string, not an object, on both create and update (header rotation).
	if st.HeadersType != "string" {
		t.Fatalf("expected create headers input to be a string, got %q", st.HeadersType)
	}
	if st.UpdateHeadersType != "string" {
		t.Fatalf("expected update headers input to be a string, got %q", st.UpdateHeadersType)
	}
}

// TestUnit_WebhookResource_Filters covers the filters happy path: filters is
// sent as a JSON object (not a string) on create and update, is refreshed from
// the API without a spurious diff, and survives a value change.
func TestUnit_WebhookResource_Filters(t *testing.T) {
	st := &webhookState{}
	srv := newWebhookServer(st)
	defer srv.Close()
	provider := webhookProviderBlock(srv.URL)

	config := provider + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Moves"
		url     = "https://example.com/hook"
		actions = ["card.move"]
		filters = jsonencode({ from_phase_id = [268] })
	}
	`

	configChanged := provider + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Moves"
		url     = "https://example.com/hook"
		actions = ["card.move"]
		filters = jsonencode({ from_phase_id = [999] })
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
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("filters"), knownvalue.StringExact(`{"from_phase_id":[268]}`)),
				},
			},
			{
				// Same config again: refresh must not produce a spurious diff.
				Config: config,
			},
			{
				Config: configChanged,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("filters"), knownvalue.StringExact(`{"from_phase_id":[999]}`)),
				},
			},
		},
	})

	if st.FiltersType != "map[string]interface {}" {
		t.Fatalf("expected create filters input to be a JSON object, got %q", st.FiltersType)
	}
	if st.UpdateFiltersType != "map[string]interface {}" {
		t.Fatalf("expected update filters input to be a JSON object, got %q", st.UpdateFiltersType)
	}
}

// TestUnit_WebhookResource_FiltersDrift proves filters is reconciled: an
// out-of-band change to the remote filters is detected as a non-empty plan.
func TestUnit_WebhookResource_FiltersDrift(t *testing.T) {
	st := &webhookState{}
	srv := newWebhookServer(st)
	defer srv.Close()
	provider := webhookProviderBlock(srv.URL)

	config := provider + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Moves"
		url     = "https://example.com/hook"
		actions = ["card.move"]
		filters = jsonencode({ from_phase_id = [268] })
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
				// Simulate a change made outside Terraform, then expect the refresh
				// to surface it as a pending change.
				PreConfig: func() {
					st.Filters = `{"from_phase_id":[999]}`
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestUnit_WebhookResource_ClearOnRemove proves that removing headers and
// filters from config clears them on the API: the update payload carries an
// explicit null for both, and state ends up null.
func TestUnit_WebhookResource_ClearOnRemove(t *testing.T) {
	st := &webhookState{}
	srv := newWebhookServer(st)
	defer srv.Close()
	provider := webhookProviderBlock(srv.URL)

	withBoth := provider + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Moves"
		url     = "https://example.com/hook"
		actions = ["card.move"]
		headers = jsonencode({ Authorization = "Bearer secret" })
		filters = jsonencode({ from_phase_id = [268] })
	}
	`

	withoutBoth := provider + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Moves"
		url     = "https://example.com/hook"
		actions = ["card.move"]
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: withBoth,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("filters"), knownvalue.StringExact(`{"from_phase_id":[268]}`)),
				},
			},
			{
				Config: withoutBoth,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("headers"), knownvalue.Null()),
					statecheck.ExpectKnownValue("pipefy_webhook.test", tfjsonpath.New("filters"), knownvalue.Null()),
				},
			},
		},
	})

	if !st.UpdateHeadersNull {
		t.Fatalf("expected update to send headers: null when removed")
	}
	if !st.UpdateFiltersNull {
		t.Fatalf("expected update to send filters: null when removed")
	}
	if st.Filters != "" {
		t.Fatalf("expected remote filters to be cleared, got %q", st.Filters)
	}
}

// TestUnit_WebhookResource_Validations guarantees every client-side validation
// is wired to its attribute and rejects bad input at plan time: the URL
// validator and the JSON validation on headers and filters.
func TestUnit_WebhookResource_Validations(t *testing.T) {
	provider := `
	provider "pipefy" {
		token = "testtoken"
	}
	`
	base := func(body string) string {
		return provider + `
		resource "pipefy_webhook" "test" {
			pipe_id = "2"
			name    = "Hook"
` + body + `
		}
		`
	}

	cases := map[string]struct {
		body    string
		wantErr *regexp.Regexp
	}{
		"invalid url": {
			body: `
			url     = "not-a-url"
			actions = ["card.create"]`,
			wantErr: regexp.MustCompile(`Invalid URL`),
		},
		"invalid headers json": {
			body: `
			url     = "https://example.com/hook"
			actions = ["card.create"]
			headers = "{not valid json"`,
			wantErr: regexp.MustCompile(`Invalid JSON String Value`),
		},
		"invalid filters json": {
			body: `
			url     = "https://example.com/hook"
			actions = ["card.move"]
			filters = "{not valid json"`,
			wantErr: regexp.MustCompile(`Invalid JSON String Value`),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				TerraformVersionChecks: []tfversion.TerraformVersionCheck{
					tfversion.SkipBelow(tfversion.Version1_8_0),
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      base(tc.body),
						PlanOnly:    true,
						ExpectError: tc.wantErr,
					},
				},
			})
		})
	}
}

// TestUnit_WebhookResource_Import covers importing an existing webhook using the
// pipe_id/webhook_id syntax, and rejecting a malformed import ID.
func TestUnit_WebhookResource_Import(t *testing.T) {
	st := &webhookState{}
	srv := newWebhookServer(st)
	defer srv.Close()

	config := webhookProviderBlock(srv.URL) + `
	resource "pipefy_webhook" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Card events"
		url     = "https://example.com/hook"
		actions = ["card.create"]
		headers = jsonencode({ Authorization = "Bearer secret" })
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
				ResourceName:      "pipefy_webhook.test",
				ImportState:       true,
				ImportStateId:     "pipe_1/webhook_123",
				ImportStateVerify: true,
				// headers is not read back from the API, so it is absent from
				// imported state and excluded from the diff. filters is refreshed,
				// so it is not ignored.
				ImportStateVerifyIgnore: []string{"headers"},
			},
			{
				ResourceName:  "pipefy_webhook.test",
				ImportState:   true,
				ImportStateId: "webhook_123",
				ExpectError:   regexp.MustCompile(`expected pipe_id/webhook_id`),
			},
		},
	})
}
