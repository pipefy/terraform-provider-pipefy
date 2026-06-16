// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestWebhookURL(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"valid https URL", types.StringValue("https://example.com/hook"), false},
		{"valid http URL", types.StringValue("http://example.com/hook"), false},
		{"https with path and query", types.StringValue("https://example.com/hook?token=abc"), false},
		{"ftp scheme is rejected", types.StringValue("ftp://example.com/hook"), true},
		{"no scheme bare host", types.StringValue("example.com/hook"), true},
		{"missing host", types.StringValue("https://"), true},
		{"empty string", types.StringValue(""), true},
	}

	v := WebhookURL()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("url"),
				ConfigValue: tc.value,
			}
			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), req, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("want err=%v, got err=%v (diagnostics: %v)", tc.wantErr, resp.Diagnostics.HasError(), resp.Diagnostics)
			}
		})
	}
}