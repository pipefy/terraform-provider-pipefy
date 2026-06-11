// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestHexColor(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"null is allowed", types.StringNull(), false},
		{"unknown is allowed", types.StringUnknown(), false},
		{"three digit lower", types.StringValue("#abc"), false},
		{"three digit upper", types.StringValue("#FFF"), false},
		{"six digit lower", types.StringValue("#ff00aa"), false},
		{"six digit upper", types.StringValue("#FF0000"), false},
		{"six digit mixed", types.StringValue("#FfA500"), false},
		{"missing hash", types.StringValue("FF0000"), true},
		{"too short", types.StringValue("#FF"), true},
		{"too long", types.StringValue("#FF00000"), true},
		{"non-hex char", types.StringValue("#GG0000"), true},
		{"with alpha", types.StringValue("#FF000000"), true},
		{"empty string", types.StringValue(""), true},
		{"whitespace", types.StringValue(" #FF0000"), true},
	}

	v := HexColor()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("color"),
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
