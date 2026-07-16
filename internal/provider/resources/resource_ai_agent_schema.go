// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type AiAgentModel struct {
	ID            types.String           `tfsdk:"id"`
	PipeID        types.String           `tfsdk:"pipe_id"`
	Name          types.String           `tfsdk:"name"`
	Instruction   types.String           `tfsdk:"instruction"`
	Active        types.Bool             `tfsdk:"active"`
	DataSourceIDs types.Set              `tfsdk:"data_source_ids"`
	Behaviors     []AiAgentBehaviorModel `tfsdk:"behaviors"`
}

type AiAgentBehaviorModel struct {
	ID          types.String             `tfsdk:"id"`
	Name        types.String             `tfsdk:"name"`
	EventID     types.String             `tfsdk:"event_id"`
	Instruction types.String             `tfsdk:"instruction"`
	EventParams *AiAgentEventParamsModel `tfsdk:"event_params"`
	Actions     []AiAgentActionModel     `tfsdk:"actions"`
}

type AiAgentEventParamsModel struct {
	ToPhaseID       types.String `tfsdk:"to_phase_id"`
	TriggerFieldIDs types.Set    `tfsdk:"trigger_field_ids"`
}

type AiAgentActionModel struct {
	ID                 types.String        `tfsdk:"id"`
	ReferenceID        types.String        `tfsdk:"reference_id"`
	Name               types.String        `tfsdk:"name"`
	ActionType         types.String        `tfsdk:"action_type"`
	DestinationPhaseID types.String        `tfsdk:"destination_phase_id"`
	PipeID             types.String        `tfsdk:"pipe_id"`
	Fields             []AiAgentFieldModel `tfsdk:"fields"`
}

type AiAgentFieldModel struct {
	FieldID   types.String `tfsdk:"field_id"`
	InputMode types.String `tfsdk:"input_mode"`
	Value     types.String `tfsdk:"value"`
}

func (r *AiAgentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an AI agent and its ordered behaviors for a Pipefy pipe. " +
			"Behavior configuration is replaced in full on each update. When `active` is set, " +
			"status is applied with a separate API call after that update; if the status call fails, " +
			"the configuration change has already been applied.",
		Attributes: aiAgentAttributes(),
	}
}

func aiAgentAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed: true, Description: "The UUID of the AI agent.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"pipe_id": schema.StringAttribute{
			Required: true, Description: "The ID of the pipe that owns the AI agent.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"name":        requiredNonEmptyString("The display name of the AI agent."),
		"instruction": requiredNonEmptyString("The agent-level purpose shown as its description."),
		"active": schema.BoolAttribute{
			Optional: true, Computed: true,
			Description: "Whether the AI agent is active. Applied with a separate status API call " +
				"after create/update of the agent configuration.",
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
		},
		"data_source_ids": stringSetWithEmptyDefault(
			"Knowledge-source IDs managed as the complete unordered agent-level set.",
		),
		"behaviors": behaviorListAttribute(),
	}
}

func behaviorListAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Required: true, Description: "Ordered AI-agent behaviors, managed as a complete list.",
		Validators: []validator.List{listvalidator.SizeBetween(1, 5)},
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"id":   computedStableString("The API identifier of the behavior."),
			"name": requiredNonEmptyString("The behavior name."),
			"event_id": requiredNonEmptyString(
				"The Pipefy event ID. Current values are documented by the Pipefy API.",
			),
			"instruction":  requiredNonEmptyString("The instruction evaluated by this behavior."),
			"event_params": eventParamsAttribute(),
			"actions":      actionListAttribute(),
		}},
	}
}

func eventParamsAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true, Description: "Optional structural filters for the behavior event. " +
			"Omit the block entirely when no filters are needed; an empty block is treated as absent.",
		Attributes: map[string]schema.Attribute{
			"to_phase_id": schema.StringAttribute{
				Optional: true, Description: "Destination phase filter for the event.",
			},
			"trigger_field_ids": schema.SetAttribute{
				Optional: true, Description: "Field IDs that trigger the behavior event, " +
					"managed as an unordered set.",
				ElementType: types.StringType,
			},
		},
	}
}

func actionListAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Required: true, Description: "Ordered actions available to the behavior.",
		Validators: []validator.List{listvalidator.SizeAtLeast(1)},
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"id":           computedStableString("The API identifier of the action."),
			"reference_id": computedStableString("The stable reference UUID sent to Pipefy."),
			"name":         requiredNonEmptyString("The action name."),
			"action_type": requiredNonEmptyString(
				"The AI behavior action type. Supported values are defined by Pipefy; " +
					"this provider currently manages a subset of those values. " +
					"See the Pipefy API reference (https://developers.pipefy.com/reference).",
			),
			"destination_phase_id": schema.StringAttribute{
				Optional: true, Description: "Destination phase required by move_card.",
			},
			"pipe_id": schema.StringAttribute{
				Optional: true, Description: "Target pipe required by update_card and create_card.",
			},
			"fields": fieldListAttribute(),
		}},
	}
}

func fieldListAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional: true, Description: "Ordered target field metadata for card actions.",
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"field_id": requiredNonEmptyString("The target field ID."),
			"input_mode": requiredNonEmptyString(
				"How Pipefy supplies the field value. Supported values are defined by Pipefy; " +
					"see the Pipefy API reference (https://developers.pipefy.com/reference).",
			),
			"value": schema.StringAttribute{
				Optional: true, Description: "Optional fixed value or source-field reference.",
			},
		}},
	}
}

func requiredNonEmptyString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Required: true, Description: description,
		Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
	}
}

func computedStableString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed: true, Description: description,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func stringSetWithEmptyDefault(description string) schema.SetAttribute {
	empty := types.SetValueMust(types.StringType, []attr.Value{})
	return schema.SetAttribute{
		Optional: true, Computed: true, Description: description, ElementType: types.StringType,
		Default: setdefault.StaticValue(empty),
	}
}

func (r *AiAgentResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var model AiAgentModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for behaviorIndex, behavior := range model.Behaviors {
		for actionIndex, action := range behavior.Actions {
			err := actionShapeError(behaviorIndex, actionIndex, action)
			if err == nil {
				continue
			}
			actionPath := path.Root("behaviors").AtListIndex(behaviorIndex).
				AtName("actions").AtListIndex(actionIndex)
			resp.Diagnostics.AddAttributeError(actionPath, "Invalid AI agent action", err.Error())
		}
	}
}

func (r *AiAgentResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan AiAgentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	normalizeEmptyEventParams(&plan)
	ensureComputedUnknowns(&plan)
	if !req.State.Raw.IsNull() {
		var state AiAgentModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		rematchNestedIdentities(&plan, state)
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func ensureComputedUnknowns(model *AiAgentModel) {
	for behaviorIndex := range model.Behaviors {
		behavior := &model.Behaviors[behaviorIndex]
		if behavior.ID.IsNull() {
			behavior.ID = types.StringUnknown()
		}
		for actionIndex := range behavior.Actions {
			action := &behavior.Actions[actionIndex]
			if action.ID.IsNull() {
				action.ID = types.StringUnknown()
			}
			if action.ReferenceID.IsNull() {
				action.ReferenceID = types.StringUnknown()
			}
		}
	}
}

func actionShapeError(behaviorIndex, actionIndex int, action AiAgentActionModel) error {
	if action.ActionType.IsUnknown() || action.ActionType.IsNull() {
		return nil
	}
	prefix := fmt.Sprintf(
		"behavior[%d].actions[%d] action_type %q",
		behaviorIndex, actionIndex, action.ActionType.ValueString(),
	)
	switch action.ActionType.ValueString() {
	case "move_card":
		return validateMoveCardShape(prefix, action)
	case "update_card", "create_card":
		return validateCardFieldsShape(prefix, action)
	default:
		return fmt.Errorf(
			"%s; expected one of move_card, update_card, create_card", prefix,
		)
	}
}

func validateMoveCardShape(prefix string, action AiAgentActionModel) error {
	if action.DestinationPhaseID.IsUnknown() {
		return nil
	}
	if !hasString(action.DestinationPhaseID) {
		return fmt.Errorf("%s requires destination_phase_id", prefix)
	}
	if hasString(action.PipeID) || len(action.Fields) > 0 {
		return fmt.Errorf("%s accepts destination_phase_id but not pipe_id or fields", prefix)
	}
	return nil
}

func validateCardFieldsShape(prefix string, action AiAgentActionModel) error {
	if action.PipeID.IsUnknown() {
		return nil
	}
	if !hasString(action.PipeID) || len(action.Fields) == 0 {
		return fmt.Errorf("%s requires pipe_id and at least one field", prefix)
	}
	if hasString(action.DestinationPhaseID) {
		return fmt.Errorf("%s does not accept destination_phase_id", prefix)
	}
	return nil
}

func hasString(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueString() != ""
}
