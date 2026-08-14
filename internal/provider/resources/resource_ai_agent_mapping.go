// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
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

// createdPartialState flattens unknowns Terraform rejects in mid-create state
// and returns a copy so the live model still tracks unresolved ids.
func createdPartialState(model AiAgentModel) AiAgentModel {
	partial := model
	partial.Behaviors = make([]AiAgentBehaviorModel, len(model.Behaviors))
	for behaviorIndex, behavior := range model.Behaviors {
		behavior.ID = knownOrNull(behavior.ID)
		actions := make([]AiAgentActionModel, len(behavior.Actions))
		for actionIndex, action := range behavior.Actions {
			action.ID = knownOrNull(action.ID)
			actions[actionIndex] = action
		}
		behavior.Actions = actions
		partial.Behaviors[behaviorIndex] = behavior
	}
	return partial
}

func knownOrNull(value types.String) types.String {
	if value.IsUnknown() {
		return types.StringNull()
	}
	return value
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

func (model AiAgentModel) graphQLInput(repoUUID string, keepAliveIndex int) map[string]any {
	input := map[string]any{
		"name":          model.Name.ValueString(),
		"instruction":   model.Instruction.ValueString(),
		"repoUuid":      repoUUID,
		"dataSourceIds": stringSetValues(model.DataSourceIDs),
	}
	behaviors := make([]map[string]any, len(model.Behaviors))
	for index, behavior := range model.Behaviors {
		var active *bool
		if index == keepAliveIndex {
			value := true
			active = &value
		}
		behaviors[index] = behavior.graphQLInput(active)
	}
	input["behaviors"] = behaviors
	return input
}

const omitBehaviorActive = -1

// keepAliveBehaviorIndex is the planned behavior that already reports active
// on the last read. One true is enough for updateAiAgent to leave the agent
// on; the rest omit the flag so stored enablement is not overwritten.
// A disabled agent reports every behavior inactive, so that read is ignored.
func keepAliveBehaviorIndex(plan []AiAgentBehaviorModel, current pipefy.Agent) int {
	if current.DisabledAt != nil {
		return omitBehaviorActive
	}
	used := make([]bool, len(current.Behaviors))
	for index, planned := range plan {
		for apiIndex, candidate := range current.Behaviors {
			if used[apiIndex] {
				continue
			}
			if candidate.Name != planned.Name.ValueString() ||
				candidate.EventID != planned.EventID.ValueString() {
				continue
			}
			used[apiIndex] = true
			if candidate.Active {
				return index
			}
			break
		}
	}
	return omitBehaviorActive
}

func (behavior AiAgentBehaviorModel) graphQLInput(active *bool) map[string]any {
	input := map[string]any{
		"name": behavior.Name.ValueString(), "eventId": behavior.EventID.ValueString(),
	}
	// Omit active unless this is the keep-alive signal. A false persists off
	// on the automation and survives a later updateAiAgentStatus(true).
	if active != nil {
		input["active"] = *active
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

func behaviorsToModel(behaviors []pipefy.Behavior) []AiAgentBehaviorModel {
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

func actionsToModel(actions []pipefy.Action) []AiAgentActionModel {
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

func fieldsToModel(fields []pipefy.AgentField) []AiAgentFieldModel {
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

func eventParamsToModel(params pipefy.AgentEventParams) *AiAgentEventParamsModel {
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
