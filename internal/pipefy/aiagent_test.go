// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestAiAgentsGet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"aiAgent":{
			"uuid":"ag-1","name":"Triager","instruction":"sort things",
			"repoUuid":"pipe-uuid","dataSourceIds":["ds1","ds2"],"disabledAt":null,
			"behaviors":[{"id":"b1","name":"on move","event_id":"card_move",
			  "event_params":{"to_phase_id":"900","triggerFieldIds":["f1"]},
			  "action_params":{"aiBehaviorParams":{"instruction":"do it","actionsAttributes":[
			    {"id":"a1","referenceId":"ref1","name":"fill","actionType":"update_fields",
			     "metadata":{"destinationPhaseId":"901","pipeId":"301","fieldsAttributes":[
			       {"fieldId":"f1","inputMode":"ai","value":null},
			       {"fieldId":"f2","inputMode":"fixed","value":"v2"}]}}]}}}]
		}}}`)
	})

	agent, err := c.AiAgents.Get(t.Context(), "ag-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if agent.UUID != "ag-1" || agent.Name != "Triager" || agent.RepoUUID != "pipe-uuid" {
		t.Errorf("agent = %+v", agent)
	}
	if strings.Join(agent.DataSourceIDs, ",") != "ds1,ds2" {
		t.Errorf("DataSourceIDs = %v", agent.DataSourceIDs)
	}
	if agent.DisabledAt != nil {
		t.Errorf("DisabledAt = %v, want nil", agent.DisabledAt)
	}
	if len(agent.Behaviors) != 1 {
		t.Fatalf("Behaviors = %+v", agent.Behaviors)
	}
	b := agent.Behaviors[0]
	if b.EventID != "card_move" || b.EventParams.ToPhaseID == nil || *b.EventParams.ToPhaseID != "900" {
		t.Errorf("behavior = %+v", b)
	}
	actions := b.ActionParams.AIBehaviorParams.Actions
	if len(actions) != 1 || actions[0].ReferenceID != "ref1" || actions[0].ActionType != "update_fields" {
		t.Fatalf("actions = %+v", actions)
	}
	fields := actions[0].Metadata.Fields
	if len(fields) != 2 || fields[0].FieldID != "f1" {
		t.Fatalf("fields = %+v", fields)
	}
	// A null value is distinct from an empty one, so it stays nil.
	if fields[0].Value != nil {
		t.Errorf("fields[0].Value = %v, want nil", fields[0].Value)
	}
	if fields[1].Value == nil || *fields[1].Value != "v2" {
		t.Errorf("fields[1].Value = %v", fields[1].Value)
	}
}

func TestAiAgentsGetNullAgentIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"aiAgent":null}}`)
	})

	if _, err := c.AiAgents.Get(t.Context(), "ag-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// The aiAgent query reports a missing agent as an error rather than a null node,
// so this entity classifies by message.
func TestAiAgentsGetRecordNotFoundErrorIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"errors":[{"message":"RECORD_NOT_FOUND"}]}`)
	})

	if _, err := c.AiAgents.Get(t.Context(), "ag-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// An auth failure must never read as a missing agent: doing so would drop a live
// resource from state on a token problem.
func TestAiAgentsGetAuthErrorIsNotNotFound(t *testing.T) {
	for name, message := range map[string]string{
		"permission":   "you do not have permission, resource not found",
		"token":        "invalid token, could not find session",
		"unauthorized": "unauthorized: does not exist for this user",
		"forbidden":    "forbidden: not found in your scope",
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, `{"errors":[{"message":"`+message+`"}]}`)
			})
			_, err := c.AiAgents.Get(t.Context(), "ag-1")
			if errors.Is(err, ErrNotFound) {
				t.Fatalf("err = %v, want a hard failure", err)
			}
			if err == nil {
				t.Fatal("err = nil, want a hard failure")
			}
		})
	}
}

func TestIsNotFoundMessage(t *testing.T) {
	cases := map[string]bool{
		"RECORD_NOT_FOUND":               true,
		"AI agent not found":             true,
		"permission not found for token": false,
		"record_not_found with no token": true, // record_not_found wins over the auth exclusions
		"Agent not found":                true,
		"agent does not exist":           true,
		"couldn't find Agent":            true,
		"could not find Agent":           true,
		"invalid token":                  false,
		"missing permission":             false,
		"unauthorized":                   false,
		"forbidden":                      false,
		"permission denied: not found":   false,
		"something else entirely":        false,
		"":                               false,
	}
	for message, want := range cases {
		if got := isNotFoundMessage(message); got != want {
			t.Errorf("isNotFoundMessage(%q) = %v, want %v", message, got, want)
		}
	}
}

func TestAiAgentsCreateReturnsUUID(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"createAiAgent":{"agent":{"uuid":"ag-1"}}}}`)
	})

	uuid, err := c.AiAgents.Create(t.Context(), map[string]any{"name": "Triager"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if uuid != "ag-1" {
		t.Errorf("uuid = %q, want ag-1", uuid)
	}
	// The agent input is nested one level under input.
	input, ok := got.Variables["input"].(map[string]any)
	if !ok {
		t.Fatalf("variables = %+v", got.Variables)
	}
	agent, ok := input["agent"].(map[string]any)
	if !ok || agent["name"] != "Triager" {
		t.Errorf("input = %+v", input)
	}
}

func TestAiAgentsCreateEmptyUUIDKeepsMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"createAiAgent":{"agent":{"uuid":""}}}}`)
	})

	_, err := c.AiAgents.Create(t.Context(), map[string]any{"name": "X"})
	if err == nil || err.Error() != "createAiAgent returned an empty agent UUID" {
		t.Fatalf("err = %v", err)
	}
}

func TestAiAgentsUpdateEmptyUUIDKeepsMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"updateAiAgent":{"agent":{"uuid":""}}}}`)
	})

	err := c.AiAgents.Update(t.Context(), "ag-1", map[string]any{"name": "X"})
	if err == nil || err.Error() != "updateAiAgent returned an empty agent UUID" {
		t.Fatalf("err = %v", err)
	}
}

