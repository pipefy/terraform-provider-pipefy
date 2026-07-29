// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package conditionschema_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/conditiongql"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/conditionschema"
)

func comparison(field, operation, value string) conditionschema.Comparison {
	c := conditionschema.Comparison{
		Field:     types.StringValue(field),
		Operation: types.StringValue(operation),
		Value:     types.StringNull(),
	}
	if value != "" {
		c.Value = types.StringValue(value)
	}
	return c
}

// inputJSON marshals an Input() payload so a test can assert the whole wire
// shape at once, key order included.
func inputJSON(t *testing.T, c *conditionschema.Condition) string {
	t.Helper()
	b, err := json.Marshal(c.Input())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// describe keeps the all_of/any_of nesting visible. Read assertions need it
// rather than the Input() form, which flattens away the difference between
// all_of=[x,y] and a lone any_of entry nesting the same two comparisons.
func describe(c *conditionschema.Condition) string {
	switch {
	case c == nil:
		return "nil"
	case c.AllOf != nil:
		return "all_of[" + describeComparisons(c.AllOf) + "]"
	case c.AnyOf == nil:
		return "empty"
	}
	entries := make([]string, len(c.AnyOf))
	for i, e := range c.AnyOf {
		if e.AllOf != nil {
			entries[i] = "all_of[" + describeComparisons(e.AllOf) + "]"
			continue
		}
		entries[i] = describeComparison(conditionschema.Comparison{Field: e.Field, Operation: e.Operation, Value: e.Value})
	}
	return "any_of[" + strings.Join(entries, " ") + "]"
}

func describeComparisons(cs []conditionschema.Comparison) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = describeComparison(c)
	}
	return strings.Join(parts, " ")
}

func describeComparison(c conditionschema.Comparison) string {
	return fmt.Sprintf("(%s %s %s)", c.Field, c.Operation, c.Value)
}

func TestInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		cond *conditionschema.Condition
		want string
	}{
		{
			name: "all_of becomes a single group",
			cond: &conditionschema.Condition{AllOf: []conditionschema.Comparison{
				comparison("1001", "equals", "Other"),
				comparison("2001", "present", ""),
			}},
			want: `{"expressions":[` +
				`{"field_address":"1001","operation":"equals","structure_id":"0","value":"Other"},` +
				`{"field_address":"2001","operation":"present","structure_id":"1"}` +
				`],"expressions_structure":[["0","1"]]}`,
		},
		{
			name: "any_of becomes one group per entry",
			cond: &conditionschema.Condition{AnyOf: []conditionschema.AnyOfEntry{
				{AllOf: []conditionschema.Comparison{
					comparison("1001", "equals", "Other"),
					comparison("2001", "equals", "High"),
				}},
				{Field: types.StringValue("3001"), Operation: types.StringValue("present"), Value: types.StringNull()},
			}},
			want: `{"expressions":[` +
				`{"field_address":"1001","operation":"equals","structure_id":"0","value":"Other"},` +
				`{"field_address":"2001","operation":"equals","structure_id":"1","value":"High"},` +
				`{"field_address":"3001","operation":"present","structure_id":"2"}` +
				`],"expressions_structure":[["0","1"],["2"]]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := inputJSON(t, tc.cond); got != tc.want {
				t.Fatalf("Input() mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestEmptyInput(t *testing.T) {
	b, err := json.Marshal(conditionschema.EmptyInput())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"expressions":[],"expressions_structure":[]}`
	if string(b) != want {
		t.Fatalf("EmptyInput() = %s, want %s", b, want)
	}
}

func payload(t *testing.T, raw string) *conditiongql.Condition {
	t.Helper()
	var p conditiongql.Condition
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return &p
}

func TestFromPayloadNoCondition(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"no expressions and no groups", `{"expressions":[],"expressions_structure":[]}`},
		{"expressions pruned server-side", `{"expressions":[],"expressions_structure":[["0"]]}`},
		{"groups pruned server-side", `{"expressions":[{"structure_id":"0","field_address":"1001","operation":"equals"}],"expressions_structure":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			if got := conditionschema.FromPayload(payload(t, tc.raw), &diags); got != nil {
				t.Fatalf("FromPayload() = %#v, want nil", got)
			}
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
		})
	}

	var diags diag.Diagnostics
	if got := conditionschema.FromPayload(nil, &diags); got != nil {
		t.Fatalf("FromPayload(nil) = %#v, want nil", got)
	}
}

func TestFromPayload(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want *conditionschema.Condition
	}{
		{
			name: "one group reconstructs as all_of",
			raw: `{"expressions":[` +
				`{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"},` +
				`{"structure_id":"1","field_address":"2001","operation":"equals","value":"High"}` +
				`],"expressions_structure":[["0","1"]]}`,
			want: &conditionschema.Condition{AllOf: []conditionschema.Comparison{
				comparison("1001", "equals", "Other"),
				comparison("2001", "equals", "High"),
			}},
		},
		{
			name: "several groups reconstruct as any_of, multi-comparison groups nested",
			raw: `{"expressions":[` +
				`{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"},` +
				`{"structure_id":"1","field_address":"2001","operation":"equals","value":"High"},` +
				`{"structure_id":"2","field_address":"3001","operation":"present","value":""}` +
				`],"expressions_structure":[["0","1"],["2"]]}`,
			want: &conditionschema.Condition{AnyOf: []conditionschema.AnyOfEntry{
				{AllOf: []conditionschema.Comparison{
					comparison("1001", "equals", "Other"),
					comparison("2001", "equals", "High"),
				}},
				{Field: types.StringValue("3001"), Operation: types.StringValue("present"), Value: types.StringNull()},
			}},
		},
		{
			name: "group order wins over expression order, and numeric structure elements are accepted",
			raw: `{"expressions":[` +
				`{"structure_id":"1","field_address":"2001","operation":"equals","value":"High"},` +
				`{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"}` +
				`],"expressions_structure":[[1],[0]]}`,
			want: &conditionschema.Condition{AnyOf: []conditionschema.AnyOfEntry{
				{Field: types.StringValue("2001"), Operation: types.StringValue("equals"), Value: types.StringValue("High")},
				{Field: types.StringValue("1001"), Operation: types.StringValue("equals"), Value: types.StringValue("Other")},
			}},
		},
		{
			name: "a blank value normalizes to null",
			raw: `{"expressions":[` +
				`{"structure_id":"0","field_address":"1001","operation":"present","value":""},` +
				`{"structure_id":"1","field_address":"2001","operation":"blank","value":"  "}` +
				`],"expressions_structure":[["0","1"]]}`,
			want: &conditionschema.Condition{AllOf: []conditionschema.Comparison{
				comparison("1001", "present", ""),
				comparison("2001", "blank", ""),
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := conditionschema.FromPayload(payload(t, tc.raw), &diags)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if gotShape, wantShape := describe(got), describe(tc.want); gotShape != wantShape {
				t.Fatalf("FromPayload() mismatch\n got: %s\nwant: %s", gotShape, wantShape)
			}
		})
	}
}

// TestFromPayloadOrphanStructureId covers a structure group referencing an id
// no expression carries. The API enforces the bijection, so a response like this
// is broken and gets reported rather than papered over.
func TestFromPayloadOrphanStructureId(t *testing.T) {
	var diags diag.Diagnostics
	conditionschema.FromPayload(payload(t, `{"expressions":[`+
		`{"structure_id":"0","field_address":"1001","operation":"equals","value":"Other"}`+
		`],"expressions_structure":[["0"],["7"]]}`), &diags)
	if !diags.HasError() {
		t.Fatal("expected a diagnostic for the unmatched structure_id")
	}
}

// TestRoundTrip requires FromPayload(Input(c)) to reproduce c itself, nesting
// included. That is what keeps a settled resource settled.
func TestRoundTrip(t *testing.T) {
	for _, cond := range []*conditionschema.Condition{
		{AllOf: []conditionschema.Comparison{comparison("1001", "equals", "Other")}},
		{AllOf: []conditionschema.Comparison{
			comparison("1001", "equals", "Other"),
			comparison("2001", "present", ""),
		}},
		{AnyOf: []conditionschema.AnyOfEntry{
			{AllOf: []conditionschema.Comparison{
				comparison("1001", "equals", "Other"),
				comparison("2001", "equals", "High"),
			}},
			{Field: types.StringValue("3001"), Operation: types.StringValue("present"), Value: types.StringNull()},
		}},
	} {
		raw, err := json.Marshal(cond.Input())
		if err != nil {
			t.Fatal(err)
		}
		var diags diag.Diagnostics
		got := conditionschema.FromPayload(payload(t, string(raw)), &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if gotShape, wantShape := describe(got), describe(cond); gotShape != wantShape {
			t.Fatalf("round trip mismatch\n got: %s\nwant: %s", gotShape, wantShape)
		}
	}
}
