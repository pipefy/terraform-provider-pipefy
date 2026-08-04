// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"errors"
	"fmt"

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
	if err := r.createAgent(ctx, &model, repoUUID); err != nil {
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

func (r *AiAgentResource) createAgent(
	ctx context.Context,
	model *AiAgentModel,
	repoUUID string,
) error {
	uuid, err := r.api.AiAgents.Create(ctx, model.graphQLInput(repoUUID))
	if err != nil {
		return err
	}
	model.ID = types.StringValue(uuid)
	return nil
}

// finishCreate applies optional status then refreshes state. Status stays a
// separate mutation because createAiAgent does not accept the active flag.
func (r *AiAgentResource) finishCreate(
	ctx context.Context,
	model *AiAgentModel,
	configuredActive types.Bool,
	resp *resource.CreateResponse,
) {
	if isConfiguredBool(configuredActive) {
		if err := r.updateStatus(ctx, model.ID.ValueString(), configuredActive.ValueBool()); err != nil {
			r.rollbackCreate(ctx, model.ID.ValueString(), err, resp)
			return
		}
	}
	agent, err := r.fetchAgent(ctx, model.ID.ValueString())
	if err != nil {
		r.rollbackCreate(ctx, model.ID.ValueString(), err, resp)
		return
	}
	if agent == nil {
		r.rollbackCreate(ctx, model.ID.ValueString(), fmt.Errorf("aiAgent %q returned no agent", model.ID.ValueString()), resp)
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
	var state AiAgentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	var configuredActive types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("active"), &configuredActive)...)
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
	r.applyUpdate(ctx, &plan, state, repoUUID, configuredActive, resp)
}

func (r *AiAgentResource) applyUpdate(
	ctx context.Context,
	plan *AiAgentModel,
	state AiAgentModel,
	repoUUID string,
	configuredActive types.Bool,
	resp *resource.UpdateResponse,
) {
	if err := r.updateAgent(ctx, *plan, repoUUID); err != nil {
		resp.Diagnostics.AddError("update AI agent failed", err.Error())
		return
	}
	statusChanged := isConfiguredBool(configuredActive) &&
		(state.Active.IsNull() || state.Active.ValueBool() != configuredActive.ValueBool())
	if statusChanged {
		if err := r.updateStatus(ctx, plan.ID.ValueString(), configuredActive.ValueBool()); err != nil {
			r.refreshStateAfterPartialUpdate(ctx, plan, resp)
			resp.Diagnostics.AddError("update AI agent status failed", err.Error())
			return
		}
	}
	agent, err := r.fetchAgent(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("read AI agent after update failed", err.Error())
		return
	}
	if agent == nil {
		resp.Diagnostics.AddError(
			"read AI agent after update failed",
			fmt.Sprintf("aiAgent %q returned no agent", plan.ID.ValueString()),
		)
		return
	}
	plan.applyGraphQL(*agent)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
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

func (r *AiAgentResource) updateAgent(
	ctx context.Context,
	model AiAgentModel,
	repoUUID string,
) error {
	return r.api.AiAgents.Update(ctx, model.ID.ValueString(), model.graphQLInput(repoUUID))
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
