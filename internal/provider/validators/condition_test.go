// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

func TestConditionAnyOfMinSize(t *testing.T) {
	v := validators.ConditionAnyOfMinSize()
	for _, tc := range []struct {
		name    string
		list    types.List
		wantErr bool
	}{
		{"null is allowed", types.ListNull(types.StringType), false},
		{"unknown is allowed", types.ListUnknown(types.StringType), false},
		{"two elements", mustStringList(t, "a", "b"), false},
		{"single element", mustStringList(t, "a"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.ListResponse{}
			v.ValidateList(t.Context(), validator.ListRequest{Path: path.Root("any_of"), ConfigValue: tc.list}, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("want err=%v, got err=%v (diagnostics: %v)", tc.wantErr, resp.Diagnostics.HasError(), resp.Diagnostics)
			}
		})
	}
}

func comparisonOrGroupObject(t *testing.T, field, operation, value string, allOf []string) types.Object {
	t.Helper()
	attrTypes := map[string]attr.Type{
		"field":     types.StringType,
		"operation": types.StringType,
		"value":     types.StringType,
		"all_of":    types.ListType{ElemType: types.StringType},
	}

	strOrNull := func(s string) attr.Value {
		if s == "" {
			return types.StringNull()
		}
		return types.StringValue(s)
	}

	allOfVal := types.ListNull(types.StringType)
	if allOf != nil {
		allOfVal = mustStringList(t, allOf...)
	}

	o, d := types.ObjectValue(attrTypes, map[string]attr.Value{
		"field":     strOrNull(field),
		"operation": strOrNull(operation),
		"value":     strOrNull(value),
		"all_of":    allOfVal,
	})
	if d.HasError() {
		t.Fatal(d)
	}
	return o
}

func TestConditionComparisonOrGroup(t *testing.T) {
	v := validators.ConditionComparisonOrGroup()
	for _, tc := range []struct {
		name    string
		obj     types.Object
		wantErr bool
	}{
		{"plain comparison with value", comparisonOrGroupObject(t, "1001", "equals", "Other", nil), false},
		{"plain comparison without value", comparisonOrGroupObject(t, "1001", "present", "", nil), false},
		{"group with two comparisons", comparisonOrGroupObject(t, "", "", "", []string{"a", "b"}), false},
		{"both comparison and group set", comparisonOrGroupObject(t, "1001", "equals", "", []string{"a", "b"}), true},
		{"neither comparison nor group set", comparisonOrGroupObject(t, "", "", "", nil), true},
		{"field without operation", comparisonOrGroupObject(t, "1001", "", "", nil), true},
		{"single-element all_of is redundant", comparisonOrGroupObject(t, "", "", "", []string{"a"}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.ObjectResponse{}
			v.ValidateObject(t.Context(), validator.ObjectRequest{Path: path.Root("any_of").AtListIndex(0), ConfigValue: tc.obj}, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("want err=%v, got err=%v (diagnostics: %v)", tc.wantErr, resp.Diagnostics.HasError(), resp.Diagnostics)
			}
		})
	}
}

func mustStringList(t *testing.T, values ...string) types.List {
	t.Helper()
	elems := make([]attr.Value, len(values))
	for i, v := range values {
		elems[i] = types.StringValue(v)
	}
	l, d := types.ListValue(types.StringType, elems)
	if d.HasError() {
		t.Fatal(d)
	}
	return l
}
