// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package labelgql holds the GraphQL field selection and the typed label
// payload shared across the pipefy_label resource's reads and writes.
package labelgql

const Selection = "id name color"

type Label struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func FindByID(labels []Label, id string) (Label, bool) {
	for _, l := range labels {
		if l.Id == id {
			return l, true
		}
	}
	return Label{}, false
}
