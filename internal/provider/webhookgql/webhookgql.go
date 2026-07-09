// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package webhookgql holds the GraphQL field selection and the typed webhook
// payload shared across the pipefy_webhook resource's reads. headers is omitted
// on purpose: it is sensitive and never refreshed from the API. filters is
// included so it can be reconciled for drift detection.
package webhookgql

import "encoding/json"

const Selection = "id name url actions filters"

type Webhook struct {
	Id      string          `json:"id"`
	Name    string          `json:"name"`
	Url     string          `json:"url"`
	Actions []string        `json:"actions"`
	Filters json.RawMessage `json:"filters"`
}

func FindByID(webhooks []Webhook, id string) (Webhook, bool) {
	for _, w := range webhooks {
		if w.Id == id {
			return w, true
		}
	}
	return Webhook{}, false
}
