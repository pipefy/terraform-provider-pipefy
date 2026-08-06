// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package pipefy talks to the Pipefy GraphQL API. It holds every operation the
// provider sends, and knows nothing about Terraform.
package pipefy

import "context"

// GraphQLDoer runs one GraphQL document and decodes data into out. It decodes
// data even when the response carries top-level errors, so a caller can read a
// payload the API returns alongside them.
type GraphQLDoer interface {
	DoGraphQL(ctx context.Context, query string, vars map[string]any, out any) error
}

// Locking rule for this package: an exported method may acquire a repo lock, an
// unexported one never does and never assumes one is held, and no exported
// method is called while a lock is held. locks.LockRepo uses a plain
// sync.Mutex, which is not reentrant, so breaking the last rule deadlocks rather
// than failing a test.

// Client is the entry point to the API, one service per entity.
type Client struct {
	gql GraphQLDoer

	Labels *LabelService
	Pipes  *PipeService
	Phases *PhaseService
	Fields *FieldService
	Tables *TableService

	TableFields *TableFieldService
	Webhooks    *WebhookService

	PipeRelations *PipeRelationService

	Automations     *AutomationService
	FieldConditions *FieldConditionService
	AiAgents        *AiAgentService
}

// New wires a client onto a transport.
func New(gql GraphQLDoer) *Client {
	c := &Client{gql: gql}
	c.Labels = &LabelService{c: c}
	c.Pipes = &PipeService{c: c}
	c.Phases = &PhaseService{c: c}
	c.Fields = &FieldService{c: c}
	c.Tables = &TableService{c: c}
	c.TableFields = &TableFieldService{c: c}
	c.Webhooks = &WebhookService{c: c}
	c.PipeRelations = &PipeRelationService{c: c}
	c.Automations = &AutomationService{c: c}
	c.FieldConditions = &FieldConditionService{c: c}
	c.AiAgents = &AiAgentService{c: c}
	return c
}

// setIf adds key to vars only when value is non-nil. A nil pointer means "leave
// this out of the request", which is how an attribute the config omits keeps its
// server value instead of being cleared.
func setIf[T any](vars map[string]any, key string, value *T) {
	if value != nil {
		vars[key] = *value
	}
}

// do runs a document and tags any failure with the operation that caused it.
// out is still populated when this returns an error, because the transport
// decodes data alongside a GraphQL errors array.
func (c *Client) do(ctx context.Context, op, document string, vars map[string]any, out any) error {
	if err := c.gql.DoGraphQL(ctx, document, vars, out); err != nil {
		return &APIError{Op: op, err: err}
	}
	return nil
}
