// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// ValidWebhookActionsSlice is the authoritative list of accepted action names.
var ValidWebhookActionsSlice = []string{
	"card.create", "card.move", "card.late", "card.expired", "card.overdue",
	"card.done", "card.delete", "card.comment_create", "card.email_received", "card.field_update",
	"user.removal_from_org", "user.removal_from_pipe", "user.removal_from_table",
	"user.invitation_acceptance", "user.invitation_sent", "user.role_set",
	"audit_log.export_finished",
}

var validWebhookActionsSet = func() map[string]bool {
	m := make(map[string]bool, len(ValidWebhookActionsSlice))
	for _, a := range ValidWebhookActionsSlice {
		m[a] = true
	}
	return m
}()

func WebhookActions() validator.Set { return webhookActionsValidator{} }

type webhookActionsValidator struct{}

func (v webhookActionsValidator) Description(_ context.Context) string {
	return "each action must be one of: " + strings.Join(ValidWebhookActionsSlice, ", ")
}

func (v webhookActionsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v webhookActionsValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	var actions []string
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &actions, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(actions) == 0 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid webhook actions",
			"at least one action must be specified",
		)
		return
	}
	for _, a := range actions {
		if !validWebhookActionsSet[a] {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid webhook action",
				`"`+a+`" is not a valid webhook action. Valid actions: `+strings.Join(ValidWebhookActionsSlice, ", "),
			)
		}
	}
}
