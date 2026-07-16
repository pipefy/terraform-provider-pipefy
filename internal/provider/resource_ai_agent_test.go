// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

type aiAgentMock struct {
	mu               sync.Mutex
	exists           bool
	active           bool
	name             string
	instruction      string
	behaviors        []any
	dataSourceIDs    []any
	operations       []string
	referenceHistory [][]string
	pipeUUIDByID     map[string]string
	failCreate       bool
	failUpdate       bool
	failStatus       bool
	failRead         bool
	readNull         bool
	nullAfterUpdate  bool
	failDelete       bool
	deleteCalls      int
}

func (mock *aiAgentMock) serveHTTP(w http.ResponseWriter, r *http.Request) {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var request gqlReq
	_ = json.Unmarshal(body, &request)
	w.Header().Set("Content-Type", "application/json")
	operation := aiAgentOperation(request.Query)
	mock.operations = append(mock.operations, operation)
	response := mock.handle(operation, request.Variables)
	_, _ = io.WriteString(w, response)
}

func (mock *aiAgentMock) handle(operation string, variables map[string]any) string {
	switch operation {
	case "GetPipeUuid":
		return mock.pipeUUID(variables)
	case "Create":
		return mock.create(variables)
	case "Update":
		return mock.update(variables)
	case "Status":
		return mock.updateStatus(variables)
	case "Read":
		return mock.read()
	case "Delete":
		return mock.delete()
	default:
		return `{"data":{}}`
	}
}

func (mock *aiAgentMock) pipeUUID(variables map[string]any) string {
	id, _ := variables["id"].(string)
	if mock.pipeUUIDByID != nil {
		if uuid, ok := mock.pipeUUIDByID[id]; ok {
			return `{"data":{"pipe":{"uuid":"` + uuid + `"}}}`
		}
	}
	return `{"data":{"pipe":{"uuid":"pipe-uuid"}}}`
}

func (mock *aiAgentMock) create(variables map[string]any) string {
	if mock.failCreate {
		return `{"errors":[{"message":"agent create rejected"}]}`
	}
	agent := aiAgentInput(variables)
	mock.exists = true
	mock.name, _ = agent["name"].(string)
	mock.instruction, _ = agent["instruction"].(string)
	return `{"data":{"createAiAgent":{"agent":{"uuid":"agent-uuid"}}}}`
}

func (mock *aiAgentMock) update(variables map[string]any) string {
	if mock.failUpdate {
		return `{"errors":[{"message":"behavior update rejected"}]}`
	}
	agent := aiAgentInput(variables)
	mock.name, _ = agent["name"].(string)
	mock.instruction, _ = agent["instruction"].(string)
	mock.behaviors, _ = agent["behaviors"].([]any)
	mock.dataSourceIDs, _ = agent["dataSourceIds"].([]any)
	mock.referenceHistory = append(mock.referenceHistory, actionReferences(mock.behaviors))
	if mock.nullAfterUpdate {
		mock.readNull = true
	}
	return `{"data":{"updateAiAgent":{"agent":{"uuid":"agent-uuid"}}}}`
}

func (mock *aiAgentMock) updateStatus(variables map[string]any) string {
	if mock.failStatus {
		return `{"errors":[{"message":"status update rejected"}]}`
	}
	input, _ := variables["input"].(map[string]any)
	mock.active, _ = input["active"].(bool)
	return `{"data":{"updateAiAgentStatus":{"success":true}}}`
}

func (mock *aiAgentMock) read() string {
	if mock.failRead {
		return `{"errors":[{"message":"agent read rejected"}]}`
	}
	if !mock.exists || mock.readNull {
		return `{"data":{"aiAgent":null}}`
	}
	payload, _ := json.Marshal(mock.agentPayload())
	return `{"data":{"aiAgent":` + string(payload) + `}}`
}

func (mock *aiAgentMock) delete() string {
	mock.deleteCalls++
	if mock.failDelete {
		mock.failDelete = false
		return `{"errors":[{"message":"rollback delete rejected"}]}`
	}
	mock.exists = false
	return `{"data":{"deleteAiAgent":{"success":true,"errors":[]}}}`
}

func (mock *aiAgentMock) agentPayload() map[string]any {
	disabledAt := any("2026-01-01T00:00:00Z")
	if mock.active {
		disabledAt = nil
	}
	return map[string]any{
		"uuid": "agent-uuid", "repoUuid": "pipe-uuid", "name": mock.name,
		"instruction": mock.instruction, "disabledAt": disabledAt,
		"dataSourceIds": reversedAnySlice(mock.dataSourceIDs),
		"behaviors":     responseBehaviors(mock.behaviors),
	}
}

func reversedAnySlice(values []any) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[len(values)-1-index] = value
	}
	return result
}

