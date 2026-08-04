// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import "context"

// pipeRelationSelection resolves parent and child through inline fragments
// because either end can be a pipe or a table.
const pipeRelationSelection = "id name " +
	"canCreateNewItems canConnectExistingItems canConnectMultipleItems " +
	"allChildrenMustBeDoneToFinishParent allChildrenMustBeDoneToMoveParent " +
	"childMustExistToFinishParent childMustExistToMoveParent autoFillFieldEnabled " +
	"parent { ... on Pipe { id } ... on Table { id } } " +
	"child { ... on Pipe { id } ... on Table { id } } " +
	"ownFieldMaps { fieldId inputMode value }"

const createPipeRelationMutation = "mutation CreatePipeRelation_tf($input:CreatePipeRelationInput!){ createPipeRelation(input:$input){ pipeRelation{ id } } }"

const getPipeRelationsQuery = "query GetPipeRelations_tf($pipeId:ID!){ pipe(id:$pipeId){ childrenRelations{ " + pipeRelationSelection + " } } }"

const updatePipeRelationMutation = "mutation UpdatePipeRelation_tf($input:UpdatePipeRelationInput!){ updatePipeRelation(input:$input){ pipeRelation{ id } } }"

const deletePipeRelationMutation = "mutation DeletePipeRelation_tf($id:ID!){ deletePipeRelation(input:{ id:$id }){ success } }"

// RepoRef is either end of a relation, a pipe or a table.
type RepoRef struct {
	ID string `json:"id"`
}

// FieldMap is one entry of a relation's field mapping.
type FieldMap struct {
	FieldID   string `json:"fieldId"`
	InputMode string `json:"inputMode"`
	Value     string `json:"value"`
}

// PipeRelation is a connection between two repos.
type PipeRelation struct {
	ID                                  string     `json:"id"`
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

// PipeRelationService reads and writes connections between repos.
type PipeRelationService struct{ c *Client }

// Create makes a relation and returns its id. The mutation selects only the id.
// The input stays a map because typing it is a redesign with real judgment about
// which keys are optional, and that belongs in its own change.
//
// There is no lock: the API accepts concurrent creates for one repo.
func (s *PipeRelationService) Create(ctx context.Context, input map[string]any) (string, error) {
	var out struct {
		CreatePipeRelation struct {
			PipeRelation struct {
				ID string `json:"id"`
			} `json:"pipeRelation"`
		} `json:"createPipeRelation"`
	}
	if err := s.c.do(ctx, "CreatePipeRelation_tf", createPipeRelationMutation, map[string]any{"input": input}, &out); err != nil {
		return "", err
	}
	return out.CreatePipeRelation.PipeRelation.ID, nil
}

// Get returns one relation of a pipe, read through the parent's
// childrenRelations because there is no query for a relation on its own.
func (s *PipeRelationService) Get(ctx context.Context, pipeID, relationID string) (PipeRelation, error) {
	var out struct {
		Pipe *struct {
			ChildrenRelations []PipeRelation `json:"childrenRelations"`
		} `json:"pipe"`
	}
	if err := s.c.do(ctx, "GetPipeRelations_tf", getPipeRelationsQuery, map[string]any{"pipeId": pipeID}, &out); err != nil {
		return PipeRelation{}, err
	}
	if out.Pipe == nil {
		return PipeRelation{}, ErrNotFound
	}
	for _, relation := range out.Pipe.ChildrenRelations {
		if relation.ID == relationID {
			return relation, nil
		}
	}
	return PipeRelation{}, ErrNotFound
}

// Update applies relation settings. It returns no relation because the mutation
// selects only the id. updatePipeRelation replaces ownFieldMaps wholesale, so the
// caller sends the full list every time.
func (s *PipeRelationService) Update(ctx context.Context, input map[string]any) error {
	var out struct {
		UpdatePipeRelation struct {
			PipeRelation struct {
				ID string `json:"id"`
			} `json:"pipeRelation"`
		} `json:"updatePipeRelation"`
	}
	return s.c.do(ctx, "UpdatePipeRelation_tf", updatePipeRelationMutation, map[string]any{"input": input}, &out)
}

// Delete removes a relation.
func (s *PipeRelationService) Delete(ctx context.Context, id string) error {
	var out struct {
		DeletePipeRelation struct {
			Success bool `json:"success"`
		} `json:"deletePipeRelation"`
	}
	return s.c.do(ctx, "DeletePipeRelation_tf", deletePipeRelationMutation, map[string]any{"id": id}, &out)
}
