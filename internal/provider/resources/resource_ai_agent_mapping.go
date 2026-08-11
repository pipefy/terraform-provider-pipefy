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

// createdPartialState is the state written right after createAiAgent, so an
// agent that already exists stays tracked if a later step of Create fails.
// Terraform rejects unknowns in state, so this flattens the ones the API has not
// resolved yet. It builds a copy: the model the rest of Create fills has to keep
// knowing which of its values are still unresolved.
func createdPartialState(model AiAgentModel) AiAgentModel {
	partial := model
	if partial.Active.IsUnknown() {
		partial.Active = types.BoolValue(false)
	}
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

// fillFromAgent writes an API response into a plan without disturbing what the
// plan already decided. Terraform rejects an apply whose result differs from the
// plan on any known attribute, so Create and Update take from the response only
// what the plan could not know: the agent UUID, the identifiers the API assigns
// to behaviors and actions, and the status, which the caller has already verified
// against the desired one. Read keeps using applyGraphQL, because Read exists to
// detect drift and has to overwrite everything.
func (model *AiAgentModel) fillFromAgent(agent pipefy.Agent) {
	model.ID = fillUnknownString(model.ID, types.StringValue(agent.UUID))
	model.Active = types.BoolValue(agent.DisabledAt == nil)
	if model.DataSourceIDs.IsUnknown() {
		model.DataSourceIDs = stringsToSet(agent.DataSourceIDs)
	}
	fillBehaviorIdentities(model.Behaviors, behaviorsToModel(agent.Behaviors))
}

// fillBehaviorIdentities grafts the ids the API owns onto the planned behaviors.
// Pairing goes by behavior and action identity first, reusing the matchers that
// ModifyPlan already relies on, so a response listed in another order than the
// request still lands on the right entry; whatever is left over pairs by
// position, which is what a full-list replace sends and receives.
func fillBehaviorIdentities(plan, fromAPI []AiAgentBehaviorModel) {
	for index, match := range pairByIdentity(plan, fromAPI, matchBehavior) {
		behavior := &plan[index]
		behavior.ID = fillUnknownString(behavior.ID, match.ID)
		fillActionIdentities(behavior.Actions, match.Actions)
	}
}

func fillActionIdentities(plan, fromAPI []AiAgentActionModel) {
	for index, match := range pairByIdentity(plan, fromAPI, matchAction) {
		action := &plan[index]
		action.ID = fillUnknownString(action.ID, match.ID)
		action.ReferenceID = fillUnknownString(action.ReferenceID, match.ReferenceID)
	}
}

// pairByIdentity lines each planned entry up with the response entry it came
// from. Entries with no identity match take the response entry at their own
// index if that one is still free, and the zero value otherwise, which reads as
// null everywhere it is used.
func pairByIdentity[T any](
	plan, fromAPI []T,
	match func(*T, []T, []bool) (T, bool),
) []T {
	matched := make([]T, len(plan))
	used := make([]bool, len(fromAPI))
	pending := make([]int, 0, len(plan))
	for index := range plan {
		found, ok := match(&plan[index], fromAPI, used)
		if !ok {
			pending = append(pending, index)
			continue
		}
		matched[index] = found
	}
	for _, index := range pending {
		if index < len(fromAPI) && !used[index] {
			used[index] = true
			matched[index] = fromAPI[index]
		}
	}
	return matched
}

// fillUnknownString keeps a planned value and falls back to the API value only
// where the plan had none. An unmatched entry leaves null rather than unknown,
// which Terraform would reject.
func fillUnknownString(planned, fromAPI types.String) types.String {
	if !planned.IsUnknown() {
		return planned
	}
	if fromAPI.IsNull() || fromAPI.IsUnknown() {
		return types.StringNull()
	}
	return fromAPI
}

func (model *AiAgentModel) applyGraphQL(agent pipefy.Agent) {
	model.ID = types.StringValue(agent.UUID)
	model.Name = types.StringValue(agent.Name)
	model.Instruction = types.StringValue(agent.Instruction)
	model.Active = types.BoolValue(agent.DisabledAt == nil)
	model.DataSourceIDs = stringsToSet(agent.DataSourceIDs)
	model.Behaviors = behaviorsToModel(agent.Behaviors)
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
