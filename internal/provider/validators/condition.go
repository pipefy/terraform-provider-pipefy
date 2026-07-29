// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ConditionAnyOfMinSize rejects an any_of with fewer than two entries, since
// neither size survives a round trip: any_of=[x] flattens to the same wire group
// as all_of=[x] and comes back as all_of, and any_of=[] flattens to the payload
// that clears the condition and comes back as nothing. The empty case needs
// checking here rather than through a size validator on the attribute, because
// ExactlyOneOf already counts an empty list as set.
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
	switch len(req.ConfigValue.Elements()) {
	case 0:
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid any_of",
			"any_of is empty, which describes no condition at all; give it at least 2 entries, or drop the condition",
		)
	case 1:
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid any_of",
			"any_of with a single entry is equivalent to all_of with that entry; use all_of instead",
		)
	}
}

// ConditionComparisonOrGroup requires an any_of entry to be either a comparison
// or a nested all_of group, never both and never neither. It rejects a
// single-comparison all_of for the same reason as a single-entry any_of: the
// nesting is invisible on the wire, so it would come back inlined.
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
	// attribute, not an absent one, and only a null means the configuration
	// omitted it. Terraform re-runs config validation on the apply walk with
	// values resolved, so deferring loses nothing but the earlier error.
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
