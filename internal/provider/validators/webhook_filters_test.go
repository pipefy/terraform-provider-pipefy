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

func TestWebhookFilters(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"null is allowed", types.StringNull(), false},
		{"unknown is allowed", types.StringUnknown(), false},
		{"valid single filter", types.StringValue(`{"on_phase_id":[123]}`), false},
		{"valid multiple IDs", types.StringValue(`{"on_phase_id":[1,2,3]}`), false},
		{"valid multiple keys", types.StringValue(`{"on_phase_id":[1],"on_assignee_id":[42]}`), false},
		{"empty object", types.StringValue(`{}`), false},
		{"invalid JSON", types.StringValue(`not json`), true},
		{"value is string not array", types.StringValue(`{"on_phase_id":"123"}`), true},
		{"value is number not array", types.StringValue(`{"on_phase_id":123}`), true},
		{"array contains string", types.StringValue(`{"on_phase_id":["abc"]}`), true},
		{"array contains bool", types.StringValue(`{"on_phase_id":[true]}`), true},
	}

	v := WebhookFilters()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("filters"),
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
