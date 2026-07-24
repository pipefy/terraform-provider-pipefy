// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// NonBlank returns a validator.String that rejects an empty or whitespace-only
// value. Null and unknown values pass through so the check composes with the
// schema's own required/optional handling instead of duplicating it.
func NonBlank() validator.String {
	return nonBlankValidator{}
}

type nonBlankValidator struct{}

func (v nonBlankValidator) Description(_ context.Context) string {
	return "value must not be empty or blank"
}

func (v nonBlankValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v nonBlankValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if strings.TrimSpace(req.ConfigValue.ValueString()) == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Blank value",
			"expected a non-empty, non-whitespace value",
		)
	}
}
