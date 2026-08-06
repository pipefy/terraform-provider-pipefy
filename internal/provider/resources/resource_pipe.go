// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/pipefy"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

var _ resource.Resource = &PipeResource{}
var _ resource.ResourceWithImportState = &PipeResource{}

func NewPipeResource() resource.Resource { return &PipeResource{} }

type PipeResource struct{ api *pipefy.Client }

type pipePreferencesModel struct {
	InboxEmailEnabled types.Bool `tfsdk:"inbox_email_enabled"`
	MainTabViews      types.List `tfsdk:"main_tab_views"`
}

type pipeSLAModel struct {
	Time types.Int64  `tfsdk:"time"`
	Unit types.String `tfsdk:"unit"`
}

type PipeModel struct {
	Id                        types.String          `tfsdk:"id"`
	Name                      types.String          `tfsdk:"name"`
	OrganizationId            types.String          `tfsdk:"organization_id"`
	Public                    types.Bool            `tfsdk:"public"`
	Icon                      types.String          `tfsdk:"icon"`
	Color                     types.String          `tfsdk:"color"`
	OnlyAdminCanRemoveCards   types.Bool            `tfsdk:"only_admin_can_remove_cards"`
	OnlyAssigneesCanEditCards types.Bool            `tfsdk:"only_assignees_can_edit_cards"`
	Preferences               *pipePreferencesModel `tfsdk:"preferences"`
	SLA                       *pipeSLAModel         `tfsdk:"sla"`
	StartFormPhaseId          types.String          `tfsdk:"start_form_phase_id"`
}

func (r *PipeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipe"
}

func (r *PipeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Pipe resource",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The ID of the pipe",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":            schema.StringAttribute{Required: true, Description: "Name of the pipe"},
			"organization_id": schema.StringAttribute{Required: true, Description: "The ID of the organization that the pipe belongs to", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"public":          schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether the pipe is public"},
			"icon":            schema.StringAttribute{Optional: true, Computed: true, Description: "Named pipe icon. Defaults to pipefy. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference/pipes) and the GraphiQL explorer (https://app.pipefy.com/graphiql) for in-depth definitions."},
			"color": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Pipe color. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference/pipes) and the GraphiQL explorer (https://app.pipefy.com/graphiql) for in-depth definitions.",
			},
			"only_admin_can_remove_cards":   schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether only admins can delete cards"},
			"only_assignees_can_edit_cards": schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether only card assignees can edit a card"},
			"preferences": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Pipe preferences. Omit the block to leave them unmanaged; removing it stops managing them but does not reset them on the server.",
				Attributes: map[string]schema.Attribute{
					"inbox_email_enabled": schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether the email inbox is enabled"},
					"main_tab_views": schema.ListAttribute{
						ElementType: types.StringType,
						Optional:    true,
						Computed:    true,
						Description: "Card views to show on a card. Supported values are defined by Pipefy; see the API reference (https://developers.pipefy.com/reference/pipes) and the GraphiQL explorer (https://app.pipefy.com/graphiql) for in-depth definitions.",
					},
				},
			},
			"sla": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Card SLA. Omit the block to leave it unmanaged; removing it stops managing it but does not reset it on the server.",
				Validators:  []validator.Object{validators.SLADuration()},
				Attributes: map[string]schema.Attribute{
					"time": schema.Int64Attribute{Required: true, Description: "Count of units (minutes 1-59, hours 1-23, days >= 1)"},
					"unit": schema.StringAttribute{
						Required:    true,
						Description: "SLA unit: " + strings.Join(pipefy.UnitNames, ", ") + ".",
						Validators:  []validator.String{stringvalidator.OneOf(pipefy.UnitNames...)},
					},
				},
			},
			"start_form_phase_id": schema.StringAttribute{
				Computed:      true,
				Description:   "The ID of the start form phase",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *PipeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*pipefy.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *pipefy.Client, got %T", req.ProviderData))
		return
	}
	r.api = api
}

func (m *PipeModel) apply(ctx context.Context, p pipefy.Pipe, onlyUnknown bool) diag.Diagnostics {
	if !onlyUnknown || m.Id.IsUnknown() {
		m.Id = types.StringValue(p.ID)
	}
	if !onlyUnknown || m.Name.IsUnknown() {
		m.Name = types.StringValue(p.Name)
	}
	if !onlyUnknown || m.Public.IsUnknown() {
		m.Public = types.BoolPointerValue(p.Public)
	}
	if !onlyUnknown || m.Icon.IsUnknown() {
		m.Icon = types.StringPointerValue(p.Icon)
	}
	if !onlyUnknown || m.Color.IsUnknown() {
		m.Color = types.StringPointerValue(p.Color)
	}
	if !onlyUnknown || m.OnlyAdminCanRemoveCards.IsUnknown() {
		m.OnlyAdminCanRemoveCards = types.BoolPointerValue(p.OnlyAdminCanRemoveCards)
	}
	if !onlyUnknown || m.OnlyAssigneesCanEditCards.IsUnknown() {
		m.OnlyAssigneesCanEditCards = types.BoolPointerValue(p.OnlyAssigneesCanEditCards)
	}
	if onlyUnknown {
		if m.StartFormPhaseId.IsUnknown() {
			m.StartFormPhaseId = types.StringValue(p.StartFormPhaseID)
		}
	} else if p.StartFormPhaseID != "" {
		m.StartFormPhaseId = types.StringValue(p.StartFormPhaseID)
	}
	if m.Preferences != nil && p.Preferences != nil {
		return m.Preferences.fill(ctx, p.Preferences, onlyUnknown)
	}
	return nil
}

