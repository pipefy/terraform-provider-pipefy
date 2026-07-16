// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestUnit_AiAgentResource_RollsBackFailedRead(t *testing.T) {
	mock := &aiAgentMock{failRead: true}
	server := newAiAgentServer(mock)
	defer server.Close()
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{{
		Config:      aiAgentConfig(server.URL, "", false),
		ExpectError: regexp.MustCompile("agent read rejected"),
	}}))
	if mock.deleteCalls != 1 || mock.exists {
		t.Fatalf("rollback deleteCalls=%d exists=%v, want 1 and false", mock.deleteCalls, mock.exists)
	}
}

func TestUnit_AiAgentResource_ReportsRollbackFailure(t *testing.T) {
	mock := &aiAgentMock{failRead: true, failDelete: true}
	server := newAiAgentServer(mock)
	defer server.Close()
	want := regexp.MustCompile(`create AI agent failed and rollback failed`)
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{{
		Config: aiAgentConfig(server.URL, "", false), ExpectError: want,
	}}))
	if mock.deleteCalls < 1 {
		t.Fatalf("rollback delete calls = %d, want at least 1", mock.deleteCalls)
	}
}

func TestUnit_AiAgentResource_RollsBackFailedStatus(t *testing.T) {
	mock := &aiAgentMock{failStatus: true}
	server := newAiAgentServer(mock)
	defer server.Close()
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{{
		Config:      aiAgentConfig(server.URL, "true", false),
		ExpectError: regexp.MustCompile("status update rejected"),
	}}))
	if mock.deleteCalls != 1 || mock.exists {
		t.Fatalf("rollback deleteCalls=%d exists=%v, want 1 and false", mock.deleteCalls, mock.exists)
	}
}

func TestUnit_AiAgentResource_CreateFailureDoesNotDelete(t *testing.T) {
	mock := &aiAgentMock{failCreate: true}
	server := newAiAgentServer(mock)
	defer server.Close()
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{{
		Config:      aiAgentConfig(server.URL, "", false),
		ExpectError: regexp.MustCompile("agent create rejected"),
	}}))
	if mock.deleteCalls != 0 {
		t.Fatalf("delete called %d times after create failed, want 0", mock.deleteCalls)
	}
}

func TestUnit_AiAgentResource_CreateRollsBackWhenReadReturnsNull(t *testing.T) {
	mock := &aiAgentMock{readNull: true}
	server := newAiAgentServer(mock)
	defer server.Close()
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{{
		Config:      aiAgentConfig(server.URL, "", false),
		ExpectError: regexp.MustCompile("(?s)create AI agent failed.*returned no agent"),
	}}))
	if mock.deleteCalls != 1 || mock.exists {
		t.Fatalf("rollback deleteCalls=%d exists=%v, want 1 and false", mock.deleteCalls, mock.exists)
	}
}

func TestUnit_AiAgentResource_UpdateFailsWhenReadReturnsNull(t *testing.T) {
	mock := &aiAgentMock{}
	server := newAiAgentServer(mock)
	defer server.Close()
	config := aiAgentConfig(server.URL, "", false)
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{
		{Config: config},
		{
			PreConfig:   func() { mock.nullAfterUpdate = true },
			Config:      aiAgentConfig(server.URL, "", true),
			ExpectError: regexp.MustCompile("(?s)read AI agent after update failed.*returned no agent"),
		},
	}))
}

func TestUnit_AiAgentResource_ReadFailureIsActionable(t *testing.T) {
	mock := &aiAgentMock{}
	server := newAiAgentServer(mock)
	defer server.Close()
	config := aiAgentConfig(server.URL, "", false)
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{
		{Config: config},
		{
			PreConfig: func() { mock.failRead = true },
			Config:    config, PlanOnly: true,
			ExpectError: regexp.MustCompile("(?s)read AI agent failed.*agent read rejected"),
		},
	}))
}

func TestUnit_AiAgentResource_DeleteFailureIsActionable(t *testing.T) {
	mock := &aiAgentMock{}
	server := newAiAgentServer(mock)
	defer server.Close()
	config := aiAgentConfig(server.URL, "", false)
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{
		{Config: config},
		{
			PreConfig:   func() { mock.failDelete = true },
			Config:      aiAgentProvider(server.URL),
			ExpectError: regexp.MustCompile("(?s)delete AI agent failed.*rollback delete rejected"),
		},
	}))
}

func TestUnit_AiAgentResource_Import(t *testing.T) {
	mock := &aiAgentMock{}
	server := newAiAgentServer(mock)
	defer server.Close()
	config := aiAgentConfig(server.URL, "", false)
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{
		{Config: config},
		{
			ResourceName: "pipefy_ai_agent.test", ImportState: true,
			ImportStateId: "42/agent-uuid", ImportStateVerify: true,
		},
		{
			ResourceName: "pipefy_ai_agent.test", ImportState: true,
			ImportStateId: "agent-uuid",
			ExpectError:   regexp.MustCompile(`got "agent-uuid".*pipe_id/agent_uuid`),
		},
	}))
}

func TestUnit_AiAgentResource_ImportRejectsMismatchedPipe(t *testing.T) {
	mock := &aiAgentMock{
		exists: true, name: "Triage", instruction: "Classify cards",
		pipeUUIDByID: map[string]string{"999": "other-pipe-uuid", "42": "pipe-uuid"},
		behaviors: []any{map[string]any{
			"name": "On create", "eventId": "card_created",
			"actionParams": map[string]any{"aiBehaviorParams": map[string]any{
				"instruction": "Choose a destination",
				"actionsAttributes": []any{map[string]any{
					"name": "Move", "actionType": "move_card", "referenceId": "ref-1",
					"metadata": map[string]any{"destinationPhaseId": "phase-2"},
				}},
			}},
		}},
	}
	server := newAiAgentServer(mock)
	defer server.Close()
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{{
		Config:             aiAgentConfig(server.URL, "", false),
		ResourceName:       "pipefy_ai_agent.test",
		ImportState:        true,
		ImportStateId:      "999/agent-uuid",
		ImportStatePersist: true,
		ExpectError:        regexp.MustCompile(`belongs to repo UUID`),
	}}))
}

func TestUnit_AiAgentResource_OmittedFieldValueNoPerpetualDiff(t *testing.T) {
	mock := &aiAgentMock{}
	server := newAiAgentServer(mock)
	defer server.Close()
	config := aiAgentConfig(server.URL, "", true)
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{
		{Config: config},
		{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
	}))
}

func TestUnit_AiAgentResource_UpdateStatusFailureKeepsConfigState(t *testing.T) {
	mock := &aiAgentMock{}
	server := newAiAgentServer(mock)
	defer server.Close()
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{
		{Config: aiAgentConfig(server.URL, "false", false)},
		{
			PreConfig:   func() { mock.failStatus = true },
			Config:      aiAgentConfig(server.URL, "true", true),
			ExpectError: regexp.MustCompile("update AI agent status failed"),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(
					"pipefy_ai_agent.test", tfjsonpath.New("active"), knownvalue.Bool(false),
				),
				statecheck.ExpectKnownValue(
					"pipefy_ai_agent.test",
					tfjsonpath.New("behaviors").AtSliceIndex(1).AtMapKey("name"),
					knownvalue.StringExact("On field update"),
				),
			},
		},
	}))
}
