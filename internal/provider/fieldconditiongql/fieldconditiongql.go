// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package fieldconditiongql holds the GraphQL selection and the typed field
// condition payload shared across the pipefy_field_condition resource's reads
// and writes. The condition itself comes from conditionschema, which every
// resource carrying a condition shares.
package fieldconditiongql

import "github.com/pipefy/terraform-provider-pipefy/internal/provider/conditionschema"

// Selection is the field condition sub-selection used by reads and by the
// create/update mutation payloads. Fields are referenced by their internal_id:
// expressions carry field_address and actions carry phaseField.internal_id, both
// of which round-trip against the internal_id sent on writes.
const Selection = "id name phase{ id } " +
	"condition{ " + conditionschema.Selection + " } " +
	"actions{ actionId phaseField{ internal_id } whenEvaluator }"

type FieldCondition struct {
	Id        string                   `json:"id"`
	Name      string                   `json:"name"`
	Phase     *Phase                   `json:"phase"`
	Condition *conditionschema.Payload `json:"condition"`
	Actions   []Action                 `json:"actions"`
}

type Phase struct {
	Id string `json:"id"`
}

type Action struct {
	ActionId      string      `json:"actionId"`
	PhaseField    *PhaseField `json:"phaseField"`
	WhenEvaluator *bool       `json:"whenEvaluator"`
}

type PhaseField struct {
	InternalId string `json:"internal_id"`
}
