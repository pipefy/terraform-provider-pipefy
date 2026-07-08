// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package webhookgql holds the GraphQL field selection and the typed webhook
// payload shared across the pipefy_webhook resource's reads. headers and
// filters are intentionally omitted: headers are sensitive and never refreshed
// from the API, and filters are kept as the user-supplied JSON string to avoid
// re-serialization diffs.
package webhookgql

const Selection = "id name url actions"

type Webhook struct {
	Id      string   `json:"id"`
	Name    string   `json:"name"`
	Url     string   `json:"url"`
	Actions []string `json:"actions"`
}

func FindByID(webhooks []Webhook, id string) (Webhook, bool) {
	for _, w := range webhooks {
		if w.Id == id {
			return w, true
		}
	}
	return Webhook{}, false
}
