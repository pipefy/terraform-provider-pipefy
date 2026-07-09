// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package webhookgql_test

import (
	"testing"

	"github.com/pipefy/terraform-provider-pipefy/internal/provider/webhookgql"
)

func TestFindByID(t *testing.T) {
	webhooks := []webhookgql.Webhook{
		{Id: "1", Name: "Card created", Url: "https://a.example/hook", Actions: []string{"card.create"}},
		{Id: "2", Name: "Card moved", Url: "https://b.example/hook", Actions: []string{"card.move"}},
	}
	cases := map[string]struct {
		id       string
		wantName string
		wantOk   bool
	}{
		"first":   {"1", "Card created", true},
		"second":  {"2", "Card moved", true},
		"missing": {"3", "", false},
		"empty":   {"", "", false},
	}
	for name, c := range cases {
		got, ok := webhookgql.FindByID(webhooks, c.id)
		if ok != c.wantOk || got.Name != c.wantName {
			t.Errorf("%s: FindByID(%q) = (%q,%v), want (%q,%v)", name, c.id, got.Name, ok, c.wantName, c.wantOk)
		}
	}
}

func TestFindByID_EmptySlice(t *testing.T) {
	if _, ok := webhookgql.FindByID(nil, "1"); ok {
		t.Error("FindByID(nil, ...) returned ok=true, want false")
	}
}
