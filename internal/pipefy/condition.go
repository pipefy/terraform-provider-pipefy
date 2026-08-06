// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

// conditionSelection leaves out the condition's own id, each expression's DB id
// and related_cards: all server-assigned, none of them modelled.
const conditionSelection = "expressions{ structure_id field_address operation value } expressions_structure"

// Condition is the criteria shared by automations and field conditions.
type Condition struct {
	Expressions []ConditionExpression `json:"expressions"`
	// ExpressionsStructure references expressions by structure_id: the outer
	// list is ORed, each inner list ANDed.
	ExpressionsStructure [][]any `json:"expressions_structure"`
}

// ConditionExpression is one comparison inside a Condition.
type ConditionExpression struct {
	// StructureID is untyped for the same reason the elements of
	// ExpressionsStructure are: both sides of that reference come back as a
	// number or a string depending on how the condition was written, and a
	// concrete Go type would fail the whole read on the other one.
	StructureID  any     `json:"structure_id"`
	FieldAddress string  `json:"field_address"`
	Operation    string  `json:"operation"`
	Value        *string `json:"value"`
}
