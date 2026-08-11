// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
)

var _ resource.Resource = &AiAgentResource{}
var _ resource.ResourceWithImportState = &AiAgentResource{}
var _ resource.ResourceWithValidateConfig = &AiAgentResource{}
var _ resource.ResourceWithModifyPlan = &AiAgentResource{}

type AiAgentResource struct {
	api *pipefy.Client
}

func NewAiAgentResource() resource.Resource {
	return &AiAgentResource{}
}

func (r *AiAgentResource) Metadata(
	ctx context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_ai_agent"
}

func (r *AiAgentResource) Configure(
	ctx context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*pipefy.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data", fmt.Sprintf("expected *pipefy.Client, got %T", req.ProviderData),
		)
		return
	}
	r.api = api
}

func (r *AiAgentResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	model, configuredActive, ok := loadCreateModel(ctx, req, resp)
	if !ok {
		return
	}
	repoUUID, err := r.api.Pipes.UUID(ctx, model.PipeID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("create AI agent failed", err.Error())
		return
	}
	if err := ensureActionReferenceIDs(&model); err != nil {
		resp.Diagnostics.AddError("create AI agent failed", "generate action reference IDs: "+err.Error())
		return
	}
	if err := r.createAgent(ctx, &model, repoUUID, configuredActive); err != nil {
		resp.Diagnostics.AddError("create AI agent failed", err.Error())
		return
	}
	prepareCreatedPartialState(&model)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		r.rollbackCreate(ctx, model.ID.ValueString(), fmt.Errorf("persist created agent state"), resp)
		return
	}
	r.finishCreate(ctx, &model, configuredActive, resp)
}

func loadCreateModel(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) (AiAgentModel, types.Bool, bool) {
	var model AiAgentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	var configuredActive types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("active"), &configuredActive)...)
	return model, configuredActive, !resp.Diagnostics.HasError()
}

// createAgent creates the agent disabled or enabled as configured. createAiAgent
// always returns a disabled agent, but it honours an explicit disabledAt, so a
// configuration that wants the agent off says so in the payload and needs no
// status mutation afterwards.
func (r *AiAgentResource) createAgent(
	ctx context.Context,
	model *AiAgentModel,
	repoUUID string,
	configuredActive types.Bool,
) error {
	input := model.graphQLInput(repoUUID)
	if wantsInactive(configuredActive) {
		input["disabledAt"] = disabledAtNow()
	}
	uuid, err := r.api.AiAgents.Create(ctx, input)
	if err != nil {
		return err
	}
	model.ID = types.StringValue(uuid)
	return nil
}

// finishCreate switches the agent on when configured active, then verifies the
// status the API actually reports before writing state. Status stays a separate
// mutation because createAiAgent does not accept the active flag.
func (r *AiAgentResource) finishCreate(
	ctx context.Context,
	model *AiAgentModel,
	configuredActive types.Bool,
	resp *resource.CreateResponse,
) {
	if wantsActive(configuredActive) {
		if err := r.updateStatus(ctx, model.ID.ValueString(), true); err != nil {
			r.rollbackCreate(ctx, model.ID.ValueString(), err, resp)
			return
		}
	}
	agent, err := r.verifiedAgent(ctx, model.ID.ValueString(), configuredActive)
	if err != nil {
		r.rollbackCreate(ctx, model.ID.ValueString(), err, resp)
		return
	}
	model.applyGraphQL(*agent)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
	if resp.Diagnostics.HasError() {
		r.rollbackCreate(ctx, model.ID.ValueString(), fmt.Errorf("persist created agent state"), resp)
	}
}

func (r *AiAgentResource) rollbackCreate(
	ctx context.Context,
	agentUUID string,
	operationErr error,
	resp *resource.CreateResponse,
) {
	rollbackErr := r.deleteAgent(ctx, agentUUID)
	if rollbackErr == nil {
		resp.State.RemoveResource(ctx)
		resp.Diagnostics.AddError("create AI agent failed", operationErr.Error())
		return
	}
	resp.Diagnostics.AddError(
		"create AI agent failed and rollback failed",
		fmt.Sprintf(
			"agent %q is orphaned: operation failed: %v; rollback failed: %v",
			agentUUID, operationErr, rollbackErr,
		),
	)
}

