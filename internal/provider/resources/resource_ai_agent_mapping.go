// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/aiagentgql"
)

func ensureActionReferenceIDs(model *AiAgentModel) error {
	for behaviorIndex := range model.Behaviors {
		actions := model.Behaviors[behaviorIndex].Actions
		for actionIndex := range actions {
			reference := actions[actionIndex].ReferenceID
			if !reference.IsNull() && !reference.IsUnknown() && reference.ValueString() != "" {
				continue
			}
			generated, err := generateActionReferenceID()
			if err != nil {
				return err
			}
			actions[actionIndex].ReferenceID = types.StringValue(generated)
		}
		model.Behaviors[behaviorIndex].Actions = actions
	}
	return nil
}

func generateActionReferenceID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate UUID bytes: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16],
	), nil
}

func prepareCreatedPartialState(model *AiAgentModel) {
	if model.Active.IsUnknown() {
		model.Active = types.BoolValue(false)
	}
	for behaviorIndex := range model.Behaviors {
		behavior := &model.Behaviors[behaviorIndex]
		if behavior.ID.IsUnknown() {
			behavior.ID = types.StringNull()
		}
		for actionIndex := range behavior.Actions {
			if behavior.Actions[actionIndex].ID.IsUnknown() {
				behavior.Actions[actionIndex].ID = types.StringNull()
			}
		}
	}
}

func instructionForAPI(instruction string, references []string) string {
	result := instruction
	for _, reference := range references {
		result += "\n%{action:" + reference + "}"
	}
	return result
}

func normalizeBehaviorInstruction(instruction string, references []string) string {
	result := instruction
	for index := len(references) - 1; index >= 0; index-- {
		suffix := "\n%{action:" + references[index] + "}"
		if !strings.HasSuffix(result, suffix) {
			return instruction
		}
		result = strings.TrimSuffix(result, suffix)
	}
	return result
}

func (model AiAgentModel) graphQLInput(repoUUID string) map[string]any {
	input := map[string]any{
		"name":          model.Name.ValueString(),
		"instruction":   model.Instruction.ValueString(),
		"repoUuid":      repoUUID,
		"dataSourceIds": stringSetValues(model.DataSourceIDs),
	}
	behaviors := make([]map[string]any, len(model.Behaviors))
	for index, behavior := range model.Behaviors {
		behaviors[index] = behavior.graphQLInput()
	}
	input["behaviors"] = behaviors
	return input
}

func (behavior AiAgentBehaviorModel) graphQLInput() map[string]any {
	input := map[string]any{
		"name": behavior.Name.ValueString(), "eventId": behavior.EventID.ValueString(),
	}
	if hasString(behavior.ID) {
		input["id"] = behavior.ID.ValueString()
	}
	addEventParams(input, behavior.EventParams)
	actions := make([]map[string]any, len(behavior.Actions))
	references := make([]string, len(behavior.Actions))
	for index, action := range behavior.Actions {
		actions[index] = action.graphQLInput()
		references[index] = action.ReferenceID.ValueString()
	}
	input["actionParams"] = map[string]any{"aiBehaviorParams": map[string]any{
		"instruction":       instructionForAPI(behavior.Instruction.ValueString(), references),
		"actionsAttributes": actions,
	}}
	return input
}

func addEventParams(input map[string]any, params *AiAgentEventParamsModel) {
	if params == nil {
		return
	}
	eventParams := map[string]any{}
	if hasString(params.ToPhaseID) {
		eventParams["to_phase_id"] = params.ToPhaseID.ValueString()
	}
	triggerFieldIDs := stringSetValues(params.TriggerFieldIDs)
	if len(triggerFieldIDs) > 0 {
		eventParams["triggerFieldIds"] = triggerFieldIDs
	}
	if len(eventParams) == 0 {
		return
	}
	input["eventParams"] = eventParams
}

func (action AiAgentActionModel) graphQLInput() map[string]any {
	input := map[string]any{
		"name": action.Name.ValueString(), "actionType": action.ActionType.ValueString(),
		"referenceId": action.ReferenceID.ValueString(),
	}
	if hasString(action.ID) {
		input["id"] = action.ID.ValueString()
	}
	metadata := map[string]any{}
	if hasString(action.DestinationPhaseID) {
		metadata["destinationPhaseId"] = action.DestinationPhaseID.ValueString()
	}
	if hasString(action.PipeID) {
		metadata["pipeId"] = action.PipeID.ValueString()
	}
	if len(action.Fields) > 0 {
		metadata["fieldsAttributes"] = fieldsGraphQLInput(action.Fields)
	}
	input["metadata"] = metadata
	return input
}

