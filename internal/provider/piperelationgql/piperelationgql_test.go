// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package piperelationgql_test

import (
	"testing"

	"github.com/pipefy/terraform-provider-pipefy/internal/provider/piperelationgql"
)

func TestFindByID(t *testing.T) {
	relations := []piperelationgql.Relation{
		{Id: "1", Name: "Sub-tasks"},
		{Id: "2", Name: "Approvals"},
	}
	cases := map[string]struct {
		id       string
		wantName string
		wantOk   bool
	}{
		"first":   {"1", "Sub-tasks", true},
		"second":  {"2", "Approvals", true},
		"missing": {"3", "", false},
		"empty":   {"", "", false},
	}
	for name, c := range cases {
		got, ok := piperelationgql.FindByID(relations, c.id)
		if ok != c.wantOk || got.Name != c.wantName {
			t.Errorf("%s: FindByID(%q) = (%q,%v), want (%q,%v)", name, c.id, got.Name, ok, c.wantName, c.wantOk)
		}
	}
}

func TestFindByID_EmptySlice(t *testing.T) {
	if _, ok := piperelationgql.FindByID(nil, "1"); ok {
		t.Error("FindByID(nil, ...) returned ok=true, want false")
	}
}
