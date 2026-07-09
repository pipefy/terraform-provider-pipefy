// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"strings"
	"testing"
)

func TestNormalizeFilters(t *testing.T) {
	cases := map[string]struct {
		raw      string
		wantNull bool
		wantVal  string
	}{
		"empty bytes":             {raw: "", wantNull: true},
		"json null":               {raw: "null", wantNull: true},
		"empty object":            {raw: "{}", wantNull: true},
		"empty object spaced":     {raw: "  { }  ", wantNull: true},
		"single filter":           {raw: `{"from_phase_id":[268]}`, wantVal: `{"from_phase_id":[268]}`},
		"trims surrounding space": {raw: `  {"from_phase_id":[268]}  `, wantVal: `{"from_phase_id":[268]}`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := normalizeFilters([]byte(tc.raw))
			if got.IsNull() != tc.wantNull {
				t.Fatalf("IsNull=%v, want %v", got.IsNull(), tc.wantNull)
			}
			if !tc.wantNull && got.ValueString() != tc.wantVal {
				t.Fatalf("ValueString=%q, want %q", got.ValueString(), tc.wantVal)
			}
		})
	}
}

// TestNormalizeFilters_LargeIntPrecision guards the reviewer's note: the API's
// raw JSON must be preserved verbatim so a numeric ID above 2^53 does not lose
// precision through a float64 round-trip.
func TestNormalizeFilters_LargeIntPrecision(t *testing.T) {
	const bigID = "9007199254740993" // 2^53 + 1, not representable as float64
	raw := `{"field_id":[` + bigID + `]}`

	got := normalizeFilters([]byte(raw))
	if got.IsNull() {
		t.Fatal("expected a value, got null")
	}
	if !strings.Contains(got.ValueString(), bigID) {
		t.Fatalf("expected exact ID %s to be preserved, got %q", bigID, got.ValueString())
	}
}
