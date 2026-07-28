// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ConditionAnyOfMinSize returns a validator.List that rejects a single-element
// any_of. any_of=[x] flattens to the same wire group as all_of=[x], so Read
// cannot tell them apart and always reconstructs a lone group as all_of;
// allowing any_of=[x] in configuration would produce a permanent diff after the
// first apply.
func ConditionAnyOfMinSize() validator.List { return conditionAnyOfMinSizeValidator{} }

type conditionAnyOfMinSizeValidator struct{}

func (v conditionAnyOfMinSizeValidator) Description(_ context.Context) string {
	return "any_of must have at least 2 entries; a single entry is equivalent to all_of"
}

func (v conditionAnyOfMinSizeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v conditionAnyOfMinSizeValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if len(req.ConfigValue.Elements()) == 1 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid any_of",
			"any_of with a single entry is equivalent to all_of with that entry; use all_of instead",
		)
	}
}

// ConditionComparisonOrGroup returns a validator.Object for an any_of entry: it
// must be either a comparison (field + operation, optionally value) or a nested
// group (all_of with at least two comparisons), never both and never neither. A
// single-comparison all_of is rejected for the same reason as a single-entry
// any_of: it flattens to a group Read cannot distinguish from an inlined
// comparison, which would produce a permanent diff.
func ConditionComparisonOrGroup() validator.Object {
	return conditionComparisonOrGroupValidator{}
}

type conditionComparisonOrGroupValidator struct{}

func (v conditionComparisonOrGroupValidator) Description(_ context.Context) string {
	return "an any_of entry must be either a comparison (field + operation) or a group (all_of), not both"
}

func (v conditionComparisonOrGroupValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v conditionComparisonOrGroupValidator) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	attrs := req.ConfigValue.Attributes()
	field, _ := attrs["field"].(types.String)
	operation, _ := attrs["operation"].(types.String)
	value, _ := attrs["value"].(types.String)
	allOf, _ := attrs["all_of"].(types.List)

	// An unknown value is a reference to another resource's not-yet-computed
	// attribute (for example field = pipefy_field.x.internal_id before that
	// field is created), not an absent one. Only a null value means the
	// configuration genuinely omitted the attribute. Defer the whole check
	// until every discriminating attribute is known, the same way HexColor/URL/
	// SLADuration let individually unknown values through rather than guess.
	if field.IsUnknown() || operation.IsUnknown() || value.IsUnknown() || allOf.IsUnknown() {
		return
	}

	hasField := !field.IsNull()
	hasOperation := !operation.IsNull()
	hasValue := !value.IsNull()
	hasAllOf := !allOf.IsNull() && len(allOf.Elements()) > 0

	switch {
	case hasAllOf && (hasField || hasOperation || hasValue):
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid any_of entry",
			"an any_of entry cannot set both all_of and field/operation/value; use one or the other",
		)
	case !hasAllOf && !hasField:
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid any_of entry",
			"an any_of entry must set either all_of or field (with operation)",
		)
	case !hasAllOf && hasField && !hasOperation:
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid any_of entry",
			"an any_of entry with field must also set operation",
		)
	}

	if hasAllOf {
		if n := len(allOf.Elements()); n == 1 {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid any_of entry",
				"all_of with a single comparison is redundant; set field/operation/value on this any_of entry directly instead",
			)
		}
	}
}
