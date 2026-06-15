// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// OneOf returns a validator.String that accepts only the listed values. Null
// and unknown values pass through so optional / computed attributes are not
// rejected.
func OneOf(allowed ...string) validator.String { return oneOfValidator{allowed: allowed} }

type oneOfValidator struct{ allowed []string }

func (v oneOfValidator) Description(_ context.Context) string {
	return "value must be one of: " + strings.Join(v.allowed, ", ")
}

func (v oneOfValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v oneOfValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueString()
	if !slices.Contains(v.allowed, value) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid value",
			fmt.Sprintf("expected one of [%s], got: %s", strings.Join(v.allowed, ", "), value),
		)
	}
}
