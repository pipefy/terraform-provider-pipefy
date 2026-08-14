// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
)

// fillFromAgent fills unknowns and grafts API-owned ids; planned values stay.
// Read keeps applyGraphQL so drift detection still overwrites from the API.
func (model *AiAgentModel) fillFromAgent(agent pipefy.Agent) error {
	model.ID = fillUnknownString(model.ID, types.StringValue(agent.UUID))
	model.Active = types.BoolValue(agent.DisabledAt == nil)
	return fillBehaviorIdentities(model.Behaviors, behaviorsToModel(agent.Behaviors))
}

// fillBehaviorIdentities grafts API ids onto planned behaviors by name and
// event. A same-length miss falls back to position so apply never stores an
// unknown id; a remaining miss is an error naming the behavior.
func fillBehaviorIdentities(plan, fromAPI []AiAgentBehaviorModel) error {
	used := make([]bool, len(fromAPI))
	for index := range plan {
		behavior := &plan[index]
		match, ok := matchBehavior(behavior, fromAPI, used)
		if !ok {
			match, ok = matchByIndex(index, fromAPI, used, len(plan))
		}
		if !ok {
			return fmt.Errorf(
				"AI agent response did not include behavior %q with event_id %q",
				behavior.Name.ValueString(), behavior.EventID.ValueString(),
			)
		}
		behavior.ID = fillUnknownString(behavior.ID, match.ID)
		if err := fillActionIdentities(behavior.Actions, match.Actions); err != nil {
			return err
		}
	}
	return nil
}

func fillActionIdentities(plan, fromAPI []AiAgentActionModel) error {
	used := make([]bool, len(fromAPI))
	for index := range plan {
		action := &plan[index]
		match, ok := matchActionByReference(action, fromAPI, used)
		if !ok {
			match, ok = matchAction(action, fromAPI, used)
		}
		if !ok {
			match, ok = matchByIndex(index, fromAPI, used, len(plan))
		}
		if !ok {
			return fmt.Errorf(
				"AI agent response did not include action %q with action_type %q",
				action.Name.ValueString(), action.ActionType.ValueString(),
			)
		}
		action.ID = fillUnknownString(action.ID, match.ID)
		action.ReferenceID = fillUnknownString(action.ReferenceID, match.ReferenceID)
	}
	return nil
}

func matchByIndex[T any](index int, fromAPI []T, used []bool, planLen int) (T, bool) {
	var zero T
	if planLen != len(fromAPI) || index >= len(fromAPI) || used[index] {
		return zero, false
	}
	used[index] = true
	return fromAPI[index], true
}

func (model *AiAgentModel) applyGraphQL(agent pipefy.Agent) {
	model.ID = types.StringValue(agent.UUID)
	model.Name = types.StringValue(agent.Name)
	model.Instruction = types.StringValue(agent.Instruction)
	model.Active = types.BoolValue(agent.DisabledAt == nil)
	model.DataSourceIDs = stringsToSet(agent.DataSourceIDs)
	model.Behaviors = alignBehaviors(model.Behaviors, behaviorsToModel(agent.Behaviors))
}

// alignBehaviors reorders the API list to the configured identity order.
// Pipefy stores and returns behaviors in creation order, which would
// otherwise perpetual-diff after an insert or swap.
func alignBehaviors(state, fromAPI []AiAgentBehaviorModel) []AiAgentBehaviorModel {
	used := make([]bool, len(fromAPI))
	aligned := make([]AiAgentBehaviorModel, 0, len(fromAPI))
	for index := range state {
		match, ok := matchBehavior(&state[index], fromAPI, used)
		if !ok {
			continue
		}
		match.Actions = alignActions(state[index].Actions, match.Actions)
		aligned = append(aligned, match)
	}
	for index, candidate := range fromAPI {
		if !used[index] {
			aligned = append(aligned, candidate)
		}
	}
	return aligned
}

func alignActions(state, fromAPI []AiAgentActionModel) []AiAgentActionModel {
	used := make([]bool, len(fromAPI))
	aligned := make([]AiAgentActionModel, 0, len(fromAPI))
	for index := range state {
		match, ok := matchActionByReference(&state[index], fromAPI, used)
		if !ok {
			match, ok = matchAction(&state[index], fromAPI, used)
		}
		if ok {
			aligned = append(aligned, match)
		}
	}
	for index, candidate := range fromAPI {
		if !used[index] {
			aligned = append(aligned, candidate)
		}
	}
	return aligned
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

func matchActionByReference(
	plan *AiAgentActionModel,
	state []AiAgentActionModel,
	used []bool,
) (AiAgentActionModel, bool) {
	if !hasString(plan.ReferenceID) {
		return AiAgentActionModel{}, false
	}
	want := plan.ReferenceID.ValueString()
	for index, candidate := range state {
		if used[index] || candidate.ReferenceID.ValueString() != want {
			continue
		}
		used[index] = true
		return candidate, true
	}
	return AiAgentActionModel{}, false
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
