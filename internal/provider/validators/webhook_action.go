// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var validWebhookActionsSet = map[string]bool{
	// card actions
	"card.create":         true,
	"card.move":           true,
	"card.late":           true,
	"card.expired":        true,
	"card.overdue":        true,
	"card.done":           true,
	"card.delete":         true,
	"card.comment_create": true,
	"card.email_received": true,
	"card.field_update":   true,
	// user actions
	"user.removal_from_org":      true,
	"user.removal_from_pipe":     true,
	"user.removal_from_table":    true,
	"user.invitation_acceptance": true,
	"user.invitation_sent":       true,
	"user.role_set":              true,
	// audit log actions
	"audit_log.export_finished": true,
}

var validWebhookActionsSlice = []string{
	"card.create", "card.move", "card.late", "card.expired", "card.overdue",
	"card.done", "card.delete", "card.comment_create", "card.email_received", "card.field_update",
	"user.removal_from_org", "user.removal_from_pipe", "user.removal_from_table",
	"user.invitation_acceptance", "user.invitation_sent", "user.role_set",
	"audit_log.export_finished",
}

// WebhookActions returns a validator.Set that ensures every element is a
// known Pipefy webhook event name (sourced from Webhooks::Actions.all).
func WebhookActions() validator.Set { return webhookActionsValidator{} }

type webhookActionsValidator struct{}

func (v webhookActionsValidator) Description(_ context.Context) string {
	return "each action must be one of: " + strings.Join(validWebhookActionsSlice, ", ")
}

func (v webhookActionsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v webhookActionsValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var actions []string
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &actions, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, a := range actions {
		if !validWebhookActionsSet[a] {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid webhook action",
				`"`+a+`" is not a valid webhook action. Valid actions: `+strings.Join(validWebhookActionsSlice, ", "),
			)
		}
	}
}
