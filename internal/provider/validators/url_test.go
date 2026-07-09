// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestURL(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"null is allowed", types.StringNull(), false},
		{"unknown is allowed", types.StringUnknown(), false},
		{"https with path", types.StringValue("https://example.com/hook"), false},
		{"https bare host", types.StringValue("https://example.com"), false},
		{"http localhost with port", types.StringValue("http://localhost:3000/graphql"), false},
		{"empty string", types.StringValue(""), true},
		{"missing scheme", types.StringValue("example.com/hook"), true},
		{"missing host", types.StringValue("https://"), true},
		{"unsupported scheme", types.StringValue("ftp://example.com"), true},
		{"whitespace", types.StringValue(" https://example.com"), true},
	}

	v := URL()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("url"),
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
