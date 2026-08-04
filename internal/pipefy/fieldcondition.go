// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"context"
	"errors"

	"github.com/pipefy/terraform-provider-pipefy/internal/locks"
)

// fieldConditionSelection references fields by their internal_id: expressions
// carry field_address and actions carry phaseField.internal_id, both of which
// round-trip against the internal_id sent on writes.
const fieldConditionSelection = "id name phase{ id } " +
	"condition{ " + conditionSelection + " } " +
	"actions{ actionId phaseField{ internal_id } whenEvaluator }"

const createFieldConditionMutation = "mutation CreateFieldCondition_tf($input:createFieldConditionInput!){ createFieldCondition(input:$input){ fieldCondition{ " + fieldConditionSelection + " } } }"

const getFieldConditionQuery = "query GetFieldCondition_tf($id:ID!){ fieldCondition(id:$id){ " + fieldConditionSelection + " } }"

const updateFieldConditionMutation = "mutation UpdateFieldCondition_tf($input:UpdateFieldConditionInput!){ updateFieldCondition(input:$input){ fieldCondition{ " + fieldConditionSelection + " } } }"

const deleteFieldConditionMutation = "mutation DeleteFieldCondition_tf($id:ID!){ deleteFieldCondition(input:{id:$id}){ success } }"

// FieldCondition is show/hide logic for a phase form.
type FieldCondition struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Phase     *FieldConditionPhase   `json:"phase"`
	Condition *Condition             `json:"condition"`
	Actions   []FieldConditionAction `json:"actions"`
}

// FieldConditionPhase is the phase the API reports as owning the condition.
type FieldConditionPhase struct {
	ID string `json:"id"`
}

// FieldConditionAction is one action the condition runs against a phase field.
// WhenEvaluator picks the branch: true for when the condition holds, false for
// when it does not.
type FieldConditionAction struct {
	ActionID      string                    `json:"actionId"`
	PhaseField    *FieldConditionPhaseField `json:"phaseField"`
	WhenEvaluator *bool                     `json:"whenEvaluator"`
}

// FieldConditionPhaseField is the field an action targets.
type FieldConditionPhaseField struct {
	InternalID string `json:"internal_id"`
}

// FieldConditionService reads and writes phase form conditions.
type FieldConditionService struct{ c *Client }

// lockPhaseRepo serializes on the repo owning phaseID. A nil phase and a zero
// repo id both render as errPhaseRepoIDUnresolved here, matching what the
// resource did.
func (s *FieldConditionService) lockPhaseRepo(ctx context.Context, phaseID string) (func(), error) {
	repoID, err := s.c.phaseRepoID(ctx, phaseID)
	if errors.Is(err, errPhaseUnresolved) {
		err = errPhaseRepoIDUnresolved
	}
	if err != nil {
		return nil, err
	}
	return locks.LockRepo(repoID), nil
}

// Create adds a condition to a phase form, serializing on the phase's repo the
// same way a phase field does. The input stays a map because the resource builds
// it from Terraform values.
//
// Note that createFieldCondition takes phaseId while updateFieldCondition takes
// phase_id. That inconsistency is the API's, and the caller spells each one.
func (s *FieldConditionService) Create(ctx context.Context, phaseID string, input map[string]any) (FieldCondition, error) {
	unlock, err := s.lockPhaseRepo(ctx, phaseID)
	if err != nil {
		return FieldCondition{}, err
	}
	defer unlock()

	var out struct {
		CreateFieldCondition struct {
			FieldCondition *FieldCondition `json:"fieldCondition"`
		} `json:"createFieldCondition"`
	}
	if err := s.c.do(ctx, "CreateFieldCondition_tf", createFieldConditionMutation, map[string]any{"input": input}, &out); err != nil {
		return FieldCondition{}, err
	}
	if out.CreateFieldCondition.FieldCondition == nil {
		return FieldCondition{}, ErrNotFound
	}
	return *out.CreateFieldCondition.FieldCondition, nil
}

// Get returns one field condition.
func (s *FieldConditionService) Get(ctx context.Context, id string) (FieldCondition, error) {
	var out struct {
		FieldCondition *FieldCondition `json:"fieldCondition"`
	}
	if err := s.c.do(ctx, "GetFieldCondition_tf", getFieldConditionQuery, map[string]any{"id": id}, &out); err != nil {
		return FieldCondition{}, err
	}
	if out.FieldCondition == nil {
		return FieldCondition{}, ErrNotFound
	}
	return *out.FieldCondition, nil
}

// Update replaces a condition's criteria and actions, serializing like Create.
// Unlike a phase field's Update, this one does take the lock.
func (s *FieldConditionService) Update(ctx context.Context, phaseID string, input map[string]any) (FieldCondition, error) {
	unlock, err := s.lockPhaseRepo(ctx, phaseID)
	if err != nil {
		return FieldCondition{}, err
	}
	defer unlock()

	var out struct {
		UpdateFieldCondition struct {
			FieldCondition *FieldCondition `json:"fieldCondition"`
		} `json:"updateFieldCondition"`
	}
	if err := s.c.do(ctx, "UpdateFieldCondition_tf", updateFieldConditionMutation, map[string]any{"input": input}, &out); err != nil {
		return FieldCondition{}, err
	}
	if out.UpdateFieldCondition.FieldCondition == nil {
		return FieldCondition{}, ErrNotFound
	}
	return *out.UpdateFieldCondition.FieldCondition, nil
}

// Delete removes a condition. An unresolvable phase is not an error here, unlike
// in Create and Update: a condition whose phase was deleted out of band has
// nothing left to lock and must still be removable rather than stuck in state. A
// failure of the lookup query itself still is an error.
func (s *FieldConditionService) Delete(ctx context.Context, phaseID, id string) error {
	repoID, err := s.c.phaseRepoID(ctx, phaseID)
	switch {
	case errors.Is(err, errPhaseUnresolved), errors.Is(err, errPhaseRepoIDUnresolved):
		// No repo to lock. Delete anyway.
	case err != nil:
		return err
	default:
		unlock := locks.LockRepo(repoID)
		defer unlock()
	}

	var out struct {
		DeleteFieldCondition struct {
			Success bool `json:"success"`
		} `json:"deleteFieldCondition"`
	}
	return s.c.do(ctx, "DeleteFieldCondition_tf", deleteFieldConditionMutation, map[string]any{"id": id}, &out)
}
