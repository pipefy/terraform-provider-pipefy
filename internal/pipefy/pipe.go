// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"context"
	"fmt"
)

const pipeSelection = "id name public icon color " +
	"only_admin_can_remove_cards only_assignees_can_edit_cards " +
	"expiration_time_by_unit expiration_unit startFormPhaseId " +
	"preferences { inboxEmailEnabled mainTabViews }"

const createPipeMutation = "mutation CreatePipe_tf($name:String!,$orgId:ID!){ createPipe(input:{name:$name, organization_id:$orgId}){ pipe{ id name } } }"

const getPipeQuery = "query GetPipe_tf($id:ID!){ pipe(id:$id){ " + pipeSelection + " organization { id } } }"

// getPipePhasesQuery stays separate from getPipeQuery so a refresh does not pull
// an unbounded phase list. It runs once, during create, to remove the phases
// createPipe seeds.
const getPipePhasesQuery = "query GetPipePhases_tf($id:ID!){ pipe(id:$id){ " + pipeSelection + " phases { id } } }"

const updatePipeMutation = "mutation UpdatePipe_tf($id:ID!,$name:String,$public:Boolean,$icon:String,$color:Colors," +
	"$onlyAdminCanRemoveCards:Boolean,$onlyAssigneesCanEditCards:Boolean," +
	"$expirationTimeByUnit:Int,$expirationUnit:Int,$preferences:RepoPreferenceInput){ " +
	"updatePipe(input:{ id:$id, name:$name, public:$public, icon:$icon, color:$color, " +
	"only_admin_can_remove_cards:$onlyAdminCanRemoveCards, only_assignees_can_edit_cards:$onlyAssigneesCanEditCards, " +
	"expiration_time_by_unit:$expirationTimeByUnit, expiration_unit:$expirationUnit, preferences:$preferences }){ pipe{ " +
	pipeSelection + " } } }"

const deletePipeMutation = "mutation DeletePipe_tf($id:ID!){ deletePipe(input:{id:$id}){ success } }"

const getPipeUUIDQuery = "query GetPipeUuid_tf($id:ID!){ pipe(id:$id){ uuid } }"

// Preferences are the pipe-level card display settings.
type Preferences struct {
	InboxEmailEnabled *bool    `json:"inboxEmailEnabled"`
	MainTabViews      []string `json:"mainTabViews"`
}

// Pipe is a pipe as the API reports it. OrganizationID comes from the
// organization sub-selection rather than a scalar on the node.
type Pipe struct {
	ID                        string       `json:"id"`
	Name                      string       `json:"name"`
	Public                    *bool        `json:"public"`
	Icon                      *string      `json:"icon"`
	Color                     *string      `json:"color"`
	OnlyAdminCanRemoveCards   *bool        `json:"only_admin_can_remove_cards"`
	OnlyAssigneesCanEditCards *bool        `json:"only_assignees_can_edit_cards"`
	ExpirationTimeByUnit      *int64       `json:"expiration_time_by_unit"`
	ExpirationUnit            *int64       `json:"expiration_unit"`
	StartFormPhaseID          string       `json:"startFormPhaseId"`
	Preferences               *Preferences `json:"preferences"`

	OrganizationID string `json:"-"`
}

// SLA unit names, as the provider spells them.
const (
	UnitMinutes = "minutes"
	UnitHours   = "hours"
	UnitDays    = "days"
)

// UnitNames lists the SLA units in increasing order of size.
var UnitNames = []string{UnitMinutes, UnitHours, UnitDays}

// UnitNameToSeconds converts a unit name to the seconds the API stores.
func UnitNameToSeconds(name string) (int64, bool) {
	switch name {
	case UnitMinutes:
		return 60, true
	case UnitHours:
		return 3600, true
	case UnitDays:
		return 86400, true
	}
	return 0, false
}

// UnitSecondsToName converts the API's seconds back to a unit name.
func UnitSecondsToName(seconds int64) (string, bool) {
	switch seconds {
	case 60:
		return UnitMinutes, true
	case 3600:
		return UnitHours, true
	case 86400:
		return UnitDays, true
	}
	return "", false
}

// ValidDuration reports whether count fits unit without the API normalizing the
// pair to a coarser unit, which is what lets a configured SLA round-trip.
func ValidDuration(unit string, count int64) bool {
	if count < 1 {
		return false
	}
	switch unit {
	case UnitMinutes:
		return count <= 59
	case UnitHours:
		return count <= 23
	case UnitDays:
		return true
	}
	return false
}

// SLA reports the card SLA as a count and a unit name. ok is false when the
// pipe has no SLA, or when its unit is not one this provider models.
func (p Pipe) SLA() (count int64, unit string, ok bool) {
	if p.ExpirationUnit == nil || p.ExpirationTimeByUnit == nil {
		return 0, "", false
	}
	name, ok := UnitSecondsToName(*p.ExpirationUnit)
	if !ok {
		return 0, "", false
	}
	return *p.ExpirationTimeByUnit, name, true
}

// CreatePipeInput is the argument set of createPipe, which accepts nothing else.
type CreatePipeInput struct {
	Name           string
	OrganizationID string
}