func responseBehaviors(behaviors []any) []any {
	result := make([]any, len(behaviors))
	for index, raw := range behaviors {
		behavior, _ := raw.(map[string]any)
		params := aiBehaviorParams(behavior)
		actions, _ := params["actionsAttributes"].([]any)
		responseParams := cloneMap(params)
		responseParams["actionsAttributes"] = responseActions(actions)
		result[index] = map[string]any{
			"id": "behavior-" + string(rune('1'+index)), "name": behavior["name"],
			"event_id": behavior["eventId"], "event_params": behavior["eventParams"],
			"action_params": map[string]any{"aiBehaviorParams": responseParams},
		}
	}
	return result
}

func responseActions(actions []any) []any {
	result := make([]any, len(actions))
	for index, raw := range actions {
		action, _ := raw.(map[string]any)
		cloned := cloneMap(action)
		cloned["id"] = "action-" + string(rune('1'+index))
		if metadata, ok := cloned["metadata"].(map[string]any); ok {
			cloned["metadata"] = responseMetadata(metadata)
		}
		result[index] = cloned
	}
	return result
}

func responseMetadata(metadata map[string]any) map[string]any {
	cloned := cloneMap(metadata)
	fields, _ := cloned["fieldsAttributes"].([]any)
	if len(fields) == 0 {
		return cloned
	}
	normalized := make([]any, len(fields))
	for index, raw := range fields {
		field, _ := raw.(map[string]any)
		fieldClone := cloneMap(field)
		if _, ok := fieldClone["value"]; !ok {
			fieldClone["value"] = ""
		}
		normalized[index] = fieldClone
	}
	cloned["fieldsAttributes"] = normalized
	return cloned
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func aiAgentInput(variables map[string]any) map[string]any {
	input, _ := variables["input"].(map[string]any)
	agent, _ := input["agent"].(map[string]any)
	return agent
}

func aiBehaviorParams(behavior map[string]any) map[string]any {
	actionParams, _ := behavior["actionParams"].(map[string]any)
	params, _ := actionParams["aiBehaviorParams"].(map[string]any)
	return params
}

func actionReferences(behaviors []any) []string {
	var references []string
	for _, rawBehavior := range behaviors {
		behavior, _ := rawBehavior.(map[string]any)
		actions, _ := aiBehaviorParams(behavior)["actionsAttributes"].([]any)
		for _, rawAction := range actions {
			action, _ := rawAction.(map[string]any)
			reference, _ := action["referenceId"].(string)
			references = append(references, reference)
		}
	}
	return references
}

func aiAgentOperation(query string) string {
	operations := map[string]string{
		"GetPipeUuid_tf": "GetPipeUuid", "CreateAiAgent_tf": "Create",
		"UpdateAiAgentStatus_tf": "Status", "UpdateAiAgent_tf": "Update",
		"GetAiAgent_tf": "Read", "DeleteAiAgent_tf": "Delete",
	}
	for marker, operation := range operations {
		if strings.Contains(query, marker) {
			return operation
		}
	}
	return "Unknown"
}

func newAiAgentServer(mock *aiAgentMock) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
}

func aiAgentProvider(endpoint string) string {
	return `provider "pipefy" {
		endpoint = "` + endpoint + `"
		token = "testtoken"
	}`
}

func aiAgentConfig(endpoint string, active string, secondBehavior bool) string {
	behaviors := aiAgentBehaviorMove()
	if secondBehavior {
		behaviors += "," + aiAgentBehaviorUpdate()
	}
	activeLine := ""
	if active != "" {
		activeLine = "active = " + active
	}
	return aiAgentProvider(endpoint) + `
	resource "pipefy_ai_agent" "test" {
		pipe_id = "42"
		name = "Triage"
		instruction = "Classify cards"
		data_source_ids = ["source-1"]
		` + activeLine + `
		behaviors = [` + behaviors + `]
	}`
}

func aiAgentBehaviorMove() string {
	return `{
		name = "On create"
		event_id = "card_created"
		instruction = "Choose a destination"
		actions = [{
			name = "Move"
			action_type = "move_card"
			destination_phase_id = "phase-2"
		}]
	}`
}

func aiAgentBehaviorUpdate() string {
	return `{
		name = "On field update"
		event_id = "field_updated"
		instruction = "Rewrite title"
		event_params = { trigger_field_ids = ["field-1"] }
		actions = [{
			name = "Update"
			action_type = "update_card"
			pipe_id = "42"
			fields = [{ field_id = "title", input_mode = "fill_with_ai" }]
		}]
	}`
}

func aiAgentTestCase(steps []resource.TestStep) resource.TestCase {
	return resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	}
}

