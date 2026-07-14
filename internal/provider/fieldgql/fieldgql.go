// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package fieldgql holds the GraphQL field selection and the typed phase-field
// payload shared across the pipefy_field resource's reads and writes.
package fieldgql

const Selection = "id internal_id uuid label type required options " +
	"description help editable minimal_view custom_validation index"

type Field struct {
	Id               string   `json:"id"`
	InternalId       string   `json:"internal_id"`
	Uuid             string   `json:"uuid"`
	Label            string   `json:"label"`
	Type             string   `json:"type"`
	Required         *bool    `json:"required"`
	Options          []string `json:"options"`
	Description      *string  `json:"description"`
	Help             *string  `json:"help"`
	Editable         *bool    `json:"editable"`
	MinimalView      *bool    `json:"minimal_view"`
	CustomValidation *string  `json:"custom_validation"`
	Index            *float64 `json:"index"`
}

func FindByUUID(fields []Field, uuid string) (Field, bool) {
	for _, f := range fields {
		if f.Uuid == uuid {
			return f, true
		}
	}
	return Field{}, false
}
