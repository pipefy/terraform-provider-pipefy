// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package ratelimit paces the provider's outgoing requests under the API's
// documented request budget and retries the requests it refuses with HTTP 429.
package ratelimit

import (
	"context"
	"net/http"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/time/rate"
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

type settings struct {
	maxRetries     int
	retryWaitMin   time.Duration
	retryWaitMax   time.Duration
	attemptTimeout time.Duration
}

// NewTransport returns a RoundTripper that paces requests under the API's
// documented budget and retries the ones it refuses with HTTP 429. A nil log
// disables retry logging.
func NewTransport(log logFunc) http.RoundTripper {
	s := settings{
		maxRetries:     maxRetries,
		retryWaitMin:   retryWaitMin,
		retryWaitMax:   retryWaitMax,
		attemptTimeout: attemptTimeout,
	}
	lim := rate.NewLimiter(rate.Limit(requestsPerSecond), burstSize)
	return newTransport(s, lim, cleanhttp.DefaultPooledTransport(), log)
}

func newTransport(s settings, lim limiter, base http.RoundTripper, log logFunc) http.RoundTripper {
	c := retryablehttp.NewClient()
	c.Logger = nil
	c.RetryMax = s.maxRetries
	c.RetryWaitMin = s.retryWaitMin
	c.RetryWaitMax = s.retryWaitMax
	c.CheckRetry = retryOn429
	c.Backoff = retrier{log: log}.backoff

	// The default handler replaces the last response with "giving up after N
	// attempts", which would hide the 429 from the caller that renders it.
	c.ErrorHandler = retryablehttp.PassthroughErrorHandler

	// retryablehttp calls HTTPClient.Do once per attempt, so this Timeout is a
	// per-attempt deadline covering the response body. limiterTransport sits
	// inside it: with a burst at or above Terraform's parallelism, the queue
	// wait stays under a second.
	//
	// limiterTransport deliberately has no CloseIdleConnections method.
	// retryablehttp calls it on every failure path, and forwarding it would tear
	// down the connection pool in the middle of a run.
	c.HTTPClient = &http.Client{
		Timeout:   s.attemptTimeout,
		Transport: &limiterTransport{base: base, limiter: lim},
	}

	return &retryablehttp.RoundTripper{Client: c}
}