func (pm *pipePreferencesModel) fill(ctx context.Context, p *pipefy.Preferences, onlyUnknown bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if !onlyUnknown || pm.InboxEmailEnabled.IsUnknown() {
		pm.InboxEmailEnabled = types.BoolPointerValue(p.InboxEmailEnabled)
	}
	if !onlyUnknown || pm.MainTabViews.IsUnknown() {
		list, d := types.ListValueFrom(ctx, types.StringType, p.MainTabViews)
		diags.Append(d...)
		pm.MainTabViews = list
	}
	return diags
}

func (m *PipeModel) refreshSLA(p pipefy.Pipe) {
	if m.SLA == nil {
		return
	}
	count, unit, ok := p.SLA()
	if !ok {
		m.SLA = nil
		return
	}
	m.SLA.Time = types.Int64Value(count)
	m.SLA.Unit = types.StringValue(unit)
}

// settings fills in every attribute updatePipe accepts apart from the id and
// the name, which the two callers supply differently. hasSettings reports
// whether anything was configured, because Create skips the update entirely
// when nothing was.
func (m *PipeModel) settings(ctx context.Context) (in pipefy.UpdatePipeInput, hasSettings bool, diags diag.Diagnostics) {
	in.Public = optionalBool(m.Public)
	in.Icon = optionalString(m.Icon)
	in.Color = optionalString(m.Color)
	in.OnlyAdminCanRemoveCards = optionalBool(m.OnlyAdminCanRemoveCards)
	in.OnlyAssigneesCanEditCards = optionalBool(m.OnlyAssigneesCanEditCards)
	hasSettings = in.Public != nil || in.Icon != nil || in.Color != nil ||
		in.OnlyAdminCanRemoveCards != nil || in.OnlyAssigneesCanEditCards != nil

	// The SLA is sent but never refreshed from the mutation response: SLADuration
	// constrains the pair so the API stores it without normalizing to a coarser
	// unit, so the configured values round-trip. Read re-derives it to catch drift.
	if m.SLA != nil {
		if secs, ok := pipefy.UnitNameToSeconds(m.SLA.Unit.ValueString()); ok {
			count := m.SLA.Time.ValueInt64()
			in.ExpirationTimeByUnit = &count
			in.ExpirationUnit = &secs
			hasSettings = true
		}
	}
	if m.Preferences != nil {
		pref := map[string]any{}
		if hasValue(m.Preferences.InboxEmailEnabled) {
			pref["inboxEmailEnabled"] = m.Preferences.InboxEmailEnabled.ValueBool()
		}
		if hasValue(m.Preferences.MainTabViews) {
			var views []string
			diags.Append(m.Preferences.MainTabViews.ElementsAs(ctx, &views, false)...)
			pref["mainTabViews"] = views
		}
		if len(pref) > 0 {
			in.Preferences = pref
			hasSettings = true
		}
	}
	return in, hasSettings, diags
}

// deletePhases removes the phases createPipe seeds, so a managed pipe starts
// empty. That is this provider choosing a shape, not the API requiring one,
// which is why it lives here rather than in the SDK.
func (r *PipeResource) deletePhases(ctx context.Context, ids []string) error {
	for _, id := range ids {
		ok, err := r.api.Phases.Delete(ctx, id)
		if err != nil {
			return fmt.Errorf("phase %s: %w", id, err)
		}
		if !ok {
			return fmt.Errorf("phase %s: success=false", id)
		}
	}
	return nil
}

func (r *PipeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pipeID, err := r.api.Pipes.Create(ctx, pipefy.CreatePipeInput{
		Name:           data.Name.ValueString(),
		OrganizationID: data.OrganizationId.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("create pipe failed", err.Error())
		return
	}
	data.Id = types.StringValue(pipeID)

	seed := PipeModel{Id: data.Id, Name: data.Name, OrganizationId: data.OrganizationId}
	resp.Diagnostics.Append(resp.State.Set(ctx, &seed)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// createPipe seeds the pipe with three default phases. Fetch them alongside the
	// current settings so they can be removed and the payload reused below.
	payload, phaseIDs, err := r.api.Pipes.GetWithPhaseIDs(ctx, pipeID)
	if errors.Is(err, pipefy.ErrNotFound) {
		resp.Diagnostics.AddError("create pipe failed", "pipe not found right after creation")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("query pipe phases failed", err.Error())
		return
	}
	if err := r.deletePhases(ctx, phaseIDs); err != nil {
		resp.Diagnostics.AddError("delete phase failed", err.Error())
		return
	}

	// createPipe accepts only name and organization. Apply every other setting
	// the user configured with a single update.
	settings, hasSettings, diags := data.settings(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if hasSettings {
		settings.ID = pipeID
		updated, err := r.api.Pipes.Update(ctx, settings)
		if err != nil {
			resp.Diagnostics.AddError("update pipe failed", err.Error())
			return
		}
		payload = updated
	}

	resp.Diagnostics.Append(data.apply(ctx, payload, true)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Id.IsNull() || data.Id.ValueString() == "" {
		return
	}

	pipe, err := r.api.Pipes.Get(ctx, data.Id.ValueString())
	if errors.Is(err, pipefy.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("read pipe failed", err.Error())
		return
	}
	resp.Diagnostics.Append(data.apply(ctx, pipe, false)...)
	if pipe.OrganizationID != "" {
		data.OrganizationId = types.StringValue(pipe.OrganizationID)
	}
	data.refreshSLA(pipe)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, _, diags := data.settings(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	in.ID = data.Id.ValueString()
	name := data.Name.ValueString()
	in.Name = &name

	pipe, err := r.api.Pipes.Update(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("update pipe failed", err.Error())
		return
	}
	resp.Diagnostics.Append(data.apply(ctx, pipe, true)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.Pipes.Delete(ctx, data.Id.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete pipe failed", err.Error())
		return
	}
}

func (r *PipeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
