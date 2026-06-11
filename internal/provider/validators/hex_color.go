// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var hexColorPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// HexColor returns a validator.String that ensures the value is a CSS-style
// hex color: a leading "#" followed by exactly 3 or 6 hexadecimal digits
// (e.g. "#FFF", "#FF0000"). Empty and unknown values are allowed through so
// optional / computed attributes are not rejected; required-attribute checks
// are handled by the schema itself.
func HexColor() validator.String {
	return hexColorValidator{}
}

type hexColorValidator struct{}

func (v hexColorValidator) Description(_ context.Context) string {
	return "value must be a hex color code like #RGB or #RRGGBB"
}

func (v hexColorValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v hexColorValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueString()
	if !hexColorPattern.MatchString(value) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid hex color",
			"expected a hex color like #RGB or #RRGGBB, got: "+value,
		)
	}
}
