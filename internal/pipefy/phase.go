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

const phaseSelection = "id name done description index lateness_time can_receive_card_directly_from_draft repo_id"

const createPhaseMutation = "mutation CreatePhase_tf($pipeId:ID!,$name:String!,$done:Boolean,$description:String,$index:Float,$latenessTime:Int,$canReceiveCardDirectlyFromDraft:Boolean){ createPhase(input:{ pipe_id:$pipeId, name:$name, done:$done, description:$description, index:$index, lateness_time:$latenessTime, can_receive_card_directly_from_draft:$canReceiveCardDirectlyFromDraft }){ phase{ " + phaseSelection + " } } }"

const getPhaseQuery = "query GetPhase_tf($id:ID!){ phase(id:$id){ " + phaseSelection + " } }"

const updatePhaseMutation = "mutation UpdatePhase_tf($id:ID!,$name:String!,$done:Boolean,$description:String,$latenessTime:Int,$canReceiveCardDirectlyFromDraft:Boolean){ updatePhase(input:{ id:$id, name:$name, done:$done, description:$description, lateness_time:$latenessTime, can_receive_card_directly_from_draft:$canReceiveCardDirectlyFromDraft }){ phase{ " + phaseSelection + " } } }"

const deletePhaseMutation = "mutation DeletePhase_tf($id:ID!){ deletePhase(input:{id:$id}){ success } }"

const getPhaseRepoIDQuery = "query GetPhaseRepoId_tf($id:ID!){ phase(id:$id){ repo_id } }"

// errPhaseUnresolved and errPhaseRepoIDUnresolved are separate because callers
// render them differently: the field resource's Delete distinguishes them, its
// Create collapses both into the second, and the field condition's Delete treats
// either as "the phase is gone, so there is nothing to lock".
var (
	errPhaseUnresolved       = errors.New("could not resolve phase from phase query")
	errPhaseRepoIDUnresolved = errors.New("could not resolve valid phase repo_id from phase query")
)

// Phase is a phase of a pipe. RepoID is the id of the pipe owning it; there is
// no pipe field on the node.
type Phase struct {
	ID                              string   `json:"id"`
	Name                            string   `json:"name"`
	Done                            bool     `json:"done"`
	Description                     *string  `json:"description"`
	Index                           *float64 `json:"index"`
	LatenessTime                    *int64   `json:"lateness_time"`
	CanReceiveCardDirectlyFromDraft *bool    `json:"can_receive_card_directly_from_draft"`
	RepoID                          int64    `json:"repo_id"`
}

// PhaseWrites are the settings both createPhase and updatePhase accept. A nil
// field is left out of the request.
type PhaseWrites struct {
	Done                            *bool
	Description                     *string
	LatenessTime                    *int64
	CanReceiveCardDirectlyFromDraft *bool
}

func (w PhaseWrites) addTo(vars map[string]any) {
	setIf(vars, "done", w.Done)
	setIf(vars, "description", w.Description)
	setIf(vars, "latenessTime", w.LatenessTime)
	setIf(vars, "canReceiveCardDirectlyFromDraft", w.CanReceiveCardDirectlyFromDraft)
}

// CreatePhaseInput is the argument set of createPhase. Index is here and not on
// UpdatePhaseInput because the API accepts a position only at creation.
type CreatePhaseInput struct {
	PipeID string
	Name   string
	Index  *float64
	PhaseWrites
}

// UpdatePhaseInput is the argument set of updatePhase.
type UpdatePhaseInput struct {
	ID   string
	Name string
	PhaseWrites
}

// PhaseService reads and writes phases.
type PhaseService struct{ c *Client }

// Create adds a phase to a pipe. The API rejects concurrent phase creates for
// the same pipe, so this serializes on the pipe id.
func (s *PhaseService) Create(ctx context.Context, in CreatePhaseInput) (Phase, error) {
	unlock := locks.LockRepo(in.PipeID)
	defer unlock()

	vars := map[string]any{"pipeId": in.PipeID, "name": in.Name}
	in.PhaseWrites.addTo(vars)
	setIf(vars, "index", in.Index)

	var out struct {
		CreatePhase struct {
			Phase Phase `json:"phase"`
		} `json:"createPhase"`
	}
	if err := s.c.do(ctx, "CreatePhase_tf", createPhaseMutation, vars, &out); err != nil {
		return Phase{}, err
	}
	return out.CreatePhase.Phase, nil
}

// Get returns one phase.
func (s *PhaseService) Get(ctx context.Context, id string) (Phase, error) {
	var out struct {
		Phase *Phase `json:"phase"`
	}
	if err := s.c.do(ctx, "GetPhase_tf", getPhaseQuery, map[string]any{"id": id}, &out); err != nil {
		return Phase{}, err
	}
	if out.Phase == nil {
		return Phase{}, ErrNotFound
	}
	return *out.Phase, nil
}

// Update renames a phase and applies its settings.
func (s *PhaseService) Update(ctx context.Context, in UpdatePhaseInput) (Phase, error) {
	vars := map[string]any{"id": in.ID, "name": in.Name}
	in.PhaseWrites.addTo(vars)

	var out struct {
		UpdatePhase struct {
			Phase Phase `json:"phase"`
		} `json:"updatePhase"`
	}
	if err := s.c.do(ctx, "UpdatePhase_tf", updatePhaseMutation, vars, &out); err != nil {
		return Phase{}, err
	}
	return out.UpdatePhase.Phase, nil
}

// Delete removes a phase and reports the mutation's success flag. The flag is
// returned rather than turned into an error because the two callers disagree
// about it: the phase resource's Delete ignores it, while the pipe resource's
// cleanup of the phases createPipe seeds treats a false as a failure.
//
// It takes no lock. Only phase creates serialize.
func (s *PhaseService) Delete(ctx context.Context, id string) (bool, error) {
	var out struct {
		DeletePhase struct {
			Success bool `json:"success"`
		} `json:"deletePhase"`
	}
	if err := s.c.do(ctx, "DeletePhase_tf", deletePhaseMutation, map[string]any{"id": id}, &out); err != nil {
		return false, err
	}
	return out.DeletePhase.Success, nil
}

// phaseRepoID resolves the id of the repo owning a phase, which is what the
// per-repo lock is keyed on. Its three outcomes are distinct on purpose; see
// errPhaseUnresolved.
func (c *Client) phaseRepoID(ctx context.Context, phaseID string) (string, error) {
	var out struct {
		Phase *struct {
			RepoID int64 `json:"repo_id"`
		} `json:"phase"`
	}
	if err := c.do(ctx, "GetPhaseRepoId_tf", getPhaseRepoIDQuery, map[string]any{"id": phaseID}, &out); err != nil {
		return "", fmt.Errorf("failed to fetch phase repo_id: %w", err)
	}
	if out.Phase == nil {
		return "", errPhaseUnresolved
	}
	if out.Phase.RepoID == 0 {
		return "", errPhaseRepoIDUnresolved
	}
	return strconv.FormatInt(out.Phase.RepoID, 10), nil
}
