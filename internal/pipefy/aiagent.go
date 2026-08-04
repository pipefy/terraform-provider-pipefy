// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const agentSelection = "uuid name instruction repoUuid dataSourceIds disabledAt behaviors { " +
	"id name event_id event_params { to_phase_id triggerFieldIds } action_params { " +
	"aiBehaviorParams { instruction actionsAttributes { id referenceId name actionType " +
	"metadata { destinationPhaseId pipeId fieldsAttributes { fieldId inputMode value } } } } } }"

const createAIAgentMutation = "mutation CreateAiAgent_tf($input:CreateAgentInput!){ " +
	"createAiAgent(input:$input){ agent{ uuid } } }"

const updateAIAgentMutation = "mutation UpdateAiAgent_tf($input:UpdateAgentInput!){ " +
	"updateAiAgent(input:$input){ agent{ uuid } } }"

const updateAIAgentStatusMutation = "mutation UpdateAiAgentStatus_tf($input:UpdateAgentStatusInput!){ " +
	"updateAiAgentStatus(input:$input){ success } }"

const getAIAgentQuery = "query GetAiAgent_tf($uuid:ID!){ aiAgent(uuid:$uuid){ " +
	agentSelection + " } }"

const deleteAIAgentMutation = "mutation DeleteAiAgent_tf($input:DeleteAgentInput!){ " +
	"deleteAiAgent(input:$input){ success errors } }"

// Agent is an AI agent. RepoUUID is the UUID of the pipe owning it.
type Agent struct {
	UUID          string     `json:"uuid"`
	Name          string     `json:"name"`
	Instruction   string     `json:"instruction"`
	RepoUUID      string     `json:"repoUuid"`
	DataSourceIDs []string   `json:"dataSourceIds"`
	DisabledAt    *string    `json:"disabledAt"`
	Behaviors     []Behavior `json:"behaviors"`
}

// Behavior is one trigger-and-actions unit of an agent.
type Behavior struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	EventID      string             `json:"event_id"`
	EventParams  AgentEventParams   `json:"event_params"`
	ActionParams BehaviorActionRoot `json:"action_params"`
}

// AgentEventParams is what triggers a behavior.
type AgentEventParams struct {
	ToPhaseID       *string  `json:"to_phase_id"`
	TriggerFieldIDs []string `json:"triggerFieldIds"`
}

// BehaviorActionRoot is the one-key wrapper the API puts around a behavior's
// action parameters.
type BehaviorActionRoot struct {
	AIBehaviorParams AIBehaviorParams `json:"aiBehaviorParams"`
}

// AIBehaviorParams is the instruction and the actions a behavior runs.
type AIBehaviorParams struct {
	Instruction string   `json:"instruction"`
	Actions     []Action `json:"actionsAttributes"`
}

// Action is one thing a behavior does.
type Action struct {
	ID          string         `json:"id"`
	ReferenceID string         `json:"referenceId"`
	Name        string         `json:"name"`
	ActionType  string         `json:"actionType"`
	Metadata    ActionMetadata `json:"metadata"`
}

// ActionMetadata is an action's target.
type ActionMetadata struct {
	DestinationPhaseID *string      `json:"destinationPhaseId"`
	PipeID             *string      `json:"pipeId"`
	Fields             []AgentField `json:"fieldsAttributes"`
}

// AgentField is one field an action writes.
type AgentField struct {
	FieldID   string  `json:"fieldId"`
	InputMode string  `json:"inputMode"`
	Value     *string `json:"value"`
}

// AiAgentService reads and writes AI agents.
type AiAgentService struct{ c *Client }

// Create makes an agent and returns its UUID. The agent input stays a map
// because the resource builds it from Terraform values.
func (s *AiAgentService) Create(ctx context.Context, agentInput map[string]any) (string, error) {
	var out struct {
		CreateAIAgent struct {
			Agent struct {
				UUID string `json:"uuid"`
			} `json:"agent"`
		} `json:"createAiAgent"`
	}
	vars := map[string]any{"input": map[string]any{"agent": agentInput}}
	if err := s.c.do(ctx, "CreateAiAgent_tf", createAIAgentMutation, vars, &out); err != nil {
		return "", err
	}
	if out.CreateAIAgent.Agent.UUID == "" {
		return "", errors.New("createAiAgent returned an empty agent UUID")
	}
	return out.CreateAIAgent.Agent.UUID, nil
}

