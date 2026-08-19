// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ratelimit

import (
	"context"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// retryOn429 retries a request the API refused with HTTP 429, and nothing else.
// A 429 means the request was never processed, so replaying it cannot duplicate
// anything. Most mutations here are not idempotent and the client cannot detect
// a duplicate after the fact, so a retried 5xx or transport error risks creating
// a resource twice and losing track of it. A transport error yields a nil error
// as well, which tells retryablehttp to stop and keep the original.
func retryOn429(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return err == nil && resp != nil && resp.StatusCode == http.StatusTooManyRequests, nil
}

// The API allows 500 requests per 30 seconds and blocks further requests for
// about five minutes once that is exceeded, so the limiter runs below the
// budget: 15 per second with a burst of 15 is at most 465 in any 30 second
// window. The burst is at or above Terraform's default parallelism of 10 so the
// first wave of a run is not serialized.
// https://help.pipefy.com/en/articles/5580799-how-to-use-pipefy-s-api
const (
	requestsPerSecond = 15
	burstSize         = 15
)

const (
	maxRetries     = 2
	retryWaitMin   = 1 * time.Second
	retryWaitMax   = 3 * time.Second
	attemptTimeout = 30 * time.Second
)

// logFunc receives one retry. The signature matches what a closure over
// tflog.Warn provides, which keeps terraform-plugin-log out of this package.
type logFunc func(ctx context.Context, msg string, fields map[string]any)

type retrier struct {
	log logFunc
}

// backoff returns retryablehttp.DefaultBackoff unchanged and logs the wait.
// DefaultBackoff returns Retry-After verbatim when the response carries it, so
// a lockout can stall for as long as the server asks. That is accepted, and the
// logged wait is how it gets noticed.
//
// Backoff is the only hook that sees the attempt and the wait together, and
// retryablehttp calls it once per retry, after it has decided to retry.
func (r retrier) backoff(waitMin, waitMax time.Duration, attempt int, resp *http.Response) time.Duration {
	wait := retryablehttp.DefaultBackoff(waitMin, waitMax, attempt, resp)
	if r.log == nil || resp == nil || resp.Request == nil {
		return wait
	}
	r.log(resp.Request.Context(), "Pipefy API returned HTTP 429, retrying", map[string]any{
		"attempt":     attempt + 1,
		"wait":        wait.String(),
		"retry_after": resp.Header.Get("Retry-After"),
		"url":         resp.Request.URL.Redacted(),
	})
	return wait
}
