// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package aiagentgql contains the GraphQL selection and typed wire payloads
// shared by the pipefy_ai_agent resource.
package aiagentgql

const Selection = "uuid name instruction repoUuid dataSourceIds disabledAt behaviors { " +
	"id name event_id event_params { to_phase_id triggerFieldIds } action_params { " +
	"aiBehaviorParams { instruction actionsAttributes { id referenceId name actionType " +
	"metadata { destinationPhaseId pipeId fieldsAttributes { fieldId inputMode value } } } } } }"

type Agent struct {
	UUID          string     `json:"uuid"`
	Name          string     `json:"name"`
	Instruction   string     `json:"instruction"`
	RepoUUID      string     `json:"repoUuid"`
	DataSourceIDs []string   `json:"dataSourceIds"`
	DisabledAt    *string    `json:"disabledAt"`
	Behaviors     []Behavior `json:"behaviors"`
}

type Behavior struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	EventID      string             `json:"event_id"`
	EventParams  EventParams        `json:"event_params"`
	ActionParams BehaviorActionRoot `json:"action_params"`
}

type EventParams struct {
	ToPhaseID       *string  `json:"to_phase_id"`
	TriggerFieldIDs []string `json:"triggerFieldIds"`
}

type BehaviorActionRoot struct {
	AIBehaviorParams AIBehaviorParams `json:"aiBehaviorParams"`
}

type AIBehaviorParams struct {
	Instruction string   `json:"instruction"`
	Actions     []Action `json:"actionsAttributes"`
}

type Action struct {
	ID          string         `json:"id"`
	ReferenceID string         `json:"referenceId"`
	Name        string         `json:"name"`
	ActionType  string         `json:"actionType"`
	Metadata    ActionMetadata `json:"metadata"`
}

type ActionMetadata struct {
	DestinationPhaseID *string `json:"destinationPhaseId"`
	PipeID             *string `json:"pipeId"`
	Fields             []Field `json:"fieldsAttributes"`
}

type Field struct {
	FieldID   string  `json:"fieldId"`
	InputMode string  `json:"inputMode"`
	Value     *string `json:"value"`
}
