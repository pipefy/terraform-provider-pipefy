// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package conditiongql holds the GraphQL selection and payload for the
// condition that pipefy_automation and pipefy_field_condition share.
package conditiongql

// Selection leaves out the condition's own id, each expression's DB id and
// related_cards: all server-assigned, none of them modelled.
const Selection = "expressions{ structure_id field_address operation value } expressions_structure"

type Condition struct {
	Expressions []Expression `json:"expressions"`
	// ExpressionsStructure references expressions by structure_id: the outer
	// list is ORed, each inner list ANDed. The API returns the inner elements
	// untyped, as numbers or strings, hence any.
	ExpressionsStructure [][]any `json:"expressions_structure"`
}

type Expression struct {
	StructureId  string  `json:"structure_id"`
	FieldAddress string  `json:"field_address"`
	Operation    string  `json:"operation"`
	Value        *string `json:"value"`
}
