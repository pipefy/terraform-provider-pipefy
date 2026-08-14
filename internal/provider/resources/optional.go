// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The optional* helpers turn a Terraform attribute into the pointer the SDK's
// input structs take: nil when the attribute has no concrete value, so an
// omitted Optional+Computed attribute is left out of the request and keeps its
// server value instead of being cleared.

func optionalBool(v types.Bool) *bool {
	if !hasValue(v) {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func optionalString(v types.String) *string {
	if !hasValue(v) {
		return nil
	}
	s := v.ValueString()
	return &s
}

func optionalInt64(v types.Int64) *int64 {
	if !hasValue(v) {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

func optionalFloat64(v types.Float64) *float64 {
	if !hasValue(v) {
		return nil
	}
	f := v.ValueFloat64()
	return &f
}

func hasValue(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }

func fillUnknownString(planned, fromAPI types.String) types.String {
	if planned.IsUnknown() {
		return fromAPI
	}
	return planned
}

func fillUnknownBool(planned, fromAPI types.Bool) types.Bool {
	if planned.IsUnknown() {
		return fromAPI
	}
	return planned
}

// mergeEmptyish keeps the model's value when it and the API's are both empty or
// null. The API stores a written "" for custom_validation as NULL, yet a field
// last written outside the GraphQL API still reads back as "", and the two mean
// the same thing. Any other case takes the API value, so drift still surfaces.
// custom_validation only: description and help store "" verbatim.
func mergeEmptyish(current types.String, api *string) types.String {
	apiEmpty := api == nil || *api == ""
	modelEmpty := !current.IsUnknown() && (current.IsNull() || current.ValueString() == "")
	if apiEmpty && modelEmpty {
		return current
	}
	return types.StringPointerValue(api)
}
