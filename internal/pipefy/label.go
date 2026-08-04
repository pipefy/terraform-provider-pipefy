// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import "context"

const labelSelection = "id name color"

const createLabelMutation = "mutation CreateLabel_tf($pipeId:ID!,$name:String!,$color:String!){ createLabel(input:{ pipe_id:$pipeId, name:$name, color:$color }){ label{ " + labelSelection + " } } }"
const getPipeLabelsQuery = "query GetPipeLabels_tf($pipeId:ID!){ pipe(id:$pipeId){ labels{ " + labelSelection + " } } }"
const updateLabelMutation = "mutation UpdateLabel_tf($id:ID!,$name:String!,$color:String!){ updateLabel(input:{ id:$id, name:$name, color:$color }){ label{ " + labelSelection + " } } }"
const deleteLabelMutation = "mutation DeleteLabel_tf($id:ID!){ deleteLabel(input:{ id:$id }){ success } }"

// Label is a pipe label.
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// CreateLabelInput is the argument set of createLabel.
type CreateLabelInput struct {
	PipeID string
	Name   string
	Color  string
}

// UpdateLabelInput is the argument set of updateLabel.
type UpdateLabelInput struct {
	ID    string
	Name  string
	Color string
}

// LabelService reads and writes pipe labels.
type LabelService struct{ c *Client }

// Create adds a label to a pipe.
func (s *LabelService) Create(ctx context.Context, in CreateLabelInput) (Label, error) {
	var out struct {
		CreateLabel struct {
			Label Label `json:"label"`
		} `json:"createLabel"`
	}
	vars := map[string]any{"pipeId": in.PipeID, "name": in.Name, "color": in.Color}
	if err := s.c.do(ctx, "CreateLabel_tf", createLabelMutation, vars, &out); err != nil {
		return Label{}, err
	}
	return out.CreateLabel.Label, nil
}

// Get returns one label of a pipe. There is no query for a label on its own, so
// this reads the pipe's labels and picks labelID out of them.
func (s *LabelService) Get(ctx context.Context, pipeID, labelID string) (Label, error) {
	var out struct {
		Pipe *struct {
			Labels []Label `json:"labels"`
		} `json:"pipe"`
	}
	vars := map[string]any{"pipeId": pipeID}
	if err := s.c.do(ctx, "GetPipeLabels_tf", getPipeLabelsQuery, vars, &out); err != nil {
		return Label{}, err
	}
	if out.Pipe == nil {
		return Label{}, ErrNotFound
	}
	for _, label := range out.Pipe.Labels {
		if label.ID == labelID {
			return label, nil
		}
	}
	return Label{}, ErrNotFound
}

// Update renames or recolors a label.
func (s *LabelService) Update(ctx context.Context, in UpdateLabelInput) (Label, error) {
	var out struct {
		UpdateLabel struct {
			Label Label `json:"label"`
		} `json:"updateLabel"`
	}
	vars := map[string]any{"id": in.ID, "name": in.Name, "color": in.Color}
	if err := s.c.do(ctx, "UpdateLabel_tf", updateLabelMutation, vars, &out); err != nil {
		return Label{}, err
	}
	return out.UpdateLabel.Label, nil
}

// Delete removes a label.
func (s *LabelService) Delete(ctx context.Context, id string) error {
	var out struct {
		DeleteLabel struct {
			Success bool `json:"success"`
		} `json:"deleteLabel"`
	}
	return s.c.do(ctx, "DeleteLabel_tf", deleteLabelMutation, map[string]any{"id": id}, &out)
}
