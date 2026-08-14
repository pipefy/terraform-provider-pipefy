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
	var model AiAgentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
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
	if err := r.createAgent(ctx, &model, repoUUID); err != nil {
		resp.Diagnostics.AddError("create AI agent failed", err.Error())
		return
	}
	partial := createdPartialState(model)
	resp.Diagnostics.Append(resp.State.Set(ctx, &partial)...)
	if resp.Diagnostics.HasError() {
		r.rollbackCreate(ctx, model.ID.ValueString(), fmt.Errorf("persist created agent state"), resp)
		return
	}
	r.finishCreate(ctx, &model, resp)
}

// createAgent sets disabledAt so the happy path needs no follow-up status call.
func (r *AiAgentResource) createAgent(
	ctx context.Context,
	model *AiAgentModel,
	repoUUID string,
) error {
	input := model.graphQLInput(repoUUID, omitBehaviorActive)
	if model.Active.ValueBool() {
		input["disabledAt"] = nil
	} else {
		input["disabledAt"] = disabledAtNow()
	}
	uuid, err := r.api.AiAgents.Create(ctx, input)
	if err != nil {
		return err
	}
	model.ID = types.StringValue(uuid)
	return nil
}

// A status mismatch fails the apply and leaves the agent in state; deleting it
// here would destroy a resource the create mutation already succeeded on.
func (r *AiAgentResource) finishCreate(
	ctx context.Context,
	model *AiAgentModel,
	resp *resource.CreateResponse,
) {
	agent, err := r.requireAgent(ctx, model.ID.ValueString())
	if err != nil {
		r.rollbackCreate(ctx, model.ID.ValueString(), err, resp)
		return
	}
	agent, err = r.enforceStatus(ctx, model.ID.ValueString(), model.Active, agent)
	if err != nil {
		resp.Diagnostics.AddError("create AI agent failed", err.Error())
		r.persistCreated(ctx, model, agent, resp)
		return
	}
	if err := model.fillFromAgent(*agent); err != nil {
		resp.Diagnostics.AddError("create AI agent failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
	if resp.Diagnostics.HasError() {
		r.rollbackCreate(ctx, model.ID.ValueString(), fmt.Errorf("persist created agent state"), resp)
	}
}

func (r *AiAgentResource) persistCreated(
	ctx context.Context,
	model *AiAgentModel,
	agent *pipefy.Agent,
	resp *resource.CreateResponse,
) {
	if err := model.fillFromAgent(*agent); err != nil {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
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

// applyUpdate enforces the planned status, not the change between config and
// prior state. updateAiAgent keeps the agent on when one behavior is active.
func (r *AiAgentResource) applyUpdate(
	ctx context.Context,
	plan *AiAgentModel,
	repoUUID string,
	resp *resource.UpdateResponse,
) {
	desired := plan.Active
	current, err := r.fetchAgent(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("read AI agent before update failed", err.Error())
		return
	}
	if current == nil {
		resp.State.RemoveResource(ctx)
		resp.Diagnostics.AddError(
			"read AI agent before update failed",
			fmt.Sprintf("AI agent %q no longer exists", plan.ID.ValueString()),
		)
		return
	}
	input := updateInput(*plan, repoUUID, desired, *current)
	if err := r.api.AiAgents.Update(ctx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("update AI agent failed", err.Error())
		return
	}
	agent, err := r.requireAgent(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("read AI agent after update failed", err.Error())
		return
	}
	agent, err = r.enforceStatus(ctx, plan.ID.ValueString(), desired, agent)
	if err != nil {
		resp.Diagnostics.AddError("update AI agent status failed", err.Error())
		r.refreshStateAfterPartialUpdate(ctx, plan, agent, resp)
		return
	}
	if err := plan.fillFromAgent(*agent); err != nil {
		resp.Diagnostics.AddError("update AI agent failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// updateInput sends active:true on one already-active behavior when the plan
// wants the agent on, and re-sends disabledAt (never behavior active=false)
// when it wants the agent off.
func updateInput(
	model AiAgentModel,
	repoUUID string,
	desired types.Bool,
	current pipefy.Agent,
) map[string]any {
	keepAlive := omitBehaviorActive
	if desired.ValueBool() {
		keepAlive = keepAliveBehaviorIndex(model.Behaviors, current)
	}
	input := model.graphQLInput(repoUUID, keepAlive)
	if desired.ValueBool() {
		return input
	}
	if current.DisabledAt != nil {
		input["disabledAt"] = *current.DisabledAt
		return input
	}
	input["disabledAt"] = disabledAtNow()
	return input
}

// enforceStatus corrects a mismatch once so state never claims a status the
// remote does not report.
func (r *AiAgentResource) enforceStatus(
	ctx context.Context,
	id string,
	desired types.Bool,
	agent *pipefy.Agent,
) (*pipefy.Agent, error) {
	if agentIsActive(*agent) == desired.ValueBool() {
		return agent, nil
	}
	if err := r.updateStatus(ctx, id, desired.ValueBool()); err != nil {
		return agent, err
	}
	corrected, err := r.requireAgent(ctx, id)
	if err != nil {
		return agent, err
	}
	if agentIsActive(*corrected) != desired.ValueBool() {
		return corrected, fmt.Errorf(
			"AI agent %q reports active=%t after applying the planned active=%t",
			id, agentIsActive(*corrected), desired.ValueBool(),
		)
	}
	return corrected, nil
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

// refreshStateAfterPartialUpdate persists planned values plus grafted ids after
// updateAiAgent succeeded but a later status call failed, so the next apply
// only retries status and does not revert Required attributes.
func (r *AiAgentResource) refreshStateAfterPartialUpdate(
	ctx context.Context,
	plan *AiAgentModel,
	agent *pipefy.Agent,
	resp *resource.UpdateResponse,
) {
	if err := plan.fillFromAgent(*agent); err != nil {
		return
	}
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

// fetchAgent maps ErrNotFound to a nil agent.
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

func agentIsActive(agent pipefy.Agent) bool {
	return agent.DisabledAt == nil
}

func disabledAtNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}
