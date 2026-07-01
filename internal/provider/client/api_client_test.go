// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

type echoData struct {
	Hello string `json:"hello"`
}

func TestApiClient_DoGraphQL_HTTPNil(t *testing.T) {
	c := &ApiClient{}
	err := c.DoGraphQL(t.Context(), "query {}", nil, nil)
	if err == nil || err.Error() != "api client http is nil" {
		t.Fatalf("expected http nil error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_EndpointEmpty(t *testing.T) {
	c := &ApiClient{HTTP: http.DefaultClient}
	err := c.DoGraphQL(t.Context(), "query {}", nil, nil)
	if err == nil || err.Error() != "api endpoint is empty" {
		t.Fatalf("expected endpoint empty error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_Non2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("oops"))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	err := c.DoGraphQL(t.Context(), "query {}", nil, nil)
	if err == nil {
		t.Fatalf("expected non-2xx error, got nil")
	}
}

func TestApiClient_DoGraphQL_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	err := c.DoGraphQL(t.Context(), "query {}", nil, nil)
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
}

func TestApiClient_DoGraphQL_GraphQLError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	err := c.DoGraphQL(t.Context(), "query {}", nil, nil)
	if err == nil || err.Error() != "graphql error: boom" {
		t.Fatalf("expected graphql error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_OutNil_Succeeds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	if err := c.DoGraphQL(t.Context(), "query {}", nil, nil); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_UserAgentHeader(t *testing.T) {
	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL, Version: "1.2.3"}
	if err := c.DoGraphQL(t.Context(), "query {}", nil, nil); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if want := "terraform-provider-pipefy/1.2.3"; gotUA != want {
		t.Fatalf("User-Agent = %q, want %q", gotUA, want)
	}
}

func TestApiClient_DoGraphQL_TraceparentHeader(t *testing.T) {
	var got string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("traceparent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer ts.Close()

	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL, TraceID: traceID}
	if err := c.DoGraphQL(t.Context(), "query {}", nil, nil); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	re := regexp.MustCompile("^00-" + regexp.QuoteMeta(traceID) + "-[0-9a-f]{16}-01$")
	if !re.MatchString(got) {
		t.Fatalf("traceparent = %q, want match %s", got, re)
	}
}

func TestApiClient_DoGraphQL_TraceparentTraceConstantSpanVaries(t *testing.T) {
	var got []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("traceparent"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer ts.Close()

	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL, TraceID: traceID}
	for range 2 {
		if err := c.DoGraphQL(t.Context(), "query {}", nil, nil); err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(got))
	}
	parts0, parts1 := strings.Split(got[0], "-"), strings.Split(got[1], "-")
	if len(parts0) != 4 || len(parts1) != 4 {
		t.Fatalf("malformed traceparent(s): %q, %q", got[0], got[1])
	}
	if parts0[1] != traceID || parts1[1] != traceID {
		t.Fatalf("trace-id not constant across requests: %q vs %q", got[0], got[1])
	}
	if parts0[2] == parts1[2] {
		t.Fatalf("span-id should differ across requests, both %q", parts0[2])
	}
}

func TestApiClient_DoGraphQL_NoTraceparentWhenUnset(t *testing.T) {
	var present bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["Traceparent"]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	if err := c.DoGraphQL(t.Context(), "query {}", nil, nil); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if present {
		t.Fatalf("traceparent header should be absent when TraceID is unset")
	}
}

func TestNewTraceID(t *testing.T) {
	re := regexp.MustCompile("^[0-9a-f]{32}$")
	a, b := NewTraceID(), NewTraceID()
	if !re.MatchString(a) {
		t.Fatalf("NewTraceID() = %q, want 32 lowercase hex chars", a)
	}
	if a == b {
		t.Fatalf("NewTraceID() returned identical values %q", a)
	}
}

func TestApiClient_DoGraphQL_DataAndErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"hello":"world"},"errors":[{"message":"boom"},{"message":"bang"}]}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	var out echoData
	err := c.DoGraphQL(t.Context(), "query {}", nil, &out)
	if err == nil || err.Error() != "graphql error: boom; bang" {
		t.Fatalf("expected joined graphql error, got: %v", err)
	}
	if out.Hello != "world" {
		t.Fatalf("expected data unmarshaled despite top-level errors, got: %+v", out)
	}
}

func TestApiClient_DoGraphQL_DataDecodeMismatchYieldsToErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		// hello is typed string in echoData; a number can't decode into it.
		_, _ = w.Write([]byte(`{"data":{"hello":42},"errors":[{"message":"boom"}]}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	var out echoData
	err := c.DoGraphQL(t.Context(), "query {}", nil, &out)
	if err == nil || err.Error() != "graphql error: boom" {
		t.Fatalf("expected top-level error to win over decode mismatch, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_DataNullWithErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"unauthorized"}]}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	var out echoData
	err := c.DoGraphQL(t.Context(), "query {}", nil, &out)
	if err == nil || err.Error() != "graphql error: unauthorized" {
		t.Fatalf("expected graphql error, got: %v", err)
	}
	if out.Hello != "" {
		t.Fatalf("expected out untouched on data:null, got: %+v", out)
	}
}

func TestApiClient_DoGraphQL_MissingData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	var out map[string]any
	err := c.DoGraphQL(t.Context(), "query {}", nil, &out)
	if err == nil || err.Error() != "graphql response missing data" {
		t.Fatalf("expected missing data error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_SuccessDecode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testtoken" {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errors":[{"message":"missing auth"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"data": echoData{Hello: "world"}}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL, Token: "testtoken"}
	var out echoData
	if err := c.DoGraphQL(t.Context(), "query {}", nil, &out); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if out.Hello != "world" {
		t.Fatalf("unexpected decode result: %+v", out)
	}
}
