// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"context"
	"errors"

	"github.com/pipefy/terraform-provider-pipefy/internal/locks"
)

const fieldSelection = "id internal_id uuid label type required options " +
	"description help editable minimal_view custom_validation index"

const createPhaseFieldMutation = "mutation CreatePhaseField_tf($phaseId:ID!,$type:ID!,$label:String!,$required:Boolean,$options:[String],$description:String,$help:String,$editable:Boolean,$minimalView:Boolean,$customValidation:String,$index:Float){ createPhaseField(input:{ phase_id:$phaseId, type:$type, label:$label, required:$required, options:$options, description:$description, help:$help, editable:$editable, minimal_view:$minimalView, custom_validation:$customValidation, index:$index }){ phase_field{ " + fieldSelection + " } } }"

const getPhaseFieldsQuery = "query GetPhaseFields_tf($phaseId:ID!){ phase(id:$phaseId){ fields{ " + fieldSelection + " } } }"

const updatePhaseFieldMutation = "mutation UpdatePhaseField_tf($id:ID!,$uuid:ID!,$label:String!,$required:Boolean,$options:[String],$description:String,$help:String,$editable:Boolean,$minimalView:Boolean,$customValidation:String,$index:Float){ updatePhaseField(input:{ id:$id, uuid:$uuid, label:$label, required:$required, options:$options, description:$description, help:$help, editable:$editable, minimal_view:$minimalView, custom_validation:$customValidation, index:$index }){ phase_field{ " + fieldSelection + " } } }"

const deletePhaseFieldMutation = "mutation DeletePhaseField_tf($id:ID!,$pipeUuid:ID!){ deletePhaseField(input:{ id:$id, pipeUuid:$pipeUuid }){ success } }"

// Field is a phase field. Index is a Float on the wire, not an Int: fractional
// and negative positions are both real.
type Field struct {
	ID               string   `json:"id"`
	InternalID       string   `json:"internal_id"`
	UUID             string   `json:"uuid"`
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

// FieldWrites are the settings both phase-field mutations accept. A nil field is
// left out of the request.
type FieldWrites struct {
	Required         *bool
	Options          *[]string
	Description      *string
	Help             *string
	Editable         *bool
	MinimalView      *bool
	CustomValidation *string
	Index            *float64
}

func (w FieldWrites) addTo(vars map[string]any) {
	setIf(vars, "required", w.Required)
	setIf(vars, "options", w.Options)
	setIf(vars, "description", w.Description)
	setIf(vars, "help", w.Help)
	setIf(vars, "editable", w.Editable)
	setIf(vars, "minimalView", w.MinimalView)
	setIf(vars, "customValidation", w.CustomValidation)
	setIf(vars, "index", w.Index)
}

// CreateFieldInput is the argument set of createPhaseField.
type CreateFieldInput struct {
	PhaseID string
	Type    string
	Label   string
	FieldWrites
}

// UpdateFieldInput is the argument set of updatePhaseField. Label is a pointer
// because the resource only sends it when the plan carries one, even though the
// document declares it non-null.
type UpdateFieldInput struct {
	ID    string
	UUID  string
	Label *string
	FieldWrites
}

// FieldService reads and writes phase fields.
type FieldService struct{ c *Client }

// Create adds a field to a phase. The API rejects concurrent field creates for
// the same repo, so this resolves the phase's repo and serializes on it.
func (s *FieldService) Create(ctx context.Context, in CreateFieldInput) (Field, error) {
	repoID, err := s.c.phaseRepoID(ctx, in.PhaseID)
	// A nil phase and a zero repo id render as one message here, while Delete
	// keeps them apart. Preserved from the resource this replaces.
	if errors.Is(err, errPhaseUnresolved) {
		err = errPhaseRepoIDUnresolved
	}
	if err != nil {
		return Field{}, err
	}
	unlock := locks.LockRepo(repoID)
	defer unlock()

	vars := map[string]any{"phaseId": in.PhaseID, "type": in.Type, "label": in.Label}
	in.FieldWrites.addTo(vars)

	var out struct {
		CreatePhaseField struct {
			PhaseField Field `json:"phase_field"`
		} `json:"createPhaseField"`
	}
	if err := s.c.do(ctx, "CreatePhaseField_tf", createPhaseFieldMutation, vars, &out); err != nil {
		return Field{}, err
	}
	return out.CreatePhaseField.PhaseField, nil
}

// GetByUUID returns one field of a phase. There is no query for a field on its
// own, and the UUID is the stable key: unlike the id, it survives a relabel.
func (s *FieldService) GetByUUID(ctx context.Context, phaseID, uuid string) (Field, error) {
	var out struct {
		Phase *struct {
			Fields []Field `json:"fields"`
		} `json:"phase"`
	}
	if err := s.c.do(ctx, "GetPhaseFields_tf", getPhaseFieldsQuery, map[string]any{"phaseId": phaseID}, &out); err != nil {
		return Field{}, err
	}
	if out.Phase == nil {
		return Field{}, ErrNotFound
	}
	for _, field := range out.Phase.Fields {
		if field.UUID == uuid {
			return field, nil
		}
	}
	return Field{}, ErrNotFound
}

// Update changes a field's label and settings. It takes no lock, matching the
// resource it replaces.
func (s *FieldService) Update(ctx context.Context, in UpdateFieldInput) (Field, error) {
	vars := map[string]any{"id": in.ID, "uuid": in.UUID}
	setIf(vars, "label", in.Label)
	in.FieldWrites.addTo(vars)

	var out struct {
		UpdatePhaseField struct {
			PhaseField Field `json:"phase_field"`
		} `json:"updatePhaseField"`
	}
	if err := s.c.do(ctx, "UpdatePhaseField_tf", updatePhaseFieldMutation, vars, &out); err != nil {
		return Field{}, err
	}
	return out.UpdatePhaseField.PhaseField, nil
}

// Delete removes a field. deletePhaseField wants the owning pipe's UUID, which
// takes two lookups to reach: the phase gives up its repo id, and the repo id is
// the pipe id. It takes no lock, unlike Create; see the follow-up issue asking
// whether the API needs one here too.
func (s *FieldService) Delete(ctx context.Context, phaseID, fieldID string) error {
	repoID, err := s.c.phaseRepoID(ctx, phaseID)
	if err != nil {
		return err
	}
	pipeUUID, err := s.c.Pipes.UUID(ctx, repoID)
	if err != nil {
		return err
	}

	var out struct {
		DeletePhaseField struct {
			Success bool `json:"success"`
		} `json:"deletePhaseField"`
	}
	vars := map[string]any{"id": fieldID, "pipeUuid": pipeUUID}
	return s.c.do(ctx, "DeletePhaseField_tf", deletePhaseFieldMutation, vars, &out)
}
