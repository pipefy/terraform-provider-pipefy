// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
)

// newTestClient points an SDK client at a fake GraphQL server. The real
// transport is used, so tests cover request encoding and response decoding.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New(&client.ApiClient{
		HTTP:     server.Client(),
		Endpoint: server.URL,
		Token:    "testtoken",
		Version:  "test",
	})
}

// respondJSON writes a GraphQL response body.
func respondJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

// capturedRequest is one decoded GraphQL request, for tests that assert on what
// went out rather than on what came back.
type capturedRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// capture decodes a request inside a handler. It reports through Errorf rather
// than Fatalf because handlers run on the server's goroutine, where Fatalf would
// stop that goroutine instead of failing the test.
func capture(t *testing.T, r *http.Request) capturedRequest {
	t.Helper()
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read request body: %v", err)
		return capturedRequest{}
	}
	var got capturedRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Errorf("decode request body %q: %v", body, err)
		return capturedRequest{}
	}
	return got
}
