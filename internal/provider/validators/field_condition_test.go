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

func actionObject(t *testing.T, whenTrue, whenFalse string) types.Object {
	t.Helper()
	strOrNull := func(s string) attr.Value {
		if s == "" {
			return types.StringNull()
		}
		return types.StringValue(s)
	}
	o, d := types.ObjectValue(
		map[string]attr.Type{"when_true": types.StringType, "when_false": types.StringType},
		map[string]attr.Value{"when_true": strOrNull(whenTrue), "when_false": strOrNull(whenFalse)},
	)
	if d.HasError() {
		t.Fatal(d)
	}
	return o
}

func TestFieldConditionActionHasBranch(t *testing.T) {
	v := validators.FieldConditionActionHasBranch()
	for _, tc := range []struct {
		name                string
		whenTrue, whenFalse string
		wantErr             bool
	}{
		{"only when_true", "show", "", false},
		{"only when_false", "", "hide", false},
		{"both set", "show", "hide", false},
		{"neither set", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.ObjectResponse{}
			v.ValidateObject(t.Context(), validator.ObjectRequest{Path: path.Root("actions").AtListIndex(0), ConfigValue: actionObject(t, tc.whenTrue, tc.whenFalse)}, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("want err=%v, got err=%v (diagnostics: %v)", tc.wantErr, resp.Diagnostics.HasError(), resp.Diagnostics)
			}
		})
	}
}

func actionsListObject(t *testing.T, fields ...string) types.List {
	t.Helper()
	attrTypes := map[string]attr.Type{"field": types.StringType}
	elems := make([]attr.Value, len(fields))
	for i, f := range fields {
		o, d := types.ObjectValue(attrTypes, map[string]attr.Value{"field": types.StringValue(f)})
		if d.HasError() {
			t.Fatal(d)
		}
		elems[i] = o
	}
	l, d := types.ListValue(types.ObjectType{AttrTypes: attrTypes}, elems)
	if d.HasError() {
		t.Fatal(d)
	}
	return l
}

func TestFieldConditionActionsUniqueField(t *testing.T) {
	v := validators.FieldConditionActionsUniqueField()
	for _, tc := range []struct {
		name    string
		fields  []string
		wantErr bool
	}{
		{"distinct fields", []string{"1001", "1002"}, false},
		{"single field", []string{"1001"}, false},
		{"duplicate field", []string{"1001", "1001"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.ListResponse{}
			v.ValidateList(t.Context(), validator.ListRequest{Path: path.Root("actions"), ConfigValue: actionsListObject(t, tc.fields...)}, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("want err=%v, got err=%v (diagnostics: %v)", tc.wantErr, resp.Diagnostics.HasError(), resp.Diagnostics)
			}
		})
	}
}
