// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
)

// TestApplyFieldConditionToModelCondition covers what a field condition's
// required condition settles to when the API reports one the provider would
// never write. A null condition has to become an empty one rather than a
// missing one, and a payload no condition can express has to fail the read
// instead of leaving a half-built condition in state.
func TestApplyFieldConditionToModelCondition(t *testing.T) {
	base := func() *pipefy.FieldCondition {
		return &pipefy.FieldCondition{
			ID:    "fc_1",
			Name:  "rule",
			Phase: &pipefy.FieldConditionPhase{ID: "phase_1", RepoID: 123},
			Actions: []pipefy.FieldConditionAction{
				{ActionID: "show", PhaseField: &pipefy.FieldConditionPhaseField{InternalID: "1002"}, WhenEvaluator: boolValue(true)},
			},
		}
	}

	t.Run("no condition becomes an empty one", func(t *testing.T) {
		for name, fc := range map[string]*pipefy.FieldCondition{
			"null condition": base(),
			"no expressions": func() *pipefy.FieldCondition {
				f := base()
				f.Condition = &pipefy.Condition{}
				return f
			}(),
		} {
			t.Run(name, func(t *testing.T) {
				var data FieldConditionModel
				var diags diag.Diagnostics
				applyFieldConditionToModel(&data, fc, false, &diags)
				if diags.HasError() {
					t.Fatalf("unexpected diagnostics: %v", diags)
				}
				if data.Condition == nil {
					t.Fatal("condition is required, so it must never settle to null")
				}
				if len(data.Condition.AllOf) != 0 || len(data.Condition.AnyOf) != 0 {
					t.Fatalf("expected an empty condition, got %#v", data.Condition)
				}
			})
		}
	})

	t.Run("an unreconstructable condition fails the read", func(t *testing.T) {
		fc := base()
		fc.Condition = &pipefy.Condition{
			Expressions:          []pipefy.ConditionExpression{{StructureID: "0", FieldAddress: "1001", Operation: "equals"}},
			ExpressionsStructure: [][]any{{"7"}},
		}
		var data FieldConditionModel
		var diags diag.Diagnostics
		applyFieldConditionToModel(&data, fc, false, &diags)
		if !diags.HasError() {
			t.Fatal("expected an error for a structure_id no expression carries")
		}
		if data.Condition != nil {
			t.Fatalf("expected no condition alongside the error, got %#v", data.Condition)
		}
	})

	t.Run("onlyUnknown keeps planned name and actions", func(t *testing.T) {
		fc := base()
		fc.Name = "renamed by the API"
		fc.Phase = &pipefy.FieldConditionPhase{ID: "phase_2"}
		fc.Actions = []pipefy.FieldConditionAction{
			{ActionID: "hide", PhaseField: &pipefy.FieldConditionPhaseField{InternalID: "9999"}, WhenEvaluator: boolValue(false)},
		}

		data := FieldConditionModel{
			Id:      types.StringUnknown(),
			PipeId:  types.StringValue("123"),
			PhaseId: types.StringValue("phase_1"),
			Name:    types.StringValue("rule"),
			Actions: []fieldConditionActionModel{{
				Field:     types.StringValue("1002"),
				WhenTrue:  types.StringValue("show"),
				WhenFalse: types.StringNull(),
			}},
		}
		var diags diag.Diagnostics
		applyFieldConditionToModel(&data, fc, true, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if data.Id.ValueString() != "fc_1" {
			t.Fatalf("expected the unknown id to be filled from the response, got %#v", data.Id)
		}
		if data.Name.ValueString() != "rule" {
			t.Fatalf("expected the planned name to survive the write, got %q", data.Name.ValueString())
		}
		if data.PhaseId.ValueString() != "phase_1" {
			t.Fatalf("expected the planned phase_id to survive the write, got %q", data.PhaseId.ValueString())
		}
		if len(data.Actions) != 1 || data.Actions[0].Field.ValueString() != "1002" {
			t.Fatalf("expected the planned actions to survive the write, got %#v", data.Actions)
		}
	})

	t.Run("onlyUnknown fills unknown phase_id and pipe_id", func(t *testing.T) {
		fc := base()
		data := FieldConditionModel{
			Id:      types.StringUnknown(),
			PipeId:  types.StringUnknown(),
			PhaseId: types.StringUnknown(),
			Name:    types.StringValue("rule"),
			Actions: []fieldConditionActionModel{{
				Field:     types.StringValue("1002"),
				WhenTrue:  types.StringValue("show"),
				WhenFalse: types.StringNull(),
			}},
		}
		var diags diag.Diagnostics
		applyFieldConditionToModel(&data, fc, true, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if data.PhaseId.ValueString() != "phase_1" {
			t.Fatalf("expected the unknown phase_id to be filled from the response, got %#v", data.PhaseId)
		}
		if data.PipeId.ValueString() != "123" {
			t.Fatalf("expected the unknown pipe_id to be filled from the response, got %#v", data.PipeId)
		}
		if data.Name.ValueString() != "rule" {
			t.Fatalf("expected the planned name to survive the write, got %q", data.Name.ValueString())
		}
	})
}

func boolValue(b bool) *bool { return &b }
