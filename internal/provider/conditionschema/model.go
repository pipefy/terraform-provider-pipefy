// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package conditionschema models the Pipefy condition grammar shared by every
// resource that carries one. The schema attributes, the flattening into the
// wire's expressions plus expressions_structure, and the canonicalization back
// into all_of/any_of live here so pipefy_automation and pipefy_field_condition
// cannot drift.
package conditionschema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/conditiongql"
)

// Condition holds exactly one of all_of or any_of, which the schema's
// ExactlyOneOf enforces.
type Condition struct {
	AllOf []Comparison `tfsdk:"all_of"`
	AnyOf []AnyOfEntry `tfsdk:"any_of"`
}

type Comparison struct {
	Field     types.String `tfsdk:"field"`
	Operation types.String `tfsdk:"operation"`
	Value     types.String `tfsdk:"value"`
}

// AnyOfEntry is a sum type: either a comparison (field + operation, optionally
// value) or a nested all_of group, never both. The ConditionComparisonOrGroup
// validator enforces that exclusivity.
type AnyOfEntry struct {
	Field     types.String `tfsdk:"field"`
	Operation types.String `tfsdk:"operation"`
	Value     types.String `tfsdk:"value"`
	AllOf     []Comparison `tfsdk:"all_of"`
}

// Input flattens all_of/any_of into the wire ConditionInput shape: a flat
// expressions list plus expressions_structure, assigning fresh sequential
// structure_ids in flattening order. The ids are sent as strings, which is
// what reads return, so a mutation echo round-trips unchanged.
func (c *Condition) Input() map[string]any {
	groups := c.comparisonGroups()

	exprs := make([]map[string]any, 0, len(groups))
	structure := make([][]string, len(groups))

	nextID := 0
	for gi, g := range groups {
		row := make([]string, len(g))
		for ei, cmp := range g {
			id := strconv.Itoa(nextID)
			expr := map[string]any{
				"structure_id":  id,
				"field_address": cmp.Field.ValueString(),
				"operation":     cmp.Operation.ValueString(),
			}
			if !cmp.Value.IsNull() && !cmp.Value.IsUnknown() {
				expr["value"] = cmp.Value.ValueString()
			}
			exprs = append(exprs, expr)
			row[ei] = id
			nextID++
		}
		structure[gi] = row
	}

	return map[string]any{
		"expressions":           exprs,
		"expressions_structure": structure,
	}
}

// EmptyInput is the ConditionInput that clears a condition. The API accepts it
// on both create and update, while an explicit null condition errors, so a
// resource that manages its condition in full sends this when the block is
// absent from configuration.
func EmptyInput() map[string]any {
	return map[string]any{
		"expressions":           []map[string]any{},
		"expressions_structure": [][]string{},
	}
}

// comparisonGroups expands all_of/any_of into the ordered list of AND-groups
// the wire format expects: a single group for all_of, or one group per any_of
// entry (that entry's all_of if set, otherwise a one-comparison group built
// from its own field/operation/value).
func (c *Condition) comparisonGroups() [][]Comparison {
	if len(c.AllOf) > 0 {
		return [][]Comparison{c.AllOf}
	}
	groups := make([][]Comparison, len(c.AnyOf))
	for i, entry := range c.AnyOf {
		if len(entry.AllOf) > 0 {
			groups[i] = entry.AllOf
			continue
		}
		groups[i] = []Comparison{{
			Field:     entry.Field,
			Operation: entry.Operation,
			Value:     entry.Value,
		}}
	}
	return groups
}

// FromPayload reconstructs all_of/any_of from the API's flat expressions plus
// expressions_structure, canonicalizing deterministically so Read never has to
// guess between two configurations that flatten identically: a single group is
// always all_of; multiple groups are always any_of, and within any_of a
// single-comparison group is a plain entry while a multi-comparison group is a
// nested all_of. Without this rule, all_of=[x] and any_of=[x] (or a nested
// all_of=[x] inside an any_of entry) are indistinguishable on the wire, and
// reconstructing the "wrong" legal shape would produce a permanent diff. The
// ConditionAnyOfMinSize and ConditionComparisonOrGroup validators reject the
// shapes this cannot reproduce.
//
// A payload carrying no condition at all maps to nil. Callers whose condition
// attribute is optional store that as a null block; callers whose attribute is
// required substitute an empty one.
func FromPayload(p *conditiongql.Condition, diags *diag.Diagnostics) *Condition {
	if p == nil || len(p.Expressions) == 0 || len(p.ExpressionsStructure) == 0 {
		return nil
	}

	groups := comparisonGroupsFromPayload(p, diags)

	if len(groups) == 1 {
		return &Condition{AllOf: groups[0]}
	}

	anyOf := make([]AnyOfEntry, len(groups))
	for gi, g := range groups {
		if len(g) == 1 {
			anyOf[gi] = AnyOfEntry{
				Field:     g[0].Field,
				Operation: g[0].Operation,
				Value:     g[0].Value,
			}
			continue
		}
		anyOf[gi] = AnyOfEntry{AllOf: g}
	}
	return &Condition{AnyOf: anyOf}
}

// comparisonGroupsFromPayload reconstructs AND-groups of comparisons from the
// flat expressions plus expressions_structure. Outer order follows
// expressions_structure; inner (within-group) order follows each inner array.
// A structure_id referenced by a group with no matching expression indicates an
// inconsistency in the API response rather than a configuration error, so it is
// reported as a diagnostic rather than silently dropped.
func comparisonGroupsFromPayload(p *conditiongql.Condition, diags *diag.Diagnostics) [][]Comparison {
	byID := make(map[string]Comparison, len(p.Expressions))
	for _, e := range p.Expressions {
		byID[e.StructureId] = Comparison{
			Field:     types.StringValue(e.FieldAddress),
			Operation: types.StringValue(e.Operation),
			Value:     comparisonValue(e.Value),
		}
	}

	groups := make([][]Comparison, len(p.ExpressionsStructure))
	for gi, ids := range p.ExpressionsStructure {
		comparisons := make([]Comparison, 0, len(ids))
		for _, rawID := range ids {
			key := stringifyStructureElem(rawID)
			c, ok := byID[key]
			if !ok {
				diags.AddError(
					"condition API inconsistency",
					fmt.Sprintf("expressions_structure group %d references structure_id %q, which has no matching entry in expressions", gi, key),
				)
				continue
			}
			comparisons = append(comparisons, c)
		}
		groups[gi] = comparisons
	}
	return groups
}

// comparisonValue maps a read-back expression value to state. A blank value is
// normalized to null: operations that take no value, such as present and
// blank, come back as "" rather than null for conditions the UI created, and
// keeping that "" would diff forever against a configuration that omits value.
// The schema rejects a blank value on the way in, so nothing legitimate is lost.
func comparisonValue(v *string) types.String {
	if v == nil || strings.TrimSpace(*v) == "" {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// stringifyStructureElem normalizes an expressions_structure element (which the
// API returns untyped, as a number or a string) to the same string form used to
// key expressions by structure_id.
func stringifyStructureElem(v any) string {
	switch n := v.(type) {
	case string:
		return n
	case float64:
		return strconv.FormatInt(int64(n), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}
