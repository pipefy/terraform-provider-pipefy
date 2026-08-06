// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestWebhooksGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"webhooks":[
			{"id":"other","name":"X","url":"https://x.test","actions":["card.create"],"filters":{}},
			{"id":"wh1","name":"Ship","url":"https://ship.test","actions":["card.done","card.move"],"filters":{"phase_id":"900"}}
		]}}}`)
	})

	webhook, err := c.Webhooks.Get(t.Context(), "301", "wh1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if webhook.Name != "Ship" || webhook.URL != "https://ship.test" {
		t.Errorf("webhook = %+v", webhook)
	}
	if strings.Join(webhook.Actions, ",") != "card.done,card.move" {
		t.Errorf("Actions = %v", webhook.Actions)
	}
}

func TestWebhooksGetMissingWebhookIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"webhooks":[{"id":"other"}]}}}`)
	})

	if _, err := c.Webhooks.Get(t.Context(), "301", "wh1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWebhooksGetMissingPipeIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":null}}`)
	})

	if _, err := c.Webhooks.Get(t.Context(), "301", "wh1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Filters stay raw bytes. Decoding into any would push a 19-digit id through
// float64 and lose precision.
func TestWebhooksFiltersStayRaw(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"pipe":{"webhooks":[
			{"id":"wh1","filters":{"card_id":1234567890123456789}}
		]}}}`)
	})

	webhook, err := c.Webhooks.Get(t.Context(), "301", "wh1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(webhook.Filters), "1234567890123456789") {
		t.Errorf("Filters = %s, want the 19-digit id byte-for-byte", webhook.Filters)
	}
}

// headers is never selected, so it can never leak back into state.
func TestWebhookSelectionOmitsHeaders(t *testing.T) {
	for name, document := range map[string]string{
		"create": createWebhookMutation,
		"read":   getPipeWebhooksQuery,
		"update": updateWebhookMutation,
	} {
		if strings.Contains(document, "headers") {
			t.Errorf("%s selects headers: %s", name, document)
		}
	}
}

func TestWebhooksCreatePassesInputThrough(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"createWebhook":{"webhook":{"id":"wh1","name":"Ship"}}}}`)
	})

	input := map[string]any{
		"pipe_id": "301",
		"url":     "https://ship.test",
		"name":    "Ship",
		"actions": []string{"card.done"},
		"headers": `{"X-Token":"secret"}`,
		"filters": map[string]any{"phase_id": "900"},
	}
	webhook, err := c.Webhooks.Create(t.Context(), input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if webhook.ID != "wh1" {
		t.Errorf("webhook = %+v", webhook)
	}
	sent, ok := got.Variables["input"].(map[string]any)
	if !ok {
		t.Fatalf("variables = %+v", got.Variables)
	}
	// headers goes out as a JSON string, filters as an object. The asymmetry is
	// the API's and has to survive.
	if _, isString := sent["headers"].(string); !isString {
		t.Errorf("headers = %#v, want a JSON string", sent["headers"])
	}
	if _, isObject := sent["filters"].(map[string]any); !isObject {
		t.Errorf("filters = %#v, want an object", sent["filters"])
	}
}

func TestWebhooksUpdate(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updateWebhook":{"webhook":{"id":"wh1"}}}}`)
	})

	if err := c.Webhooks.Update(t.Context(), map[string]any{"id": "wh1", "name": "Ship"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	sent, ok := got.Variables["input"].(map[string]any)
	if !ok || sent["id"] != "wh1" {
		t.Errorf("variables = %+v", got.Variables)
	}
}

func TestWebhooksDelete(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"deleteWebhook":{"success":true}}}`)
	})

	if err := c.Webhooks.Delete(t.Context(), "wh1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
