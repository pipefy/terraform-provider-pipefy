// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import "errors"

// ErrNotFound reports that the API did not return the requested object. It is
// raised from the shape of a successful response, a null node or a miss in a
// list, not from the presence of a GraphQL errors array. Callers decide what
// that means: a resource removes it from state, a data source raises an error.
var ErrNotFound = errors.New("not found")

// APIError wraps a failed request. Error renders the underlying transport
// message unchanged, because those strings reach Terraform diagnostics.
type APIError struct {
	Op  string
	err error
}

func (e *APIError) Error() string { return e.err.Error() }

func (e *APIError) Unwrap() error { return e.err }

// ErrorDetail is one entry of a mutation's error_details payload.
type ErrorDetail struct {
	ObjectName string   `json:"object_name"`
	ObjectKey  string   `json:"object_key"`
	Messages   []string `json:"messages"`
}

// ValidationError reports a mutation that returned error_details instead of the
// object. Details stays structured, because the caller renders it.
//
// A response can carry error_details and a top-level error at once, so the
// transport error is kept as well: a caller that finds nothing printable in
// Details can still fall back to it.
type ValidationError struct {
	Op      string
	Details []ErrorDetail
	err     error
}

func (e *ValidationError) Error() string {
	return e.Op + " returned error_details"
}

func (e *ValidationError) Unwrap() error { return e.err }
