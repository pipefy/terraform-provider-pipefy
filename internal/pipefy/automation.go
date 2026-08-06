// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"context"
	"encoding/json"
	"errors"
)

// automationSelection is the field set read back for an automation, so Read
// detects out-of-band changes.
const automationSelection = "id name active event_id action_id " +
	"event_repo{ id } action_repo_v2{ ... on Pipe{ id } ... on Table{ id } } " +
	"scheduler_frequency schedulerCron{ minute hour dayOfMonth month dayOfWeek } " +
	"searchFor{ field id operation value } responseSchema " +
	"condition{ " + conditionSelection + " }"

const createAutomationMutation = "mutation CreateAutomation_tf($input:CreateAutomationInput!){ createAutomation(input:$input){ automation{ id name action_id event_id active } error_details{ object_name object_key messages } } }"

const getAutomationQuery = "query GetAutomation_tf($id:ID!){ automation(id:$id){ " + automationSelection + " } }"

const updateAutomationMutation = "mutation UpdateAutomation_tf($input:UpdateAutomationInput!){ updateAutomation(input:$input){ automation{ id } error_details{ object_name object_key messages } } }"

const deleteAutomationMutation = "mutation DeleteAutomation_tf($id:ID!){ deleteAutomation(input:{id:$id}){ success } }"

// ErrNoAutomation reports a mutation that returned neither an automation nor any
// error_details, which leaves nothing to explain the failure with.
var ErrNoAutomation = errors.New("the API returned no automation and no error_details")

// AutomationRepoRef is the pipe or table on either side of an automation.
type AutomationRepoRef struct {
	ID string `json:"id"`
}

// AutomationCron is a scheduler automation's cron expression, field by field.
type AutomationCron struct {
	Minute     *string `json:"minute"`
	Hour       *string `json:"hour"`
	DayOfMonth *string `json:"dayOfMonth"`
	Month      *string `json:"month"`
	DayOfWeek  *string `json:"dayOfWeek"`
}

// AutomationSearchCondition is one entry of a scheduler automation's searchFor
// list. The id is client-supplied and the list is ordered.
type AutomationSearchCondition struct {
	Field     string  `json:"field"`
	ID        string  `json:"id"`
	Operation string  `json:"operation"`
	Value     *string `json:"value"`
}

// Automation is an automation as the read reports it. ResponseSchema stays raw
// because the caller compares it semantically, which decoding would break.
type Automation struct {
	ID                 string                      `json:"id"`
	Name               string                      `json:"name"`
	Active             *bool                       `json:"active"`
	EventID            string                      `json:"event_id"`
	ActionID           string                      `json:"action_id"`
	EventRepo          *AutomationRepoRef          `json:"event_repo"`
	ActionRepoV2       *AutomationRepoRef          `json:"action_repo_v2"`
	SchedulerFrequency *string                     `json:"scheduler_frequency"`
	SchedulerCron      *AutomationCron             `json:"schedulerCron"`
	SearchFor          []AutomationSearchCondition `json:"searchFor"`
	ResponseSchema     json.RawMessage             `json:"responseSchema"`
	Condition          *Condition                  `json:"condition"`
}

// CreatedAutomation is the narrower payload createAutomation returns.
type CreatedAutomation struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ActionID string `json:"action_id"`
	EventID  string `json:"event_id"`
	Active   bool   `json:"active"`
}

// AutomationService reads and writes automations.
type AutomationService struct{ c *Client }

// Create makes an automation. The input stays a map because the resource
// assembles it from Terraform values.
//
// The three outcomes are ordered deliberately: an automation in the payload wins
// even when the response also carried a top-level error, then error_details,
// then the transport error. Reordering these would change which diagnostic a
// user sees.
func (s *AutomationService) Create(ctx context.Context, input map[string]any) (CreatedAutomation, error) {
	var out struct {
		CreateAutomation struct {
			Automation   *CreatedAutomation `json:"automation"`
			ErrorDetails []ErrorDetail      `json:"error_details"`
		} `json:"createAutomation"`
	}
	err := s.c.do(ctx, "CreateAutomation_tf", createAutomationMutation, map[string]any{"input": input}, &out)
	if out.CreateAutomation.Automation != nil {
		return *out.CreateAutomation.Automation, nil
	}
	if len(out.CreateAutomation.ErrorDetails) > 0 {
		return CreatedAutomation{}, &ValidationError{Op: "CreateAutomation_tf", Details: out.CreateAutomation.ErrorDetails, err: err}
	}
	if err != nil {
		return CreatedAutomation{}, err
	}
	return CreatedAutomation{}, ErrNoAutomation
}

// Get returns one automation.
func (s *AutomationService) Get(ctx context.Context, id string) (Automation, error) {
	var out struct {
		Automation *Automation `json:"automation"`
	}
	if err := s.c.do(ctx, "GetAutomation_tf", getAutomationQuery, map[string]any{"id": id}, &out); err != nil {
		return Automation{}, err
	}
	if out.Automation == nil {
		return Automation{}, ErrNotFound
	}
	return *out.Automation, nil
}

// Update applies automation settings. It returns no automation because the
// mutation selects only the id, and it reports failure the same way Create does.
func (s *AutomationService) Update(ctx context.Context, input map[string]any) error {
	var out struct {
		UpdateAutomation struct {
			Automation *struct {
				ID string `json:"id"`
			} `json:"automation"`
			ErrorDetails []ErrorDetail `json:"error_details"`
		} `json:"updateAutomation"`
	}
	err := s.c.do(ctx, "UpdateAutomation_tf", updateAutomationMutation, map[string]any{"input": input}, &out)
	if out.UpdateAutomation.Automation != nil {
		return nil
	}
	if len(out.UpdateAutomation.ErrorDetails) > 0 {
		return &ValidationError{Op: "UpdateAutomation_tf", Details: out.UpdateAutomation.ErrorDetails, err: err}
	}
	if err != nil {
		return err
	}
	return ErrNoAutomation
}

// Delete removes an automation.
func (s *AutomationService) Delete(ctx context.Context, id string) error {
	var out struct {
		DeleteAutomation struct {
			Success bool `json:"success"`
		} `json:"deleteAutomation"`
	}
	return s.c.do(ctx, "DeleteAutomation_tf", deleteAutomationMutation, map[string]any{"id": id}, &out)
}
