// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRetryOn429(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		transport error
		wantRetry bool
	}{
		{name: "429 is retried", status: http.StatusTooManyRequests, wantRetry: true},
		{name: "200 is not", status: http.StatusOK},
		{name: "401 is not", status: http.StatusUnauthorized},
		{name: "422 is not", status: http.StatusUnprocessableEntity},
		{name: "500 is not", status: http.StatusInternalServerError},
		{name: "503 is not", status: http.StatusServiceUnavailable},
		{name: "transport error is not", transport: errors.New("connection reset")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *http.Response
			if tt.transport == nil {
				resp = &http.Response{StatusCode: tt.status}
			}

			retry, err := retryOn429(t.Context(), resp, tt.transport)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if retry != tt.wantRetry {
				t.Errorf("retry = %v, want %v", retry, tt.wantRetry)
			}
		})
	}
}

func TestRetryOn429CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	retry, err := retryOn429(ctx, &http.Response{StatusCode: http.StatusTooManyRequests}, nil)
	if retry {
		t.Error("retry = true, want false for a cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestBackoffLogsAndPreservesRetryAfter(t *testing.T) {
	var gotMsg string
	var gotFields map[string]any
	r := retrier{log: func(_ context.Context, msg string, fields map[string]any) {
		gotMsg = msg
		gotFields = fields
	}}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.invalid/graphql", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"7"}},
		Request:    req,
	}

	wait := r.backoff(retryWaitMin, retryWaitMax, 0, resp)

	if wait != 7*time.Second {
		t.Errorf("wait = %v, want 7s: Retry-After must reach the caller unchanged", wait)
	}
	if gotMsg == "" {
		t.Fatal("nothing was logged")
	}
	if gotFields["attempt"] != 1 {
		t.Errorf("attempt = %v, want 1", gotFields["attempt"])
	}
	if gotFields["wait"] != "7s" {
		t.Errorf("wait field = %v, want \"7s\"", gotFields["wait"])
	}
	if gotFields["retry_after"] != "7" {
		t.Errorf("retry_after = %v, want \"7\"", gotFields["retry_after"])
	}
}

func TestBackoffWithoutRetryAfterIsExponential(t *testing.T) {
	r := retrier{}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.invalid/graphql", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, Request: req}

	if got := r.backoff(retryWaitMin, retryWaitMax, 0, resp); got != retryWaitMin {
		t.Errorf("first wait = %v, want %v", got, retryWaitMin)
	}
	if got := r.backoff(retryWaitMin, retryWaitMax, 1, resp); got != 2*time.Second {
		t.Errorf("second wait = %v, want 2s", got)
	}
}

func TestBackoffToleratesNilLogAndNilResponse(t *testing.T) {
	r := retrier{}
	if got := r.backoff(retryWaitMin, retryWaitMax, 0, nil); got != retryWaitMin {
		t.Errorf("wait = %v, want %v", got, retryWaitMin)
	}
}
