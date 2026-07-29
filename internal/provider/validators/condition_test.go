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
		{"empty list", mustStringList(t), true},
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

var comparisonAttrTypes = map[string]attr.Type{
	"field":     types.StringType,
	"operation": types.StringType,
	"value":     types.StringType,
}

var anyOfEntryAttrTypes = map[string]attr.Type{
	"field":     types.StringType,
	"operation": types.StringType,
	"value":     types.StringType,
	"all_of":    types.ListType{ElemType: types.ObjectType{AttrTypes: comparisonAttrTypes}},
}

// comparisonList builds a nested all_of the way the schema types it, a list of
// comparison objects, so the validator sees the object it sees in production
// rather than a stand-in.
func comparisonList(t *testing.T, fields ...string) types.List {
	t.Helper()
	elems := make([]attr.Value, len(fields))
	for i, f := range fields {
		o, d := types.ObjectValue(comparisonAttrTypes, map[string]attr.Value{
			"field":     types.StringValue(f),
			"operation": types.StringValue("equals"),
			"value":     types.StringNull(),
		})
		if d.HasError() {
			t.Fatal(d)
		}
		elems[i] = o
	}
	l, d := types.ListValue(types.ObjectType{AttrTypes: comparisonAttrTypes}, elems)
	if d.HasError() {
		t.Fatal(d)
	}
	return l
}

func anyOfEntry(t *testing.T, attrs map[string]attr.Value) types.Object {
	t.Helper()
	full := map[string]attr.Value{
		"field":     types.StringNull(),
		"operation": types.StringNull(),
		"value":     types.StringNull(),
		"all_of":    types.ListNull(types.ObjectType{AttrTypes: comparisonAttrTypes}),
	}
	for k, v := range attrs {
		full[k] = v
	}
	o, d := types.ObjectValue(anyOfEntryAttrTypes, full)
	if d.HasError() {
		t.Fatal(d)
	}
	return o
}

func comparisonOrGroupObject(t *testing.T, field, operation, value string, allOf []string) types.Object {
	t.Helper()
	strOrNull := func(s string) attr.Value {
		if s == "" {
			return types.StringNull()
		}
		return types.StringValue(s)
	}
	attrs := map[string]attr.Value{
		"field":     strOrNull(field),
		"operation": strOrNull(operation),
		"value":     strOrNull(value),
	}
	if allOf != nil {
		attrs["all_of"] = comparisonList(t, allOf...)
	}
	return anyOfEntry(t, attrs)
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
		{"group with two comparisons", comparisonOrGroupObject(t, "", "", "", []string{"1001", "2001"}), false},
		{"both comparison and group set", comparisonOrGroupObject(t, "1001", "equals", "", []string{"1001", "2001"}), true},
		{"neither comparison nor group set", comparisonOrGroupObject(t, "", "", "", nil), true},
		{"field without operation", comparisonOrGroupObject(t, "1001", "", "", nil), true},
		{"single-element all_of is redundant", comparisonOrGroupObject(t, "", "", "", []string{"1001"}), true},

		// An unknown attribute is a reference to something not yet created, so
		// the entry cannot be judged yet. Terraform validates the config again
		// on the apply walk with the reference resolved, which is where an
		// entry like the second one below is caught.
		{"unknown field alone", anyOfEntry(t, map[string]attr.Value{"field": types.StringUnknown()}), false},
		{"unknown field alongside a group", anyOfEntry(t, map[string]attr.Value{
			"field":  types.StringUnknown(),
			"all_of": comparisonList(t, "1001", "2001"),
		}), false},
		{"unknown all_of", anyOfEntry(t, map[string]attr.Value{
			"all_of": types.ListUnknown(types.ObjectType{AttrTypes: comparisonAttrTypes}),
		}), false},
		{"null object", types.ObjectNull(anyOfEntryAttrTypes), false},
		{"unknown object", types.ObjectUnknown(anyOfEntryAttrTypes), false},
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
