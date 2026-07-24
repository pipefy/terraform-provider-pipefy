// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNonBlank(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"null is allowed", types.StringNull(), false},
		{"unknown is allowed", types.StringUnknown(), false},
		{"non-empty value", types.StringValue("equals"), false},
		{"value with surrounding space", types.StringValue(" 427453916 "), false},
		{"empty string", types.StringValue(""), true},
		{"only spaces", types.StringValue("   "), true},
		{"only tab and newline", types.StringValue("\t\n"), true},
	}

	v := NonBlank()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("field"),
				ConfigValue: tc.value,
			}
			resp := &validator.StringResponse{}
			v.ValidateString(t.Context(), req, resp)
			gotErr := resp.Diagnostics.HasError()
			if gotErr != tc.wantErr {
				t.Fatalf("want err=%v, got err=%v (diagnostics: %v)", tc.wantErr, gotErr, resp.Diagnostics)
			}
		})
	}
}
