// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type ApiClient struct {
	HTTP     *http.Client
	Endpoint string
	Token    string
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

func (c *ApiClient) DoGraphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	if c.HTTP == nil {
		return fmt.Errorf("api client http is nil")
	}
	if c.Endpoint == "" {
		return fmt.Errorf("api endpoint is empty")
	}
	bodyBytes, err := json.Marshal(graphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Only set Authorization header if we have a static token
	// For OAuth2 client credentials, the http.Client handles this automatically
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		fmt.Printf("DEBUG: Using static token for authorization\n")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Check for non-2xx status codes first
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("graphql http status %d (content-type=%s): %s", resp.StatusCode, resp.Header.Get("Content-Type"), string(respBody))
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		responsePreview := string(respBody)
		if len(responsePreview) > 200 {
			responsePreview = responsePreview[:200] + "..."
		}
		return fmt.Errorf("failed to parse JSON response (status %d): %s. Response preview: %s", resp.StatusCode, err.Error(), responsePreview)
	}

	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}
	if out == nil {
		return nil
	}
	if len(gqlResp.Data) == 0 {
		return fmt.Errorf("graphql response missing data")
	}
	return json.Unmarshal(gqlResp.Data, out)
}