func (r *AiAgentResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var model AiAgentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() || !hasString(model.ID) {
		return
	}
	agent, err := r.fetchAgent(ctx, model.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("read AI agent failed", err.Error())
		return
	}
	if agent == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	if err := r.verifyPipeOwnsAgent(ctx, model.PipeID.ValueString(), *agent); err != nil {
		resp.Diagnostics.AddError("read AI agent failed", err.Error())
		return
	}
	model.applyGraphQL(*agent)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *AiAgentResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan AiAgentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	repoUUID, err := r.api.Pipes.UUID(ctx, plan.PipeID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("update AI agent failed", err.Error())
		return
	}
	if err := ensureActionReferenceIDs(&plan); err != nil {
		resp.Diagnostics.AddError("update AI agent failed", "generate action reference IDs: "+err.Error())
		return
	}
	r.applyUpdate(ctx, &plan, repoUUID, resp)
}

// applyUpdate writes the configuration and then makes the agent's status match
// the plan. updateAiAgent disables the agent on every call, so the desired status
// drives the whole sequence: an agent that should stay off carries its own
// disabledAt in the payload, and one that should stay on is switched back on
// afterwards. The plan's active value, not the prior state, is what is enforced,
// because the update disables the agent whether or not the status changed.
func (r *AiAgentResource) applyUpdate(
	ctx context.Context,
	plan *AiAgentModel,
	repoUUID string,
	resp *resource.UpdateResponse,
) {
	desired := plan.Active
	input, err := r.updateInput(ctx, *plan, repoUUID, desired)
	if err != nil {
		resp.Diagnostics.AddError("update AI agent failed", err.Error())
		return
	}
	if err := r.api.AiAgents.Update(ctx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("update AI agent failed", err.Error())
		return
	}
	if wantsActive(desired) {
		if err := r.updateStatus(ctx, plan.ID.ValueString(), true); err != nil {
			r.reportStatusFailure(ctx, plan, err, resp)
			return
		}
	}
	agent, err := r.requireAgent(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("read AI agent after update failed", err.Error())
		return
	}
	agent, err = r.enforceStatus(ctx, plan.ID.ValueString(), desired, agent)
	if err != nil {
		r.reportStatusFailure(ctx, plan, err, resp)
		return
	}
	plan.applyGraphQL(*agent)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// updateInput builds the updateAiAgent payload. Re-sending the current
// disabledAt is the only way to keep a disabled agent disabled without the
// update stamping a fresh timestamp; an agent that should end up active omits it
// and is switched on by the status mutation that follows.
func (r *AiAgentResource) updateInput(
	ctx context.Context,
	model AiAgentModel,
	repoUUID string,
	desired types.Bool,
) (map[string]any, error) {
	input := model.graphQLInput(repoUUID)
	if !wantsInactive(desired) {
		return input, nil
	}
	current, err := r.fetchAgent(ctx, model.ID.ValueString())
	if err != nil {
		return nil, err
	}
	if current != nil && current.DisabledAt != nil {
		input["disabledAt"] = *current.DisabledAt
		return input, nil
	}
	input["disabledAt"] = disabledAtNow()
	return input, nil
}

// verifiedAgent reads the agent back and makes its real status match the desired
// one. Every write path ends here rather than trusting the mutation it just sent.
func (r *AiAgentResource) verifiedAgent(
	ctx context.Context,
	id string,
	desired types.Bool,
) (*pipefy.Agent, error) {
	agent, err := r.requireAgent(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.enforceStatus(ctx, id, desired, agent)
}

// enforceStatus corrects a status that came back different from the desired one
// and fails loudly if the correction did not take, so state never claims a status
// the API does not report.
func (r *AiAgentResource) enforceStatus(
	ctx context.Context,
	id string,
	desired types.Bool,
	agent *pipefy.Agent,
) (*pipefy.Agent, error) {
	if !isConfiguredBool(desired) || agentIsActive(*agent) == desired.ValueBool() {
		return agent, nil
	}
	if err := r.updateStatus(ctx, id, desired.ValueBool()); err != nil {
		return nil, err
	}
	corrected, err := r.requireAgent(ctx, id)
	if err != nil {
		return nil, err
	}
	if agentIsActive(*corrected) != desired.ValueBool() {
		return nil, fmt.Errorf(
			"AI agent %q reports active=%t after applying the configured active=%t",
			id, agentIsActive(*corrected), desired.ValueBool(),
		)
	}
	return corrected, nil
}

// reportStatusFailure persists the remote configuration before failing, so the
// next apply only has the status left to retry.
func (r *AiAgentResource) reportStatusFailure(
	ctx context.Context,
	plan *AiAgentModel,
	statusErr error,
	resp *resource.UpdateResponse,
) {
	r.refreshStateAfterPartialUpdate(ctx, plan, resp)
	resp.Diagnostics.AddError("update AI agent status failed", statusErr.Error())
}

func (r *AiAgentResource) requireAgent(ctx context.Context, id string) (*pipefy.Agent, error) {
	agent, err := r.fetchAgent(ctx, id)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, fmt.Errorf("aiAgent %q returned no agent", id)
	}
	return agent, nil
}

// refreshStateAfterPartialUpdate persists the remote config after updateAiAgent
// succeeded but a later status call failed, so the next apply only retries status.
func (r *AiAgentResource) refreshStateAfterPartialUpdate(
	ctx context.Context,
	plan *AiAgentModel,
	resp *resource.UpdateResponse,
) {
	agent, err := r.fetchAgent(ctx, plan.ID.ValueString())
	if err != nil || agent == nil {
		return
	}
	plan.applyGraphQL(*agent)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AiAgentResource) verifyPipeOwnsAgent(
	ctx context.Context,
	pipeID string,
	agent pipefy.Agent,
) error {
	repoUUID, err := r.api.Pipes.UUID(ctx, pipeID)
	if err != nil {
		return err
	}
	if agent.RepoUUID == "" || agent.RepoUUID == repoUUID {
		return nil
	}
	return fmt.Errorf(
		"pipe_id %q resolves to UUID %q but AI agent %q belongs to repo UUID %q",
		pipeID, repoUUID, agent.UUID, agent.RepoUUID,
	)
}

func (r *AiAgentResource) updateStatus(ctx context.Context, id string, active bool) error {
	return r.api.AiAgents.UpdateStatus(ctx, id, active)
}

// fetchAgent maps ErrNotFound to a nil agent, which is the contract its five
// call sites are written against.
func (r *AiAgentResource) fetchAgent(
	ctx context.Context,
	id string,
) (*pipefy.Agent, error) {
	agent, err := r.api.AiAgents.Get(ctx, id)
	if errors.Is(err, pipefy.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func (r *AiAgentResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var model AiAgentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.deleteAgent(ctx, model.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete AI agent failed", err.Error())
	}
}

func (r *AiAgentResource) deleteAgent(ctx context.Context, id string) error {
	return r.api.AiAgents.Delete(ctx, id)
}

func (r *AiAgentResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	parts, ok := splitImportID(req.ID)
	if !ok || len(parts) != 2 {
		resp.Diagnostics.AddError(
			"invalid import ID",
			fmt.Sprintf("got %q; expected pipe_id/agent_uuid", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pipe_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func isConfiguredBool(value types.Bool) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func wantsActive(value types.Bool) bool {
	return isConfiguredBool(value) && value.ValueBool()
}

func wantsInactive(value types.Bool) bool {
	return isConfiguredBool(value) && !value.ValueBool()
}

func agentIsActive(agent pipefy.Agent) bool {
	return agent.DisabledAt == nil
}

func disabledAtNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
