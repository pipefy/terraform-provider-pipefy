// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func mustSetValue(vals []string) types.Set {
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	s, err := types.SetValue(types.StringType, elems)
	if err != nil {
		panic(err)
	}
	return s
}

func TestWebhookActions(t *testing.T) {
	cases := []struct {
		name    string
		value   types.Set
		wantErr bool
	}{
		{"empty set is rejected", mustSetValue([]string{}), true},
		{"single valid action", mustSetValue([]string{"card.create"}), false},
		{"multiple valid actions", mustSetValue([]string{"card.create", "card.move"}), false},
		{"invalid action", mustSetValue([]string{"not.valid"}), true},
		{"mix valid and invalid", mustSetValue([]string{"card.create", "not.valid"}), true},
		{"all known valid actions", mustSetValue(ValidWebhookActionsSlice), false},
	}

	v := WebhookActions()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.SetRequest{
				Path:        path.Root("actions"),
				ConfigValue: tc.value,
			}
			resp := &validator.SetResponse{}
			v.ValidateSet(context.Background(), req, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("want err=%v, got err=%v (diagnostics: %v)", tc.wantErr, resp.Diagnostics.HasError(), resp.Diagnostics)
			}
		})
	}
}
