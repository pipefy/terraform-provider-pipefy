package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type echoData struct {
	Hello string `json:"hello"`
}

func TestApiClient_DoGraphQL_HTTPNil(t *testing.T) {
	c := &ApiClient{}
	err := c.DoGraphQL(context.Background(), "query {}", nil, nil)
	if err == nil || err.Error() != "api client http is nil" {
		t.Fatalf("expected http nil error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_EndpointEmpty(t *testing.T) {
	c := &ApiClient{HTTP: http.DefaultClient}
	err := c.DoGraphQL(context.Background(), "query {}", nil, nil)
	if err == nil || err.Error() != "api endpoint is empty" {
		t.Fatalf("expected endpoint empty error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_Non2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("oops"))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	err := c.DoGraphQL(context.Background(), "query {}", nil, nil)
	if err == nil {
		t.Fatalf("expected non-2xx error, got nil")
	}
}

func TestApiClient_DoGraphQL_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	err := c.DoGraphQL(context.Background(), "query {}", nil, nil)
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
}

func TestApiClient_DoGraphQL_GraphQLError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	err := c.DoGraphQL(context.Background(), "query {}", nil, nil)
	if err == nil || err.Error() != "graphql error: boom" {
		t.Fatalf("expected graphql error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_OutNil_Succeeds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	if err := c.DoGraphQL(context.Background(), "query {}", nil, nil); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_MissingData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL}
	var out map[string]any
	err := c.DoGraphQL(context.Background(), "query {}", nil, &out)
	if err == nil || err.Error() != "graphql response missing data" {
		t.Fatalf("expected missing data error, got: %v", err)
	}
}

func TestApiClient_DoGraphQL_SuccessDecode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testtoken" {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"errors":[{"message":"missing auth"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"data": echoData{Hello: "world"}}
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
	defer ts.Close()

	c := &ApiClient{HTTP: ts.Client(), Endpoint: ts.URL, Token: "testtoken"}
	var out echoData
	if err := c.DoGraphQL(context.Background(), "query {}", nil, &out); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if out.Hello != "world" {
		t.Fatalf("unexpected decode result: %+v", out)
	}
}
