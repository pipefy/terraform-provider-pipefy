// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
)

func TestStringSetRoundTripIgnoresOrder(t *testing.T) {
	configured := stringsToSet([]string{"source-a", "source-b"})
	fromAPI := stringsToSet([]string{"source-b", "source-a"})
	if !configured.Equal(fromAPI) {
		t.Fatalf("set equality failed for reordered IDs: %#v vs %#v", configured, fromAPI)
	}
	values := stringSetValues(fromAPI)
	if len(values) != 2 {
		t.Fatalf("stringSetValues returned %v", values)
	}
}

func TestEnsureActionReferenceIDsIsStable(t *testing.T) {
	model := aiAgentModelWithActions(
		AiAgentActionModel{ReferenceID: types.StringUnknown()},
		AiAgentActionModel{ReferenceID: types.StringValue("existing-reference")},
	)
	if err := ensureActionReferenceIDs(&model); err != nil {
		t.Fatalf("generate reference IDs: %v", err)
	}
	first := model.Behaviors[0].Actions[0].ReferenceID.ValueString()
	if first == "" || model.Behaviors[0].Actions[1].ReferenceID.ValueString() != "existing-reference" {
		t.Fatalf("unexpected references after generation: %#v", model.Behaviors[0].Actions)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(first) {
		t.Fatalf("generated reference %q is not a UUIDv4", first)
	}
	if err := ensureActionReferenceIDs(&model); err != nil {
		t.Fatalf("regenerate reference IDs: %v", err)
	}
	if model.Behaviors[0].Actions[0].ReferenceID.ValueString() != first {
		t.Fatal("generated reference ID changed between calls")
	}
}

func TestBehaviorInstructionRoundTrip(t *testing.T) {
	references := []string{"reference-1", "reference-2"}
	apiValue := instructionForAPI("Analyze the card", references)
	wantSuffix := "%{action:reference-1}\n%{action:reference-2}"
	if !strings.HasSuffix(apiValue, wantSuffix) {
		t.Fatalf("API instruction %q missing suffix %q", apiValue, wantSuffix)
	}
	if got := normalizeBehaviorInstruction(apiValue, references); got != "Analyze the card" {
		t.Fatalf("normalized instruction = %q", got)
	}
}

func TestBehaviorInstructionPreservesUserContent(t *testing.T) {
	const authored = "Keep %{action:user-authored} in the prompt"
	got := normalizeBehaviorInstruction(authored, []string{"generated-reference"})
	if got != authored {
		t.Fatalf("normalization changed user content: %q", got)
	}
}

func TestActionShapeErrors(t *testing.T) {
	cases := map[string]struct {
		action AiAgentActionModel
		want   string
	}{
		"move missing destination": {
			action: actionModel("move_card"),
			want:   `behavior[0].actions[0] action_type "move_card"`,
		},
		"move rejects field metadata": {
			action: actionModel("move_card", withDestination("phase-2"), withPipeID("55"), withFields()),
			want:   "not pipe_id or fields",
		},
		"update missing fields": {
			action: actionModel("update_card", withPipeID("55")),
			want:   "requires pipe_id and at least one field",
		},
		"create missing pipe": {
			action: actionModel("create_card", withFields()),
			want:   "requires pipe_id and at least one field",
		},
		"unsupported": {
			action: actionModel("send_email_template"),
			want:   `"send_email_template"; expected one of`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := actionShapeError(0, 0, tc.action)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestActionShapeAcceptsSupportedMetadata(t *testing.T) {
	actions := []AiAgentActionModel{
		actionModel("move_card", withDestination("phase-2")),
		actionModel("update_card", withPipeID("55"), withFields()),
		actionModel("create_card", withPipeID("56"), withFields()),
	}
	for index, action := range actions {
		if err := actionShapeError(0, index, action); err != nil {
			t.Errorf("action %d rejected: %v", index, err)
		}
	}
}

func TestActionShapeDefersWhenMetadataUnknown(t *testing.T) {
	actions := []AiAgentActionModel{
		actionModel("move_card", withUnknownDestination()),
		actionModel("update_card", withUnknownPipeID(), withFields()),
		actionModel("create_card", withUnknownPipeID(), withFields()),
	}
	for index, action := range actions {
		if err := actionShapeError(0, index, action); err != nil {
			t.Errorf("action %d rejected unknown metadata: %v", index, err)
		}
	}
}

func TestEmptyAPIStringAsNull(t *testing.T) {
	empty := ""
	nonEmpty := "Follow-up"
	if !emptyAPIStringAsNull(nil).IsNull() {
		t.Fatal("nil pointer should map to null")
	}
	if !emptyAPIStringAsNull(&empty).IsNull() {
		t.Fatal("empty API string should map to null")
	}
	got := emptyAPIStringAsNull(&nonEmpty)
	if got.IsNull() || got.ValueString() != nonEmpty {
		t.Fatalf("non-empty value mapped incorrectly: %#v", got)
	}
}

func TestFieldsGraphQLInputAlwaysSendsValue(t *testing.T) {
	fields := []AiAgentFieldModel{{
		FieldID: types.StringValue("title"), InputMode: types.StringValue("fill_with_ai"),
		Value: types.StringNull(),
	}}
	got := fieldsGraphQLInput(fields)
	if got[0]["value"] != "" {
		t.Fatalf("expected empty string value for omitted field, got %#v", got[0]["value"])
	}
}

func TestNormalizeEmptyEventParams(t *testing.T) {
	model := AiAgentModel{Behaviors: []AiAgentBehaviorModel{{
		EventParams: &AiAgentEventParamsModel{
			ToPhaseID:       types.StringNull(),
			TriggerFieldIDs: stringsToSet(nil),
		},
	}}}
	normalizeEmptyEventParams(&model)
	if model.Behaviors[0].EventParams != nil {
		t.Fatalf("empty event_params should normalize to nil, got %#v", model.Behaviors[0].EventParams)
	}
}

func TestNormalizeEmptyEventParamsDefersUnknownTriggerFields(t *testing.T) {
	model := AiAgentModel{Behaviors: []AiAgentBehaviorModel{{
		EventParams: &AiAgentEventParamsModel{
			ToPhaseID: types.StringNull(),
			TriggerFieldIDs: types.SetValueMust(types.StringType, []attr.Value{
				types.StringUnknown(),
			}),
		},
	}}}
	normalizeEmptyEventParams(&model)
	if model.Behaviors[0].EventParams == nil {
		t.Fatal("event_params with unknown trigger_field_ids must not be cleared")
	}
}

func TestRematchNestedIdentitiesOnReorder(t *testing.T) {
	state := AiAgentModel{Behaviors: []AiAgentBehaviorModel{{
		ID: types.StringValue("behavior-1"), Name: types.StringValue("On create"),
		EventID: types.StringValue("card_created"),
		Actions: []AiAgentActionModel{
			{
				ID: types.StringValue("action-1"), ReferenceID: types.StringValue("ref-move"),
				Name: types.StringValue("Move"), ActionType: types.StringValue("move_card"),
				DestinationPhaseID: types.StringValue("phase-2"),
			},
			{
				ID: types.StringValue("action-2"), ReferenceID: types.StringValue("ref-update"),
				Name: types.StringValue("Update"), ActionType: types.StringValue("update_card"),
				PipeID: types.StringValue("42"),
			},
		},
	}}}
	// Simulate UseStateForUnknown copying by index after a reorder.
	plan := AiAgentModel{Behaviors: []AiAgentBehaviorModel{{
		ID: types.StringValue("behavior-1"), Name: types.StringValue("On create"),
		EventID: types.StringValue("card_created"),
		Actions: []AiAgentActionModel{
			{
				ID: types.StringValue("action-1"), ReferenceID: types.StringValue("ref-move"),
				Name: types.StringValue("Update"), ActionType: types.StringValue("update_card"),
				PipeID: types.StringValue("42"),
			},
			{
				ID: types.StringValue("action-2"), ReferenceID: types.StringValue("ref-update"),
				Name: types.StringValue("Move"), ActionType: types.StringValue("move_card"),
				DestinationPhaseID: types.StringValue("phase-2"),
			},
		},
	}}}
	rematchNestedIdentities(&plan, state)
	actions := plan.Behaviors[0].Actions
	if actions[0].ReferenceID.ValueString() != "ref-update" || actions[0].ID.ValueString() != "action-2" {
		t.Fatalf("reordered Update kept wrong identity: %#v", actions[0])
	}
	if actions[1].ReferenceID.ValueString() != "ref-move" || actions[1].ID.ValueString() != "action-1" {
		t.Fatalf("reordered Move kept wrong identity: %#v", actions[1])
	}
}

// Anything the plan already decided has to survive verbatim, and the ids have to
// land on the right entry even when the response reorders the behaviors.
func TestFillFromAgentKeepsPlannedValuesAndGraftsIDs(t *testing.T) {
	plan := plannedAgentModel()
	plan.fillFromAgent(pipefy.Agent{
		UUID: "agent-uuid", Name: "renamed elsewhere", Instruction: "rewritten elsewhere",
		DataSourceIDs: []string{"source-9"},
		Behaviors: []pipefy.Behavior{
			apiBehavior("behavior-updated", "On update", "field_updated", "action-update", "Update", "update_card"),
			apiBehavior("behavior-created", "On create", "card_created", "action-move", "Move", "move_card"),
		},
	})
	if plan.Name.ValueString() != "Triage" || plan.Instruction.ValueString() != "Classify cards" {
		t.Fatalf("response overwrote planned values: %#v", plan)
	}
	if plan.ID.ValueString() != "agent-uuid" || !plan.Active.ValueBool() {
		t.Fatalf("unknowns not filled from response: %#v", plan)
	}
	if !plan.DataSourceIDs.Equal(stringsToSet([]string{"source-1"})) {
		t.Fatalf("response overwrote planned data_source_ids: %#v", plan.DataSourceIDs)
	}
	assertBehaviorIdentity(t, plan.Behaviors[0], "behavior-created", "action-move")
	assertBehaviorIdentity(t, plan.Behaviors[1], "behavior-updated", "action-update")
}

func assertBehaviorIdentity(t *testing.T, behavior AiAgentBehaviorModel, wantID, wantActionID string) {
	t.Helper()
	if behavior.ID.ValueString() != wantID {
		t.Fatalf("behavior %q got id %q, want %q", behavior.Name, behavior.ID, wantID)
	}
	if behavior.Actions[0].ID.ValueString() != wantActionID {
		t.Fatalf("behavior %q got action id %q, want %q", behavior.Name, behavior.Actions[0].ID, wantActionID)
	}
	if behavior.Instruction.ValueString() != "Planned instruction" {
		t.Fatalf("behavior %q lost its planned instruction: %q", behavior.Name, behavior.Instruction)
	}
}

func plannedAgentModel() AiAgentModel {
	return AiAgentModel{
		ID: types.StringUnknown(), Name: types.StringValue("Triage"),
		Instruction: types.StringValue("Classify cards"), Active: types.BoolUnknown(),
		DataSourceIDs: stringsToSet([]string{"source-1"}),
		Behaviors: []AiAgentBehaviorModel{
			plannedBehavior("On create", "card_created", "Move", "move_card"),
			plannedBehavior("On update", "field_updated", "Update", "update_card"),
		},
	}
}

func plannedBehavior(name, eventID, actionName, actionType string) AiAgentBehaviorModel {
	return AiAgentBehaviorModel{
		ID: types.StringUnknown(), Name: types.StringValue(name),
		EventID: types.StringValue(eventID), Instruction: types.StringValue("Planned instruction"),
		Actions: []AiAgentActionModel{{
			ID: types.StringUnknown(), ReferenceID: types.StringValue("ref-" + actionName),
			Name: types.StringValue(actionName), ActionType: types.StringValue(actionType),
		}},
	}
}

func apiBehavior(id, name, eventID, actionID, actionName, actionType string) pipefy.Behavior {
	return pipefy.Behavior{
		ID: id, Name: name, EventID: eventID,
		ActionParams: pipefy.BehaviorActionRoot{AIBehaviorParams: pipefy.AIBehaviorParams{
			Instruction: "Planned instruction",
			Actions: []pipefy.Action{{
				ID: actionID, ReferenceID: "ref-" + actionName,
				Name: actionName, ActionType: actionType,
			}},
		}},
	}
}

type actionOption func(*AiAgentActionModel)

func actionModel(actionType string, options ...actionOption) AiAgentActionModel {
	action := AiAgentActionModel{ActionType: types.StringValue(actionType)}
	for _, option := range options {
		option(&action)
	}
	return action
}

func withDestination(id string) actionOption {
	return func(action *AiAgentActionModel) { action.DestinationPhaseID = types.StringValue(id) }
}

func withUnknownDestination() actionOption {
	return func(action *AiAgentActionModel) { action.DestinationPhaseID = types.StringUnknown() }
}

func withPipeID(id string) actionOption {
	return func(action *AiAgentActionModel) { action.PipeID = types.StringValue(id) }
}

func withUnknownPipeID() actionOption {
	return func(action *AiAgentActionModel) { action.PipeID = types.StringUnknown() }
}

func withFields() actionOption {
	return func(action *AiAgentActionModel) {
		action.Fields = []AiAgentFieldModel{{
			FieldID: types.StringValue("field-1"), InputMode: types.StringValue("fill_with_ai"),
		}}
	}
}

func aiAgentModelWithActions(actions ...AiAgentActionModel) AiAgentModel {
	return AiAgentModel{Behaviors: []AiAgentBehaviorModel{{Actions: actions}}}
}
