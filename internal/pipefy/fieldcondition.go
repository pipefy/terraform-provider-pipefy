// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/pipefy/terraform-provider-pipefy/internal/locks"
)

// fieldConditionSelection references fields by their internal_id: expressions
// carry field_address and actions carry phaseField.internal_id, both of which
// round-trip against the internal_id sent on writes. phase.repo_id is the pipe
// that owns the condition; FieldCondition has no pipe field of its own.
const fieldConditionSelection = "id name phase{ id repo_id } " +
	"condition{ " + conditionSelection + " } " +
	"actions{ actionId phaseField{ internal_id } whenEvaluator }"

const createFieldConditionMutation = "mutation CreateFieldCondition_tf($input:createFieldConditionInput!){ createFieldCondition(input:$input){ fieldCondition{ " + fieldConditionSelection + " } } }"

const getFieldConditionQuery = "query GetFieldCondition_tf($id:ID!){ fieldCondition(id:$id){ " + fieldConditionSelection + " } }"

const updateFieldConditionMutation = "mutation UpdateFieldCondition_tf($input:UpdateFieldConditionInput!){ updateFieldCondition(input:$input){ fieldCondition{ " + fieldConditionSelection + " } } }"

const deleteFieldConditionMutation = "mutation DeleteFieldCondition_tf($id:ID!){ deleteFieldCondition(input:{id:$id}){ success } }"

// ErrNoFieldCondition reports a mutation that returned no field condition. It is
// distinct from ErrNotFound because a caller maps that one to removing the
// resource from state, which is the wrong answer for a write that failed.
var ErrNoFieldCondition = errors.New("the API returned no field condition")

// FieldCondition is show/hide logic for fields in a pipe.
type FieldCondition struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Phase     *FieldConditionPhase   `json:"phase"`
	Condition *Condition             `json:"condition"`
	Actions   []FieldConditionAction `json:"actions"`
}

// FieldConditionPhase is the phase the API lists the condition under.
// RepoID is the id of the owning pipe.
type FieldConditionPhase struct {
	ID     string `json:"id"`
	RepoID int64  `json:"repo_id"`
}

// PipeID is the owning pipe, or empty when the API omitted a usable repo_id.
func (p *FieldConditionPhase) PipeID() string {
	if p == nil || p.RepoID == 0 {
		return ""
	}
	return strconv.FormatInt(p.RepoID, 10)
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

// FieldConditionService reads and writes field conditions.
type FieldConditionService struct{ c *Client }

func lockFieldConditionPipe(pipeID string) (func(), error) {
	if pipeID == "" {
		return nil, fmt.Errorf("empty pipe_id, expected a Pipefy pipe ID")
	}
	return locks.LockRepo(pipeID), nil
}

// Create adds a condition, serializing on the pipe. The input stays a map
// because the resource builds it from Terraform values.
//
// Note that createFieldCondition takes phaseId while updateFieldCondition takes
// phase_id. That inconsistency is the API's, and the caller spells each one.
func (s *FieldConditionService) Create(ctx context.Context, pipeID string, input map[string]any) (FieldCondition, error) {
	unlock, err := lockFieldConditionPipe(pipeID)
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
		return FieldCondition{}, ErrNoFieldCondition
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
func (s *FieldConditionService) Update(ctx context.Context, pipeID string, input map[string]any) (FieldCondition, error) {
	unlock, err := lockFieldConditionPipe(pipeID)
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
		return FieldCondition{}, ErrNoFieldCondition
	}
	return *out.UpdateFieldCondition.FieldCondition, nil
}

// Delete removes a condition. An empty pipeID skips the lock so a condition
// whose pipe is gone from state can still be deleted rather than stuck.
func (s *FieldConditionService) Delete(ctx context.Context, pipeID, id string) error {
	if pipeID != "" {
		unlock := locks.LockRepo(pipeID)
		defer unlock()
	}

	var out struct {
		DeleteFieldCondition struct {
			Success bool `json:"success"`
		} `json:"deleteFieldCondition"`
	}
	return s.c.do(ctx, "DeleteFieldCondition_tf", deleteFieldConditionMutation, map[string]any{"id": id}, &out)
}
