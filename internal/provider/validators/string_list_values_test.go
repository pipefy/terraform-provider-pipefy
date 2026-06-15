// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

func TestStringListValues(t *testing.T) {
	v := validators.StringListValues("PreviousPhases", "Comments")
	mk := func(vals ...string) types.List {
		els := make([]attr.Value, len(vals))
		for i, s := range vals {
			els[i] = types.StringValue(s)
		}
		l, _ := types.ListValue(types.StringType, els)
		return l
	}
	for _, tc := range []struct {
		list    types.List
		wantErr bool
	}{
		{mk("PreviousPhases"), false},
		{mk("PreviousPhases", "Comments"), false},
		{mk("PreviousPhases", "Nope"), true},
	} {
		resp := &validator.ListResponse{}
		v.ValidateList(t.Context(), validator.ListRequest{ConfigValue: tc.list}, resp)
		if resp.Diagnostics.HasError() != tc.wantErr {
			t.Errorf("hasErr=%v want %v", resp.Diagnostics.HasError(), tc.wantErr)
		}
	}
}