// UpdatePipeInput is the argument set of updatePipe. A nil field is left out of
// the request, so an attribute the config omits keeps its server value.
type UpdatePipeInput struct {
	ID                        string
	Name                      *string
	Public                    *bool
	Icon                      *string
	Color                     *string
	OnlyAdminCanRemoveCards   *bool
	OnlyAssigneesCanEditCards *bool
	ExpirationTimeByUnit      *int64
	ExpirationUnit            *int64
	Preferences               map[string]any
}

func (in UpdatePipeInput) vars() map[string]any {
	vars := map[string]any{"id": in.ID}
	setIf(vars, "name", in.Name)
	setIf(vars, "public", in.Public)
	setIf(vars, "icon", in.Icon)
	setIf(vars, "color", in.Color)
	setIf(vars, "onlyAdminCanRemoveCards", in.OnlyAdminCanRemoveCards)
	setIf(vars, "onlyAssigneesCanEditCards", in.OnlyAssigneesCanEditCards)
	setIf(vars, "expirationTimeByUnit", in.ExpirationTimeByUnit)
	setIf(vars, "expirationUnit", in.ExpirationUnit)
	if in.Preferences != nil {
		vars["preferences"] = in.Preferences
	}
	return vars
}

// PipeService reads and writes pipes.
type PipeService struct{ c *Client }

// Create makes a pipe. createPipe takes only a name and an organization, so
// every other setting needs a follow-up Update.
func (s *PipeService) Create(ctx context.Context, in CreatePipeInput) (string, error) {
	var out struct {
		CreatePipe struct {
			Pipe struct {
				ID string `json:"id"`
			} `json:"pipe"`
		} `json:"createPipe"`
	}
	vars := map[string]any{"name": in.Name, "orgId": in.OrganizationID}
	if err := s.c.do(ctx, "CreatePipe_tf", createPipeMutation, vars, &out); err != nil {
		return "", err
	}
	return out.CreatePipe.Pipe.ID, nil
}

// Get returns a pipe and the id of the organization owning it.
func (s *PipeService) Get(ctx context.Context, id string) (Pipe, error) {
	var out struct {
		Pipe *struct {
			Pipe
			Organization *struct {
				ID string `json:"id"`
			} `json:"organization"`
		} `json:"pipe"`
	}
	if err := s.c.do(ctx, "GetPipe_tf", getPipeQuery, map[string]any{"id": id}, &out); err != nil {
		return Pipe{}, err
	}
	if out.Pipe == nil {
		return Pipe{}, ErrNotFound
	}
	pipe := out.Pipe.Pipe
	if out.Pipe.Organization != nil {
		pipe.OrganizationID = out.Pipe.Organization.ID
	}
	return pipe, nil
}

// GetWithPhaseIDs returns a pipe along with the ids of its phases. It exists for
// the create path, which removes the phases createPipe seeds.
func (s *PipeService) GetWithPhaseIDs(ctx context.Context, id string) (Pipe, []string, error) {
	var out struct {
		Pipe *struct {
			Pipe
			Phases []struct {
				ID string `json:"id"`
			} `json:"phases"`
		} `json:"pipe"`
	}
	if err := s.c.do(ctx, "GetPipePhases_tf", getPipePhasesQuery, map[string]any{"id": id}, &out); err != nil {
		return Pipe{}, nil, err
	}
	if out.Pipe == nil {
		return Pipe{}, nil, ErrNotFound
	}
	phaseIDs := make([]string, len(out.Pipe.Phases))
	for i, phase := range out.Pipe.Phases {
		phaseIDs[i] = phase.ID
	}
	return out.Pipe.Pipe, phaseIDs, nil
}

// Update applies pipe settings and returns the pipe as the API reports it back.
func (s *PipeService) Update(ctx context.Context, in UpdatePipeInput) (Pipe, error) {
	var out struct {
		UpdatePipe struct {
			Pipe Pipe `json:"pipe"`
		} `json:"updatePipe"`
	}
	if err := s.c.do(ctx, "UpdatePipe_tf", updatePipeMutation, in.vars(), &out); err != nil {
		return Pipe{}, err
	}
	return out.UpdatePipe.Pipe, nil
}

// Delete removes a pipe.
func (s *PipeService) Delete(ctx context.Context, id string) error {
	var out struct {
		DeletePipe struct {
			Success bool `json:"success"`
		} `json:"deletePipe"`
	}
	return s.c.do(ctx, "DeletePipe_tf", deletePipeMutation, map[string]any{"id": id}, &out)
}

// UUID resolves a pipe id to the pipe's UUID, which some mutations take instead
// of the id.
func (s *PipeService) UUID(ctx context.Context, pipeID string) (string, error) {
	var out struct {
		Pipe *struct {
			UUID string `json:"uuid"`
		} `json:"pipe"`
	}
	if err := s.c.do(ctx, "GetPipeUuid_tf", getPipeUUIDQuery, map[string]any{"id": pipeID}, &out); err != nil {
		return "", fmt.Errorf("resolve pipe %q UUID: %w", pipeID, err)
	}
	if out.Pipe == nil || out.Pipe.UUID == "" {
		return "", fmt.Errorf("resolve pipe %q UUID: expected a non-empty pipe.uuid", pipeID)
	}
	return out.Pipe.UUID, nil
}
