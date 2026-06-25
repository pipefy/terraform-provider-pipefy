// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package fieldgql_test

import (
	"testing"

	"github.com/pipefy/terraform-provider-pipefy/internal/provider/fieldgql"
)

func TestFindByUUID(t *testing.T) {
	fields := []fieldgql.Field{
		{Id: "name", Uuid: "uuid-1", Label: "Name"},
		{Id: "email", Uuid: "uuid-2", Label: "Email"},
	}
	cases := map[string]struct {
		uuid   string
		wantId string
		wantOk bool
	}{
		"first":   {"uuid-1", "name", true},
		"second":  {"uuid-2", "email", true},
		"missing": {"uuid-3", "", false},
		"empty":   {"", "", false},
	}
	for name, c := range cases {
		got, ok := fieldgql.FindByUUID(fields, c.uuid)
		if ok != c.wantOk || got.Id != c.wantId {
			t.Errorf("%s: FindByUUID(%q) = (%q,%v), want (%q,%v)", name, c.uuid, got.Id, ok, c.wantId, c.wantOk)
		}
	}
}

func TestFindByUUID_EmptySlice(t *testing.T) {
	if _, ok := fieldgql.FindByUUID(nil, "uuid-1"); ok {
		t.Error("FindByUUID(nil, ...) returned ok=true, want false")
	}
}
