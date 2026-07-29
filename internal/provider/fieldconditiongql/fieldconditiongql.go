// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package fieldconditiongql holds the GraphQL selection and the typed field
// condition payload shared across the pipefy_field_condition resource's reads
// and writes.
package fieldconditiongql

// Selection is the field condition sub-selection used by reads and by the
// create/update mutation payloads. Fields are referenced by their internal_id:
// expressions carry field_address and actions carry phaseField.internal_id, both
// of which round-trip against the internal_id sent on writes.
const Selection = "id name phase{ id } " +
	"condition{ expressions{ structure_id field_address operation value } expressions_structure } " +
	"actions{ actionId phaseField{ internal_id } whenEvaluator }"

type FieldCondition struct {
	Id        string     `json:"id"`
	Name      string     `json:"name"`
	Phase     *Phase     `json:"phase"`
	Condition *Condition `json:"condition"`
	Actions   []Action   `json:"actions"`
}

type Phase struct {
	Id string `json:"id"`
}

type Condition struct {
	Expressions []Expression `json:"expressions"`
	// ExpressionsStructure is an array of arrays grouping expressions by their
	// structure_id into AND/OR sets. The API returns the inner elements untyped
	// (numbers or strings), so callers normalize each element to its string form.
	ExpressionsStructure [][]any `json:"expressions_structure"`
}

type Expression struct {
	StructureId  string  `json:"structure_id"`
	FieldAddress string  `json:"field_address"`
	Operation    string  `json:"operation"`
	Value        *string `json:"value"`
}

type Action struct {
	ActionId      string      `json:"actionId"`
	PhaseField    *PhaseField `json:"phaseField"`
	WhenEvaluator *bool       `json:"whenEvaluator"`
}

type PhaseField struct {
	InternalId string `json:"internal_id"`
}
