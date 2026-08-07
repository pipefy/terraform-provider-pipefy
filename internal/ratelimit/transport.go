// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package ratelimit paces the provider's outgoing requests under the API's
// documented request budget and retries the requests it refuses with HTTP 429.
package ratelimit

import (
	"context"
	"net/http"
)

// limiter is the part of golang.org/x/time/rate this package uses. *rate.Limiter
// satisfies it as is.
type limiter interface {
	Wait(ctx context.Context) error
}

type limiterTransport struct {
	base    http.RoundTripper
	limiter limiter
}

func (t *limiterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}