func TestAiAgentsUpdateStatus(t *testing.T) {
	var got capturedRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capture(t, r)
		respondJSON(w, `{"data":{"updateAiAgentStatus":{"success":true}}}`)
	})

	if err := c.AiAgents.UpdateStatus(t.Context(), "ag-1", false); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	input, ok := got.Variables["input"].(map[string]any)
	if !ok || input["uuid"] != "ag-1" || input["active"] != false {
		t.Errorf("variables = %+v", got.Variables)
	}
}

func TestAiAgentsUpdateStatusFailureKeepsMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"updateAiAgentStatus":{"success":false}}}`)
	})

	err := c.AiAgents.UpdateStatus(t.Context(), "ag-1", true)
	if err == nil || err.Error() != `updateAiAgentStatus returned success=false for agent "ag-1"` {
		t.Fatalf("err = %v", err)
	}
}

// Deleting an already-deleted agent succeeds, whether the API says so through a
// top-level error or through the mutation's own errors list.
func TestAiAgentsDeleteAlreadyDeletedSucceeds(t *testing.T) {
	for name, body := range map[string]string{
		"top-level error": `{"errors":[{"message":"RECORD_NOT_FOUND"}]}`,
		"mutation errors": `{"data":{"deleteAiAgent":{"success":false,"errors":["Agent not found"]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, body)
			})
			if err := c.AiAgents.Delete(t.Context(), "ag-1"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
		})
	}
}

func TestAiAgentsDeleteFailureKeepsMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"deleteAiAgent":{"success":false,"errors":["still referenced","try later"]}}}`)
	})

	err := c.AiAgents.Delete(t.Context(), "ag-1")
	want := `deleteAiAgent returned success=false for agent "ag-1": still referenced; try later`
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestAiAgentsDeleteSucceeds(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, `{"data":{"deleteAiAgent":{"success":true,"errors":[]}}}`)
	})

	if err := c.AiAgents.Delete(t.Context(), "ag-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
