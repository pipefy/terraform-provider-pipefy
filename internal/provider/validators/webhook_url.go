// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// WebhookURL returns a validator.String that ensures the value is a valid
// HTTP or HTTPS URL.
func WebhookURL() validator.String { return webhookURLValidator{} }

type webhookURLValidator struct{}

func (v webhookURLValidator) Description(_ context.Context) string {
	return "value must be a valid HTTP or HTTPS URL"
}

func (v webhookURLValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v webhookURLValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	raw := req.ConfigValue.ValueString()
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid webhook URL",
			"expected a valid HTTP or HTTPS URL, got: "+raw,
		)
	}
}
