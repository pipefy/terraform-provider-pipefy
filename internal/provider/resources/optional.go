// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import "github.com/hashicorp/terraform-plugin-framework/types"

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
