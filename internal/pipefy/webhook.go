// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"context"
	"encoding/json"
)

// webhookSelection leaves out headers on purpose: they are sensitive and never
// read back. filters is selected so drift in it is detected.
const webhookSelection = "id name url actions filters"

const createWebhookMutation = "mutation CreateWebhook_tf($input:CreateWebhookInput!){ createWebhook(input:$input){ webhook{ " + webhookSelection + " } } }"

const getPipeWebhooksQuery = "query GetPipeWebhooks_tf($pipeId:ID!){ pipe(id:$pipeId){ webhooks{ " + webhookSelection + " } } }"

// updateWebhookMutation selects only the id: the resource keeps the values it
// planned rather than reading them back.
const updateWebhookMutation = "mutation UpdateWebhook_tf($input:UpdateWebhookInput!){ updateWebhook(input:$input){ webhook{ id } } }"

const deleteWebhookMutation = "mutation DeleteWebhook_tf($id:ID!){ deleteWebhook(input:{ id:$id }){ success } }"

// Webhook is a pipe webhook. Filters stays raw so large numeric ids never
// round-trip through float64; the caller decides how to compare it.
type Webhook struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	URL     string          `json:"url"`
	Actions []string        `json:"actions"`
	Filters json.RawMessage `json:"filters"`
}

// WebhookService reads and writes pipe webhooks.
type WebhookService struct{ c *Client }

// Create adds a webhook to a pipe. The input stays a map because the two payload
// keys are not symmetric: headers goes out as a JSON string, filters as an
// object, and both take an explicit nil to clear. Building that is a Terraform
// decision, so it stays in the resource.
func (s *WebhookService) Create(ctx context.Context, input map[string]any) (Webhook, error) {
	var out struct {
		CreateWebhook struct {
			Webhook Webhook `json:"webhook"`
		} `json:"createWebhook"`
	}
	if err := s.c.do(ctx, "CreateWebhook_tf", createWebhookMutation, map[string]any{"input": input}, &out); err != nil {
		return Webhook{}, err
	}
	return out.CreateWebhook.Webhook, nil
}

// Get returns one webhook of a pipe. Pipe.webhooks is a plain list, so this
// reads them all and picks webhookID out.
func (s *WebhookService) Get(ctx context.Context, pipeID, webhookID string) (Webhook, error) {
	var out struct {
		Pipe *struct {
			Webhooks []Webhook `json:"webhooks"`
		} `json:"pipe"`
	}
	if err := s.c.do(ctx, "GetPipeWebhooks_tf", getPipeWebhooksQuery, map[string]any{"pipeId": pipeID}, &out); err != nil {
		return Webhook{}, err
	}
	if out.Pipe == nil {
		return Webhook{}, ErrNotFound
	}
	for _, webhook := range out.Pipe.Webhooks {
		if webhook.ID == webhookID {
			return webhook, nil
		}
	}
	return Webhook{}, ErrNotFound
}

// Update applies webhook settings. It returns no webhook because the mutation
// selects only the id.
func (s *WebhookService) Update(ctx context.Context, input map[string]any) error {
	var out struct {
		UpdateWebhook struct {
			Webhook struct {
				ID string `json:"id"`
			} `json:"webhook"`
		} `json:"updateWebhook"`
	}
	return s.c.do(ctx, "UpdateWebhook_tf", updateWebhookMutation, map[string]any{"input": input}, &out)
}

// Delete removes a webhook.
func (s *WebhookService) Delete(ctx context.Context, id string) error {
	var out struct {
		DeleteWebhook struct {
			Success bool `json:"success"`
		} `json:"deleteWebhook"`
	}
	return s.c.do(ctx, "DeleteWebhook_tf", deleteWebhookMutation, map[string]any{"id": id}, &out)
}
