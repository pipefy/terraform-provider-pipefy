// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package fieldgql holds the GraphQL field selection and the typed phase-field
// payload shared across the pipefy_field resource's reads and writes.
package fieldgql

const Selection = "id internal_id uuid label options"

type Field struct {
	Id         string   `json:"id"`
	InternalId string   `json:"internal_id"`
	Uuid       string   `json:"uuid"`
	Label      string   `json:"label"`
	Options    []string `json:"options"`
}

func FindByUUID(fields []Field, uuid string) (Field, bool) {
	for _, f := range fields {
		if f.Uuid == uuid {
			return f, true
		}
	}
	return Field{}, false
}