func fieldsGraphQLInput(fields []AiAgentFieldModel) []map[string]any {
	result := make([]map[string]any, len(fields))
	for index, field := range fields {
		value := ""
		if !field.Value.IsNull() && !field.Value.IsUnknown() {
			value = field.Value.ValueString()
		}
		result[index] = map[string]any{
			"fieldId": field.FieldID.ValueString(), "inputMode": field.InputMode.ValueString(),
			"value": value,
		}
	}
	return result
}

func stringSetValues(values types.Set) []string {
	if values.IsNull() || values.IsUnknown() {
		return []string{}
	}
	result := make([]string, 0, len(values.Elements()))
	for _, element := range values.Elements() {
		value, ok := element.(types.String)
		if !ok || value.IsNull() || value.IsUnknown() {
			continue
		}
		result = append(result, value.ValueString())
	}
	return result
}

func (model *AiAgentModel) applyGraphQL(agent aiagentgql.Agent) {
	model.ID = types.StringValue(agent.UUID)
	model.Name = types.StringValue(agent.Name)
	model.Instruction = types.StringValue(agent.Instruction)
	model.Active = types.BoolValue(agent.DisabledAt == nil)
	model.DataSourceIDs = stringsToSet(agent.DataSourceIDs)
	model.Behaviors = behaviorsToModel(agent.Behaviors)
}

func behaviorsToModel(behaviors []aiagentgql.Behavior) []AiAgentBehaviorModel {
	result := make([]AiAgentBehaviorModel, len(behaviors))
	for index, behavior := range behaviors {
		actions := actionsToModel(behavior.ActionParams.AIBehaviorParams.Actions)
		references := actionReferenceStrings(actions)
		result[index] = AiAgentBehaviorModel{
			ID: types.StringValue(behavior.ID), Name: types.StringValue(behavior.Name),
			EventID: types.StringValue(behavior.EventID),
			Instruction: types.StringValue(normalizeBehaviorInstruction(
				behavior.ActionParams.AIBehaviorParams.Instruction, references,
			)),
			EventParams: eventParamsToModel(behavior.EventParams), Actions: actions,
		}
	}
	return result
}

func actionsToModel(actions []aiagentgql.Action) []AiAgentActionModel {
	result := make([]AiAgentActionModel, len(actions))
	for index, action := range actions {
		result[index] = AiAgentActionModel{
			ID: types.StringValue(action.ID), ReferenceID: types.StringValue(action.ReferenceID),
			Name: types.StringValue(action.Name), ActionType: types.StringValue(action.ActionType),
			DestinationPhaseID: types.StringPointerValue(action.Metadata.DestinationPhaseID),
			PipeID:             types.StringPointerValue(action.Metadata.PipeID),
			Fields:             fieldsToModel(action.Metadata.Fields),
		}
	}
	return result
}

func fieldsToModel(fields []aiagentgql.Field) []AiAgentFieldModel {
	if len(fields) == 0 {
		return nil
	}
	result := make([]AiAgentFieldModel, len(fields))
	for index, field := range fields {
		result[index] = AiAgentFieldModel{
			FieldID: types.StringValue(field.FieldID), InputMode: types.StringValue(field.InputMode),
			Value: emptyAPIStringAsNull(field.Value),
		}
	}
	return result
}

