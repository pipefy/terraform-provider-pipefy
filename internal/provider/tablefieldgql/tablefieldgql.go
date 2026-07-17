// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package tablefieldgql holds the GraphQL field selection and the typed
// table-field payload shared across the pipefy_table_field resource's reads
// and writes.
package tablefieldgql

const Selection = "id internal_id uuid label type required options " +
	"description help minimal_view custom_validation unique"

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
	MinimalView      *bool    `json:"minimal_view"`
	CustomValidation *string  `json:"custom_validation"`
	Unique           *bool    `json:"unique"`
}

func FindByUUID(fields []Field, uuid string) (Field, bool) {
	for _, f := range fields {
		if f.Uuid == uuid {
			return f, true
		}
	}
	return Field{}, false
}
