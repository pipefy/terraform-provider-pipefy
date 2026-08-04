// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package conditionschema models the condition that pipefy_automation and
// pipefy_field_condition share: the schema attributes, and the mapping between
// them and the wire form.
package conditionschema

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
)

type Condition struct {
	AllOf []Comparison `tfsdk:"all_of"`
	AnyOf []AnyOfEntry `tfsdk:"any_of"`
}

type Comparison struct {
	Field     types.String `tfsdk:"field"`
	Operation types.String `tfsdk:"operation"`
	Value     types.String `tfsdk:"value"`
}

type AnyOfEntry struct {
	Field     types.String `tfsdk:"field"`
	Operation types.String `tfsdk:"operation"`
	Value     types.String `tfsdk:"value"`
	AllOf     []Comparison `tfsdk:"all_of"`
}

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

func EmptyInput() map[string]any {
	return map[string]any{
		"expressions":           []map[string]any{},
		"expressions_structure": [][]string{},
	}
}

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

func FromPayload(p *pipefy.Condition) (*Condition, error) {
	if p == nil || len(p.Expressions) == 0 || len(p.ExpressionsStructure) == 0 {
		return nil, nil
	}

	groups, err := comparisonGroupsFromPayload(p)
	if err != nil {
		return nil, err
	}

	if len(groups) == 1 {
		return &Condition{AllOf: groups[0]}, nil
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
	return &Condition{AnyOf: anyOf}, nil
}

func comparisonGroupsFromPayload(p *pipefy.Condition) ([][]Comparison, error) {
	byID := make(map[string]Comparison, len(p.Expressions))
	for _, e := range p.Expressions {
		byID[stringifyStructureElem(e.StructureID)] = Comparison{
			Field:     types.StringValue(e.FieldAddress),
			Operation: types.StringValue(e.Operation),
			Value:     comparisonValue(e.Value),
		}
	}

	groups := make([][]Comparison, len(p.ExpressionsStructure))
	for gi, ids := range p.ExpressionsStructure {
		if len(ids) == 0 {
			return nil, fmt.Errorf("expressions_structure group %d is empty, which no condition can express", gi)
		}
		comparisons := make([]Comparison, 0, len(ids))
		for _, rawID := range ids {
			key := stringifyStructureElem(rawID)
			c, ok := byID[key]
			if !ok {
				return nil, fmt.Errorf("expressions_structure group %d references structure_id %q, which has no matching entry in expressions", gi, key)
			}
			comparisons = append(comparisons, c)
		}
		groups[gi] = comparisons
	}
	return groups, nil
}

func comparisonValue(v *string) types.String {
	if v == nil || strings.TrimSpace(*v) == "" {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

func stringifyStructureElem(v any) string {
	switch n := v.(type) {
	case string:
		return n
	case float64:
		if n != math.Trunc(n) {
			return strconv.FormatFloat(n, 'f', -1, 64)
		}
		return strconv.FormatInt(int64(n), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}
