// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aiagentgql_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pipefy/terraform-provider-pipefy/internal/provider/aiagentgql"
)

func TestAgentPayloadMapping(t *testing.T) {
	const payload = `{
		"uuid":"agent-uuid","name":"Triage","instruction":"Classify cards",
		"repoUuid":"pipe-uuid","dataSourceIds":["source-1"],"disabledAt":null,
		"behaviors":[{
			"id":"behavior-1","name":"On create","event_id":"card_created",
			"event_params":{"to_phase_id":"phase-2","triggerFieldIds":["field-1"]},
			"action_params":{"aiBehaviorParams":{
				"instruction":"Choose a destination\n%{action:reference-1}",
				"actionsAttributes":[{
					"id":"action-1","referenceId":"reference-1","name":"Move",
					"actionType":"move_card","metadata":{"destinationPhaseId":"phase-2"}
				}]
			}}
		}]
	}`
	var got aiagentgql.Agent
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("unmarshal AI agent payload: %v", err)
	}
	assertMappedAgent(t, got)
}

func assertMappedAgent(t *testing.T, got aiagentgql.Agent) {
	t.Helper()
	if got.UUID != "agent-uuid" || got.RepoUUID != "pipe-uuid" {
		t.Fatalf("unexpected agent identity: %#v", got)
	}
	behavior := got.Behaviors[0]
	if behavior.ID != "behavior-1" || behavior.EventParams.TriggerFieldIDs[0] != "field-1" {
		t.Fatalf("unexpected behavior mapping: %#v", behavior)
	}
	action := behavior.ActionParams.AIBehaviorParams.Actions[0]
	if action.ReferenceID != "reference-1" || action.Metadata.DestinationPhaseID == nil ||
		*action.Metadata.DestinationPhaseID != "phase-2" {
		t.Fatalf("unexpected action mapping: %#v", action)
	}
}

func TestSelectionContainsManagedFields(t *testing.T) {
	required := []string{
		"uuid", "repoUuid", "dataSourceIds", "disabledAt", "behaviors",
		"event_id", "event_params", "aiBehaviorParams", "actionsAttributes",
		"referenceId", "destinationPhaseId", "fieldsAttributes",
	}
	for _, field := range required {
		if !strings.Contains(aiagentgql.Selection, field) {
			t.Errorf("selection missing managed field %q", field)
		}
	}
}
