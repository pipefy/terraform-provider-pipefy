// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// FieldConditionActionHasBranch returns a validator.Object requiring an
// action to set when_true, when_false, or both. whenEvaluator is optional on
// the API's action input, and an action only applies on the branch it is
// tagged with, so a rule may legitimately define just one branch.
func FieldConditionActionHasBranch() validator.Object {
	return fieldConditionActionHasBranchValidator{}
}

type fieldConditionActionHasBranchValidator struct{}

func (v fieldConditionActionHasBranchValidator) Description(_ context.Context) string {
	return "an action must set when_true, when_false, or both"
}

func (v fieldConditionActionHasBranchValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v fieldConditionActionHasBranchValidator) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	attrs := req.ConfigValue.Attributes()
	whenTrue, _ := attrs["when_true"].(types.String)
	whenFalse, _ := attrs["when_false"].(types.String)
	if whenTrue.IsUnknown() || whenFalse.IsUnknown() {
		return
	}
	hasTrue := !whenTrue.IsNull()
	hasFalse := !whenFalse.IsNull()
	if !hasTrue && !hasFalse {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid action",
			"an action must set when_true, when_false, or both",
		)
	}
}

// FieldConditionActionsUniqueField returns a validator.List rejecting more
// than one actions entry for the same field. Read groups the API's actions
// by target field to reconstruct one entry per field, so two config entries
// for the same field would collapse into one on the next read.
func FieldConditionActionsUniqueField() validator.List {
	return fieldConditionActionsUniqueFieldValidator{}
}

type fieldConditionActionsUniqueFieldValidator struct{}

func (v fieldConditionActionsUniqueFieldValidator) Description(_ context.Context) string {
	return "each action must target a distinct field"
}

func (v fieldConditionActionsUniqueFieldValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v fieldConditionActionsUniqueFieldValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	seen := make(map[string]bool, len(req.ConfigValue.Elements()))
	for _, elem := range req.ConfigValue.Elements() {
		obj, ok := elem.(types.Object)
		if !ok {
			continue
		}
		fieldVal, _ := obj.Attributes()["field"].(types.String)
		if fieldVal.IsNull() || fieldVal.IsUnknown() {
			continue
		}
		f := fieldVal.ValueString()
		if seen[f] {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Duplicate action field",
				fmt.Sprintf("more than one action targets field %q; combine when_true/when_false into a single entry", f),
			)
			return
		}
		seen[f] = true
	}
}
