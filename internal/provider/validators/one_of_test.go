// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

func TestOneOf(t *testing.T) {
	v := validators.OneOf("blue", "red")
	for _, tc := range []struct {
		val     types.String
		wantErr bool
	}{
		{types.StringValue("blue"), false},
		{types.StringValue("green"), true},
		{types.StringNull(), false},
		{types.StringUnknown(), false},
	} {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: tc.val}, resp)
		if resp.Diagnostics.HasError() != tc.wantErr {
			t.Errorf("value %v: hasErr=%v want %v", tc.val, resp.Diagnostics.HasError(), tc.wantErr)
		}
	}
}
