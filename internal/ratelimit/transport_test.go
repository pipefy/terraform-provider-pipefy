// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ratelimit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/time/rate"
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

// fastSettings keeps the waits short so the retry tests do not sleep.
func fastSettings() settings {
	return settings{
		maxRetries:     maxRetries,
		retryWaitMin:   time.Millisecond,
		retryWaitMax:   2 * time.Millisecond,
		attemptTimeout: 5 * time.Second,
	}
}

func TestTransportRetries429ThenSucceeds(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	lim := &recordingLimiter{}
	c := &http.Client{Transport: newTransport(fastSettings(), lim, http.DefaultTransport, nil)}

	resp, err := c.Post(srv.URL, "application/json", strings.NewReader(`{"query":"x"}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Errorf("got %d %q, want 200 \"ok\"", resp.StatusCode, string(body))
	}
	if len(bodies) != 2 {
		t.Fatalf("attempts = %d, want 2", len(bodies))
	}
	for i, b := range bodies {
		if b != `{"query":"x"}` {
			t.Errorf("attempt %d body = %q, want the request replayed unchanged", i+1, b)
		}
	}
	if lim.waits != 2 {
		t.Errorf("limiter waits = %d, want 2: retries must be paced too", lim.waits)
	}
}

func TestTransportBoundsAttemptsAndReturnsTheFinal429(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("throttled"))
	}))
	defer srv.Close()

	c := &http.Client{Transport: newTransport(fastSettings(), &recordingLimiter{}, http.DefaultTransport, nil)}

	resp, err := c.Post(srv.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("err = %v, want nil so the caller can read the status", err)
	}
	defer resp.Body.Close()

	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "throttled" {
		t.Errorf("body = %q, want it readable by the caller", string(body))
	}
}

func TestTransportDoesNotRetry500(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := &http.Client{Transport: newTransport(fastSettings(), &recordingLimiter{}, http.DefaultTransport, nil)}

	resp, err := c.Post(srv.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer resp.Body.Close()

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1: a non-idempotent mutation must not be replayed", attempts)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "boom" {
		t.Errorf("body = %q, want it preserved", string(body))
	}
}

func TestTransportStopsWhenTheContextIsCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	s := fastSettings()
	s.retryWaitMin = 10 * time.Second
	s.retryWaitMax = 10 * time.Second

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	var backoffCalls int
	log := func(context.Context, string, map[string]any) { backoffCalls++ }
	c := &http.Client{Transport: newTransport(s, &recordingLimiter{}, http.DefaultTransport, log)}

	start := time.Now()
	if _, err := c.Do(req); err == nil {
		t.Fatal("err = nil, want a context error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("returned after %v, want it to give up as soon as the context ended", elapsed)
	}
	// retryablehttp calls Backoff only after it has decided to retry and cleared
	// its remaining-attempts check, so one call here proves the run reached the
	// 10s wait and was interrupted there, rather than attempt 1 itself
	// overrunning the context before the wait was ever entered.
	if backoffCalls != 1 {
		t.Errorf("backoff calls = %d, want 1: the wait must be entered and then interrupted", backoffCalls)
	}
}

func TestTransportLogsOncePerRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	var logged []map[string]any
	log := func(_ context.Context, _ string, fields map[string]any) {
		logged = append(logged, fields)
	}

	c := &http.Client{Transport: newTransport(fastSettings(), &recordingLimiter{}, http.DefaultTransport, log)}

	resp, err := c.Post(srv.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer resp.Body.Close()

	if len(logged) != 2 {
		t.Fatalf("log entries = %d, want 2: one per retry, none for the first attempt", len(logged))
	}
	if logged[0]["attempt"] != 1 || logged[1]["attempt"] != 2 {
		t.Errorf("attempts = %v, %v, want 1, 2", logged[0]["attempt"], logged[1]["attempt"])
	}
	if _, ok := logged[0]["wait"]; !ok {
		t.Error("the wait duration is missing from the log fields")
	}
}

// TestNewTransportUsesProductionSettings walks the stack NewTransport builds,
// since every other test in this file goes through newTransport with
// fastSettings, a recordingLimiter, and http.DefaultTransport, none of which
// exercise the production wiring.
func TestNewTransportUsesProductionSettings(t *testing.T) {
	rt, ok := NewTransport(nil).(*retryablehttp.RoundTripper)
	if !ok {
		t.Fatal("NewTransport did not return a *retryablehttp.RoundTripper")
	}
	if rt.Client == nil {
		t.Fatal("rt.Client is nil")
	}

	c := rt.Client
	if c.RetryMax != 2 {
		t.Errorf("RetryMax = %d, want 2", c.RetryMax)
	}
	if c.RetryWaitMin != time.Second {
		t.Errorf("RetryWaitMin = %v, want 1s", c.RetryWaitMin)
	}
	if c.RetryWaitMax != 3*time.Second {
		t.Errorf("RetryWaitMax = %v, want 3s", c.RetryWaitMax)
	}
	if c.HTTPClient == nil {
		t.Fatal("HTTPClient is nil")
	}
	if c.HTTPClient.Timeout != 30*time.Second {
		t.Errorf("HTTPClient.Timeout = %v, want 30s", c.HTTPClient.Timeout)
	}

	lt, ok := c.HTTPClient.Transport.(*limiterTransport)
	if !ok {
		t.Fatal("HTTPClient.Transport is not a *limiterTransport, so the limiter would sit outside the retry loop")
	}

	lim, ok := lt.limiter.(*rate.Limiter)
	if !ok {
		t.Fatal("limiter is not a *rate.Limiter")
	}
	if lim.Limit() != rate.Limit(15) {
		t.Errorf("rate = %v, want 15", lim.Limit())
	}
	if lim.Burst() != 15 {
		t.Errorf("burst = %d, want 15", lim.Burst())
	}

	if _, ok := lt.base.(*http.Transport); !ok {
		t.Fatal("base is not a *http.Transport")
	}
	if lt.base == http.DefaultTransport {
		t.Error("base is the shared http.DefaultTransport, want a private pooled transport")
	}
}
