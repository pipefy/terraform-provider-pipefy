// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// recordingLimiter counts the waits and can fail on demand.
type recordingLimiter struct {
	waits int
	err   error
}

func (l *recordingLimiter) Wait(context.Context) error {
	l.waits++
	return l.err
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestLimiterTransportWaitsThenDelegates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	lim := &recordingLimiter{}
	c := &http.Client{Transport: &limiterTransport{base: http.DefaultTransport, limiter: lim}}

	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if lim.waits != 1 {
		t.Errorf("waits = %d, want 1", lim.waits)
	}
}

func TestLimiterTransportStopsOnLimiterError(t *testing.T) {
	wantErr := errors.New("limiter refused")
	var called bool
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	})

	tr := &limiterTransport{base: base, limiter: &recordingLimiter{err: wantErr}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	if _, err := tr.RoundTrip(req); !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if called {
		t.Error("base transport was called after the limiter refused")
	}
}
