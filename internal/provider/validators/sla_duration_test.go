// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

func slaObject(t *testing.T, time int64, unit string) types.Object {
	o, d := types.ObjectValue(
		map[string]attr.Type{"time": types.Int64Type, "unit": types.StringType},
		map[string]attr.Value{"time": types.Int64Value(time), "unit": types.StringValue(unit)},
	)
	if d.HasError() {
		t.Fatal(d)
	}
	return o
}

func TestSLADuration(t *testing.T) {
	v := validators.SLADuration()
	for _, tc := range []struct {
		time    int64
		unit    string
		wantErr bool
	}{
		{59, "minutes", false},
		{60, "minutes", true},
		{23, "hours", false},
		{24, "hours", true},
		{365, "days", false},
		{0, "days", true},
	} {
		resp := &validator.ObjectResponse{}
		v.ValidateObject(context.Background(), validator.ObjectRequest{ConfigValue: slaObject(t, tc.time, tc.unit)}, resp)
		if resp.Diagnostics.HasError() != tc.wantErr {
			t.Errorf("(%d,%s): hasErr=%v want %v", tc.time, tc.unit, resp.Diagnostics.HasError(), tc.wantErr)
		}
	}
}
