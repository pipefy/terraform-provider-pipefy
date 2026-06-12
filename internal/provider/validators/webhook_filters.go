// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// WebhookFilters returns a validator.String that ensures the value is a JSON
// object where each value is an array of numeric IDs.
//
// Example valid value: {"on_phase_id":[123,456]}.
func WebhookFilters() validator.String { return webhookFiltersValidator{} }

type webhookFiltersValidator struct{}

func (v webhookFiltersValidator) Description(_ context.Context) string {
	return `value must be a JSON object where each value is an array of numeric IDs, e.g. {"on_phase_id":[123]}`
}

func (v webhookFiltersValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v webhookFiltersValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid webhook filters",
			"filters must be a valid JSON object, got: "+err.Error(),
		)
		return
	}

	for key, val := range parsed {
		arr, ok := val.([]any)
		if !ok {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid webhook filters",
				`value for key "`+key+`" must be an array of numeric IDs, e.g. [123, 456]`,
			)
			continue
		}
		for _, elem := range arr {
			if _, ok := elem.(float64); !ok {
				resp.Diagnostics.AddAttributeError(
					req.Path,
					"Invalid webhook filters",
					`value for key "`+key+`" must contain only numeric IDs`,
				)
				break
			}
		}
	}
}
