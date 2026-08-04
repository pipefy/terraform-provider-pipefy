// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/conditiongql"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/fieldconditiongql"
)

// TestApplyFieldConditionToModelCondition covers what a field condition's
// required condition settles to when the API reports one the provider would
// never write. A null condition has to become an empty one rather than a
// missing one, and a payload no condition can express has to fail the read
// instead of leaving a half-built condition in state.
func TestApplyFieldConditionToModelCondition(t *testing.T) {
	base := func() *fieldconditiongql.FieldCondition {
		return &fieldconditiongql.FieldCondition{
			Id:    "fc_1",
			Name:  "rule",
			Phase: &fieldconditiongql.Phase{Id: "phase_1"},
			Actions: []fieldconditiongql.Action{
				{ActionId: "show", PhaseField: &fieldconditiongql.PhaseField{InternalId: "1002"}, WhenEvaluator: boolValue(true)},
			},
		}
	}

	t.Run("no condition becomes an empty one", func(t *testing.T) {
		for name, fc := range map[string]*fieldconditiongql.FieldCondition{
			"null condition": base(),
			"no expressions": func() *fieldconditiongql.FieldCondition {
				f := base()
				f.Condition = &conditiongql.Condition{}
				return f
			}(),
		} {
			t.Run(name, func(t *testing.T) {
				var data FieldConditionModel
				var diags diag.Diagnostics
				applyFieldConditionToModel(&data, fc, &diags)
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
		fc.Condition = &conditiongql.Condition{
			Expressions:          []conditiongql.Expression{{StructureId: "0", FieldAddress: "1001", Operation: "equals"}},
			ExpressionsStructure: [][]any{{"7"}},
		}
		var data FieldConditionModel
		var diags diag.Diagnostics
		applyFieldConditionToModel(&data, fc, &diags)
		if !diags.HasError() {
			t.Fatal("expected an error for a structure_id no expression carries")
		}
		if data.Condition != nil {
			t.Fatalf("expected no condition alongside the error, got %#v", data.Condition)
		}
	})
}

func boolValue(b bool) *bool { return &b }
