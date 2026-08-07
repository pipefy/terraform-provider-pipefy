// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ratelimit

import (
	"context"
	"net/http"
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
