// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
)

func TestResolvePipeUUID(t *testing.T) {
	var operation string
	api, closeServer := pipeUUIDTestClient(t, func(request gqlTestRequest) string {
		operation = request.Query
		if request.Variables["id"] != "42" {
			t.Fatalf("pipe id = %#v, want 42", request.Variables["id"])
		}
		return `{"data":{"pipe":{"uuid":"pipe-uuid"}}}`
	})
	defer closeServer()

	got, err := resolvePipeUUID(t.Context(), api, "42")
	if err != nil || got != "pipe-uuid" {
		t.Fatalf("resolvePipeUUID = (%q, %v), want (pipe-uuid, nil)", got, err)
	}
	if !strings.Contains(operation, "query GetPipeUuid_tf") {
		t.Fatalf("query operation missing GetPipeUuid_tf: %s", operation)
	}
}

func TestResolvePipeUUIDErrors(t *testing.T) {
	cases := map[string]struct {
		response string
		want     string
	}{
		"missing pipe": {`{"data":{"pipe":null}}`, `pipe "42"`},
		"empty uuid":   {`{"data":{"pipe":{"uuid":""}}}`, `pipe "42"`},
		"graphql":      {`{"errors":[{"message":"forbidden"}]}`, "forbidden"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			api, closeServer := pipeUUIDTestClient(t, func(gqlTestRequest) string { return tc.response })
			defer closeServer()
			_, err := resolvePipeUUID(t.Context(), api, "42")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

type gqlTestRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func pipeUUIDTestClient(t *testing.T, reply func(gqlTestRequest) string) (*client.ApiClient, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		var request gqlTestRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply(request))
	}))
	api := &client.ApiClient{HTTP: server.Client(), Endpoint: server.URL, Token: "test"}
	return api, server.Close
}
