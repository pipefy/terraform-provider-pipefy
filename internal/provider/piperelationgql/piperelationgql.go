// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package piperelationgql holds the GraphQL selection and mapping shared by the
// pipefy_pipe_relation resource's reads and writes, kept client-free so it stays
// testable without mocks.
package piperelationgql

const Selection = "id name " +
	"canCreateNewItems canConnectExistingItems canConnectMultipleItems " +
	"allChildrenMustBeDoneToFinishParent allChildrenMustBeDoneToMoveParent " +
	"childMustExistToFinishParent childMustExistToMoveParent autoFillFieldEnabled " +
	"parent { ... on Pipe { id } ... on Table { id } } " +
	"child { ... on Pipe { id } ... on Table { id } } " +
	"ownFieldMaps { fieldId inputMode value }"

type RepoRef struct {
	Id string `json:"id"`
}

type FieldMap struct {
	FieldId   string `json:"fieldId"`
	InputMode string `json:"inputMode"`
	Value     string `json:"value"`
}

type Relation struct {
	Id                                  string     `json:"id"`
	Name                                string     `json:"name"`
	CanCreateNewItems                   *bool      `json:"canCreateNewItems"`
	CanConnectExistingItems             *bool      `json:"canConnectExistingItems"`
	CanConnectMultipleItems             *bool      `json:"canConnectMultipleItems"`
	AllChildrenMustBeDoneToFinishParent *bool      `json:"allChildrenMustBeDoneToFinishParent"`
	AllChildrenMustBeDoneToMoveParent   *bool      `json:"allChildrenMustBeDoneToMoveParent"`
	ChildMustExistToFinishParent        *bool      `json:"childMustExistToFinishParent"`
	ChildMustExistToMoveParent          *bool      `json:"childMustExistToMoveParent"`
	AutoFillFieldEnabled                *bool      `json:"autoFillFieldEnabled"`
	Parent                              *RepoRef   `json:"parent"`
	Child                               *RepoRef   `json:"child"`
	OwnFieldMaps                        []FieldMap `json:"ownFieldMaps"`
}

func FindByID(relations []Relation, id string) (Relation, bool) {
	for _, r := range relations {
		if r.Id == id {
			return r, true
		}
	}
	return Relation{}, false
}
