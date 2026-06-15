// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ListContains returns a validator.List that requires the list to include the
// given string value. A list that must contain a value is also non-empty. Null
// and unknown lists pass through.
func ListContains(required string) validator.List { return listContainsValidator{required: required} }

type listContainsValidator struct{ required string }

func (v listContainsValidator) Description(_ context.Context) string {
	return "list must include " + v.required
}

func (v listContainsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v listContainsValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, el := range req.ConfigValue.Elements() {
		if s, ok := el.(types.String); ok && !s.IsNull() && !s.IsUnknown() && s.ValueString() == v.required {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Missing required value",
		fmt.Sprintf("the list must include %q", v.required),
	)
}