// emptyAPIStringAsNull maps Pipefy NON_NULL empty strings to Terraform null so
// omitted optional attributes do not perpetual-diff after Read.
func emptyAPIStringAsNull(value *string) types.String {
	if value == nil || *value == "" {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func eventParamsToModel(params aiagentgql.EventParams) *AiAgentEventParamsModel {
	if params.ToPhaseID == nil && len(params.TriggerFieldIDs) == 0 {
		return nil
	}
	return &AiAgentEventParamsModel{
		ToPhaseID:       types.StringPointerValue(params.ToPhaseID),
		TriggerFieldIDs: stringsToSet(params.TriggerFieldIDs),
	}
}

func normalizeEmptyEventParams(model *AiAgentModel) {
	for index := range model.Behaviors {
		params := model.Behaviors[index].EventParams
		if params == nil {
			continue
		}
		// Unknown values are not "empty": defer until apply-time known values exist.
		if params.ToPhaseID.IsUnknown() || params.TriggerFieldIDs.IsUnknown() ||
			setHasUnknownElements(params.TriggerFieldIDs) {
			continue
		}
		if !hasString(params.ToPhaseID) && len(stringSetValues(params.TriggerFieldIDs)) == 0 {
			model.Behaviors[index].EventParams = nil
		}
	}
}

func setHasUnknownElements(values types.Set) bool {
	if values.IsNull() || values.IsUnknown() {
		return false
	}
	for _, element := range values.Elements() {
		value, ok := element.(types.String)
		if !ok || value.IsUnknown() {
			return true
		}
	}
	return false
}

// rematchNestedIdentities copies API ids / reference_ids from prior state by
// content identity so list insert/reorder does not inherit the wrong index.
func rematchNestedIdentities(plan *AiAgentModel, state AiAgentModel) {
	usedBehaviors := make([]bool, len(state.Behaviors))
	for behaviorIndex := range plan.Behaviors {
		planBehavior := &plan.Behaviors[behaviorIndex]
		stateBehavior, ok := matchBehavior(planBehavior, state.Behaviors, usedBehaviors)
		if !ok {
			clearNestedIdentities(planBehavior)
			continue
		}
		planBehavior.ID = stateBehavior.ID
		rematchActions(planBehavior, stateBehavior)
	}
}

func clearNestedIdentities(behavior *AiAgentBehaviorModel) {
	behavior.ID = types.StringUnknown()
	for index := range behavior.Actions {
		behavior.Actions[index].ID = types.StringUnknown()
		behavior.Actions[index].ReferenceID = types.StringUnknown()
	}
}

func rematchActions(planBehavior *AiAgentBehaviorModel, stateBehavior AiAgentBehaviorModel) {
	used := make([]bool, len(stateBehavior.Actions))
	for index := range planBehavior.Actions {
		planAction := &planBehavior.Actions[index]
		stateAction, ok := matchAction(planAction, stateBehavior.Actions, used)
		if !ok {
			planAction.ID = types.StringUnknown()
			planAction.ReferenceID = types.StringUnknown()
			continue
		}
		planAction.ID = stateAction.ID
		planAction.ReferenceID = stateAction.ReferenceID
	}
}

func matchBehavior(
	plan *AiAgentBehaviorModel,
	state []AiAgentBehaviorModel,
	used []bool,
) (AiAgentBehaviorModel, bool) {
	for index, candidate := range state {
		if used[index] {
			continue
		}
		if candidate.Name.Equal(plan.Name) && candidate.EventID.Equal(plan.EventID) {
			used[index] = true
			return candidate, true
		}
	}
	return AiAgentBehaviorModel{}, false
}

func matchAction(
	plan *AiAgentActionModel,
	state []AiAgentActionModel,
	used []bool,
) (AiAgentActionModel, bool) {
	for index, candidate := range state {
		if used[index] {
			continue
		}
		if actionIdentityEqual(*plan, candidate) {
			used[index] = true
			return candidate, true
		}
	}
	return AiAgentActionModel{}, false
}

func actionIdentityEqual(left, right AiAgentActionModel) bool {
	return left.Name.Equal(right.Name) &&
		left.ActionType.Equal(right.ActionType) &&
		optionalStringEqual(left.DestinationPhaseID, right.DestinationPhaseID) &&
		optionalStringEqual(left.PipeID, right.PipeID)
}

func optionalStringEqual(left, right types.String) bool {
	leftEmpty := !hasString(left)
	rightEmpty := !hasString(right)
	if leftEmpty || rightEmpty {
		return leftEmpty && rightEmpty
	}
	return left.ValueString() == right.ValueString()
}

func stringsToSet(values []string) types.Set {
	elements := make([]attr.Value, len(values))
	for index, value := range values {
		elements[index] = types.StringValue(value)
	}
	return types.SetValueMust(types.StringType, elements)
}

func actionReferenceStrings(actions []AiAgentActionModel) []string {
	result := make([]string, len(actions))
	for index, action := range actions {
		result[index] = action.ReferenceID.ValueString()
	}
	return result
}
