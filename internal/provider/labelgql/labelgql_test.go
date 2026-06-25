// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package labelgql_test

import (
	"testing"

	"github.com/pipefy/terraform-provider-pipefy/internal/provider/labelgql"
)

func TestFindByID(t *testing.T) {
	labels := []labelgql.Label{
		{Id: "1", Name: "Bug", Color: "#FF0000"},
		{Id: "2", Name: "Feature", Color: "#00FF00"},
	}
	cases := map[string]struct {
		id       string
		wantName string
		wantOk   bool
	}{
		"first":   {"1", "Bug", true},
		"second":  {"2", "Feature", true},
		"missing": {"3", "", false},
		"empty":   {"", "", false},
	}
	for name, c := range cases {
		got, ok := labelgql.FindByID(labels, c.id)
		if ok != c.wantOk || got.Name != c.wantName {
			t.Errorf("%s: FindByID(%q) = (%q,%v), want (%q,%v)", name, c.id, got.Name, ok, c.wantName, c.wantOk)
		}
	}
}

func TestFindByID_EmptySlice(t *testing.T) {
	if _, ok := labelgql.FindByID(nil, "1"); ok {
		t.Error("FindByID(nil, ...) returned ok=true, want false")
	}
}
