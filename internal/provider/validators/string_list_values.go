// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// StringListValues returns a validator.List that requires every element to be
// one of the listed values. Null and unknown lists or elements pass through.
func StringListValues(allowed ...string) validator.List {
	return stringListValuesValidator{allowed: allowed}
}

type stringListValuesValidator struct{ allowed []string }

func (v stringListValuesValidator) Description(_ context.Context) string {
	return "every element must be one of: " + strings.Join(v.allowed, ", ")
}

func (v stringListValuesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringListValuesValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, el := range req.ConfigValue.Elements() {
		s, ok := el.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		if !slices.Contains(v.allowed, s.ValueString()) {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid value",
				fmt.Sprintf("expected each element to be one of [%s], got: %s", strings.Join(v.allowed, ", "), s.ValueString()),
			)
		}
	}
}
