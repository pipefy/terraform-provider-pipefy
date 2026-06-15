// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipeapi_test

import (
	"testing"

	"github.com/pipefy/terraform-provider-pipefy/internal/provider/pipeapi"
)

func TestUnitNameToSeconds(t *testing.T) {
	cases := map[string]struct {
		name string
		want int64
		ok   bool
	}{
		"minutes": {"minutes", 60, true},
		"hours":   {"hours", 3600, true},
		"days":    {"days", 86400, true},
		"bad":     {"weeks", 0, false},
	}
	for n, c := range cases {
		got, ok := pipeapi.UnitNameToSeconds(c.name)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got (%d,%v), want (%d,%v)", n, got, ok, c.want, c.ok)
		}
	}
}

func TestUnitSecondsToName(t *testing.T) {
	cases := map[int64]struct {
		want string
		ok   bool
	}{
		60:    {"minutes", true},
		3600:  {"hours", true},
		86400: {"days", true},
		120:   {"", false},
	}
	for secs, c := range cases {
		got, ok := pipeapi.UnitSecondsToName(secs)
		if got != c.want || ok != c.ok {
			t.Errorf("%d: got (%q,%v), want (%q,%v)", secs, got, ok, c.want, c.ok)
		}
	}
}
