// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package conditionschema models the Pipefy condition grammar shared by every
// resource that carries one: pipefy_field_condition and pipefy_automation both
// write ConditionInput and read Condition. The schema attributes, the
// flattening into the wire's expressions plus expressions_structure, and the
// canonicalization back into all_of/any_of live here so the two resources
// cannot drift.
package conditionschema

// Selection is the condition sub-selection used by reads and by the
// create/update mutation payloads. The condition's own id, the per-expression
// DB id and related_cards are server-assigned and deliberately unselected.
const Selection = "expressions{ structure_id field_address operation value } expressions_structure"

// Payload is a condition as the API returns it.
type Payload struct {
	Expressions []Expression `json:"expressions"`
	// ExpressionsStructure groups expressions by structure_id into an
	// OR-of-ANDs: the outer list is ORed, each inner list is ANDed. The API
	// returns the inner elements untyped (numbers or strings), so callers
	// normalize each element to its string form.
	ExpressionsStructure [][]any `json:"expressions_structure"`
}

type Expression struct {
	StructureId  string  `json:"structure_id"`
	FieldAddress string  `json:"field_address"`
	Operation    string  `json:"operation"`
	Value        *string `json:"value"`
}
