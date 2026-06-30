// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ApiClient struct {
	HTTP     *http.Client
	Endpoint string
	Token    string
	Version  string
	TraceID  string
}

// NewTraceID returns a W3C Trace Context trace-id: 16 random bytes as 32
// lowercase hex characters. The provider is the root of the trace (Terraform
// passes no upstream context), so one trace-id is minted per run and reused on
// every request, grouping all of a run's API calls under a single trace.
func NewTraceID() string { return randHex(16) }

// newSpanID returns a trace-context span-id: 8 random bytes as 16 hex
// characters, fresh per request so each call is its own span under the trace.
func newSpanID() string { return randHex(8) }

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// traceparent assembles a version-00 header with the sampled flag (01) set, so
// the backend records every provider run.
func traceparent(traceID, spanID string) string {
	return "00-" + traceID + "-" + spanID + "-01"
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors"`
}

func (c *ApiClient) execGraphQL(ctx context.Context, query string, variables map[string]any) (*graphQLResponse, error) {
	if c.HTTP == nil {
		return nil, fmt.Errorf("api client http is nil")
	}
	if c.Endpoint == "" {
		return nil, fmt.Errorf("api endpoint is empty")
	}
	bodyBytes, err := json.Marshal(graphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "terraform-provider-pipefy/"+c.Version)
	if c.TraceID != "" {
		req.Header.Set("traceparent", traceparent(c.TraceID, newSpanID()))
	}

	// Only set Authorization header if we have a static token
	// For OAuth2 client credentials, the http.Client handles this automatically
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check for non-2xx status codes first
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("graphql http status %d (content-type=%s): %s", resp.StatusCode, resp.Header.Get("Content-Type"), string(respBody))
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		responsePreview := string(respBody)
		if len(responsePreview) > 200 {
			responsePreview = responsePreview[:200] + "..."
		}
		return nil, fmt.Errorf("failed to parse JSON response (status %d): %s. Response preview: %s", resp.StatusCode, err.Error(), responsePreview)
	}
	return &gqlResp, nil
}

// DoGraphQL runs the query and decodes data into out. It unmarshals data
// whenever the response carries it, even alongside top-level errors, so a
// caller can read a payload the API returns with the errors (such as a
// mutation's error_details). out is therefore populated even when this returns
// an error. The error joins all top-level GraphQL error messages.
func (c *ApiClient) DoGraphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	gqlResp, err := c.execGraphQL(ctx, query, variables)
	if err != nil {
		return err
	}
	var decodeErr error
	if out != nil && len(gqlResp.Data) > 0 {
		decodeErr = json.Unmarshal(gqlResp.Data, out)
	}
	// Top-level errors take precedence over a data-decode mismatch: a rejected
	// request can still return a payload whose shape differs from the query
	// selection, and the GraphQL message is the useful one.
	if len(gqlResp.Errors) > 0 {
		messages := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			messages[i] = e.Message
		}
		return fmt.Errorf("graphql error: %s", strings.Join(messages, "; "))
	}
	if decodeErr != nil {
		return decodeErr
	}
	if out != nil && len(gqlResp.Data) == 0 {
		return fmt.Errorf("graphql response missing data")
	}
	return nil
}