func TestUnit_AiAgentResource_CRUD(t *testing.T) {
	mock := &aiAgentMock{}
	server := newAiAgentServer(mock)
	defer server.Close()
	first := aiAgentConfig(server.URL, "true", false)
	updated := aiAgentConfig(server.URL, "false", true)
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{
		{Config: first, ConfigStateChecks: aiAgentStateChecks()},
		{Config: first},
		{Config: updated},
	}))
	assertAiAgentCRUD(t, mock)
}

func aiAgentStateChecks() []statecheck.StateCheck {
	return []statecheck.StateCheck{
		statecheck.ExpectKnownValue(
			"pipefy_ai_agent.test", tfjsonpath.New("id"), knownvalue.StringExact("agent-uuid"),
		),
		statecheck.ExpectKnownValue(
			"pipefy_ai_agent.test", tfjsonpath.New("active"), knownvalue.Bool(true),
		),
		statecheck.ExpectKnownValue(
			"pipefy_ai_agent.test", tfjsonpath.New("data_source_ids"),
			knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("source-1")}),
		),
	}
}

func assertAiAgentCRUD(t *testing.T, mock *aiAgentMock) {
	t.Helper()
	wantPrefix := []string{"GetPipeUuid", "Create", "Update", "Status", "Read"}
	if len(mock.operations) < len(wantPrefix) {
		t.Fatalf("operations = %v, want prefix %v", mock.operations, wantPrefix)
	}
	for index, operation := range wantPrefix {
		if mock.operations[index] != operation {
			t.Fatalf("operations = %v, want prefix %v", mock.operations, wantPrefix)
		}
	}
	if len(mock.behaviors) != 2 || mock.deleteCalls != 1 {
		t.Fatalf("behaviors=%d deleteCalls=%d, want 2 and 1", len(mock.behaviors), mock.deleteCalls)
	}
	assertStableReferences(t, mock.referenceHistory)
}

func assertStableReferences(t *testing.T, history [][]string) {
	t.Helper()
	if len(history) < 2 || len(history[0]) != 1 || len(history[1]) != 2 {
		t.Fatalf("unexpected reference history: %#v", history)
	}
	if history[0][0] == "" || history[0][0] != history[1][0] || history[1][1] == "" {
		t.Fatalf("action reference IDs were not stable: %#v", history)
	}
}

func TestUnit_AiAgentResource_OmittedActiveSkipsStatus(t *testing.T) {
	mock := &aiAgentMock{}
	server := newAiAgentServer(mock)
	defer server.Close()
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{{
		Config: aiAgentConfig(server.URL, "", false),
	}}))
	for _, operation := range mock.operations {
		if operation == "Status" {
			t.Fatalf("status mutation called with omitted active: %v", mock.operations)
		}
	}
}

func TestUnit_AiAgentResource_RemoteDeletion(t *testing.T) {
	mock := &aiAgentMock{}
	server := newAiAgentServer(mock)
	defer server.Close()
	config := aiAgentConfig(server.URL, "", false)
	resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{
		{Config: config},
		{
			PreConfig: func() { mock.exists = false },
			Config:    config, PlanOnly: true, ExpectNonEmptyPlan: true,
		},
	}))
}

func TestUnit_AiAgentResource_Validations(t *testing.T) {
	cases := aiAgentValidationCases()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, aiAgentTestCase([]resource.TestStep{{
				Config:   aiAgentValidationConfig(tc.behaviors),
				PlanOnly: true, ExpectError: regexp.MustCompile(tc.want),
			}}))
		})
	}
}

type aiAgentValidationCase struct {
	behaviors string
	want      string
}

func aiAgentValidationCases() map[string]aiAgentValidationCase {
	return map[string]aiAgentValidationCase{
		"no behaviors": {behaviors: "[]", want: "at least 1"},
		"no actions": {
			behaviors: `[{name="B",event_id="card_created",instruction="I",actions=[]}]`,
			want:      "at least 1",
		},
		"unsupported action": {
			behaviors: behaviorWithAction(`{name="Bad",action_type="send_email_template"}`),
			want:      "send_email_template.*expected one of",
		},
		"move metadata": {
			behaviors: behaviorWithAction(`{name="Move",action_type="move_card"}`),
			want:      "requires destination_phase_id",
		},
		"update metadata": {
			behaviors: behaviorWithAction(`{name="Update",action_type="update_card",pipe_id="42"}`),
			want:      "(?s)requires pipe_id and at.*least one field",
		},
	}
}

func behaviorWithAction(action string) string {
	return `[{name="B",event_id="card_created",instruction="I",actions=[` + action + `]}]`
}

func aiAgentValidationConfig(behaviors string) string {
	return `provider "pipefy" { token = "testtoken" }
	resource "pipefy_ai_agent" "test" {
		pipe_id = "42"
		name = "Agent"
		instruction = "Instruction"
		behaviors = ` + behaviors + `
	}`
}
