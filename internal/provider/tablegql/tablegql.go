// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package tablegql holds the GraphQL field selection and the pure mapping shared
// by the pipefy_table resource's reads and writes.
package tablegql

const Selection = "id name description authorization color icon"

type Payload struct {
	Id            string  `json:"id"`
	Name          string  `json:"name"`
	Description   *string `json:"description"`
	Authorization *string `json:"authorization"`
	Color         *string `json:"color"`
	Icon          *string `json:"icon"`
}

const (
	AuthorizationRead  = "read"
	AuthorizationWrite = "write"
)

var AuthorizationValues = []string{AuthorizationRead, AuthorizationWrite}