// Get returns one agent. This is the only entity where ErrNotFound comes from
// the error message rather than the response shape; see isNotFoundMessage.
func (s *AiAgentService) Get(ctx context.Context, uuid string) (Agent, error) {
	var out struct {
		AIAgent *Agent `json:"aiAgent"`
	}
	if err := s.c.do(ctx, "GetAiAgent_tf", getAIAgentQuery, map[string]any{"uuid": uuid}, &out); err != nil {
		if isNotFoundError(err) {
			return Agent{}, ErrNotFound
		}
		return Agent{}, err
	}
	if out.AIAgent == nil {
		return Agent{}, ErrNotFound
	}
	return *out.AIAgent, nil
}

// Update applies an agent's configuration.
func (s *AiAgentService) Update(ctx context.Context, uuid string, agentInput map[string]any) error {
	var out struct {
		UpdateAIAgent struct {
			Agent struct {
				UUID string `json:"uuid"`
			} `json:"agent"`
		} `json:"updateAiAgent"`
	}
	vars := map[string]any{"input": map[string]any{"uuid": uuid, "agent": agentInput}}
	if err := s.c.do(ctx, "UpdateAiAgent_tf", updateAIAgentMutation, vars, &out); err != nil {
		return err
	}
	if out.UpdateAIAgent.Agent.UUID == "" {
		return errors.New("updateAiAgent returned an empty agent UUID")
	}
	return nil
}

// UpdateStatus enables or disables an agent. It is a separate mutation because
// createAiAgent does not accept the flag.
func (s *AiAgentService) UpdateStatus(ctx context.Context, uuid string, active bool) error {
	var out struct {
		UpdateAIAgentStatus struct {
			Success bool `json:"success"`
		} `json:"updateAiAgentStatus"`
	}
	vars := map[string]any{"input": map[string]any{"uuid": uuid, "active": active}}
	if err := s.c.do(ctx, "UpdateAiAgentStatus_tf", updateAIAgentStatusMutation, vars, &out); err != nil {
		return err
	}
	if !out.UpdateAIAgentStatus.Success {
		return fmt.Errorf("updateAiAgentStatus returned success=false for agent %q", uuid)
	}
	return nil
}

// Delete removes an agent. Deleting one that is already gone succeeds, whether
// the API says so through an error or through the mutation's own errors list.
func (s *AiAgentService) Delete(ctx context.Context, uuid string) error {
	var out struct {
		DeleteAIAgent struct {
			Success bool     `json:"success"`
			Errors  []string `json:"errors"`
		} `json:"deleteAiAgent"`
	}
	vars := map[string]any{"input": map[string]any{"uuid": uuid}}
	err := s.c.do(ctx, "DeleteAiAgent_tf", deleteAIAgentMutation, vars, &out)
	if err != nil && isNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if out.DeleteAIAgent.Success {
		return nil
	}
	if isNotFoundMessage(strings.Join(out.DeleteAIAgent.Errors, "; ")) {
		return nil
	}
	return fmt.Errorf(
		"deleteAiAgent returned success=false for agent %q: %s",
		uuid, strings.Join(out.DeleteAIAgent.Errors, "; "),
	)
}

func isNotFoundError(err error) bool {
	return err != nil && isNotFoundMessage(err.Error())
}

// isNotFoundMessage classifies an aiAgent failure by its text. This is the only
// place in the package that does so: every other entity derives ErrNotFound from
// the shape of a successful response. The aiAgent query reports a missing agent
// as an error rather than a null node, so there is no shape to read.
//
// The auth exclusions matter: a permission failure mentioning a missing
// permission must not be mistaken for a missing agent, or a token problem would
// silently remove a live resource from state.
func isNotFoundMessage(message string) bool {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "record_not_found") {
		return true
	}
	if strings.Contains(lower, "token") ||
		strings.Contains(lower, "permission") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "forbidden") {
		return false
	}
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "couldn't find") ||
		strings.Contains(lower, "could not find")
}
