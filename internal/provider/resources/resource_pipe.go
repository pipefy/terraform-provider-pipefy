// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/pipegql"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/validators"
)

var _ resource.Resource = &PipeResource{}
var _ resource.ResourceWithImportState = &PipeResource{}

func NewPipeResource() resource.Resource { return &PipeResource{} }

type PipeResource struct{ api *client.ApiClient }

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

const updatePipeMutation = "mutation($id:ID!,$name:String,$public:Boolean,$icon:String,$color:Colors," +
	"$onlyAdminCanRemoveCards:Boolean,$onlyAssigneesCanEditCards:Boolean," +
	"$expirationTimeByUnit:Int,$expirationUnit:Int,$preferences:RepoPreferenceInput){ " +
	"updatePipe(input:{ id:$id, name:$name, public:$public, icon:$icon, color:$color, " +
	"only_admin_can_remove_cards:$onlyAdminCanRemoveCards, only_assignees_can_edit_cards:$onlyAssigneesCanEditCards, " +
	"expiration_time_by_unit:$expirationTimeByUnit, expiration_unit:$expirationUnit, preferences:$preferences }){ pipe{ " +
	pipegql.Selection + " } } }"

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
						Description: "SLA unit: " + strings.Join(pipegql.UnitNames, ", ") + ".",
						Validators:  []validator.String{validators.OneOf(pipegql.UnitNames...)},
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
	api, ok := req.ProviderData.(*client.ApiClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *ApiClient, got %T", req.ProviderData))
		return
	}
	r.api = api
}

func (m *PipeModel) apply(ctx context.Context, p pipegql.Payload, onlyUnknown bool) diag.Diagnostics {
	if !onlyUnknown || m.Id.IsUnknown() {
		m.Id = types.StringValue(p.Id)
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
			m.StartFormPhaseId = types.StringValue(p.StartFormPhaseId)
		}
	} else if p.StartFormPhaseId != "" {
		m.StartFormPhaseId = types.StringValue(p.StartFormPhaseId)
	}
	if m.Preferences != nil && p.Preferences != nil {
		return m.Preferences.fill(ctx, p.Preferences, onlyUnknown)
	}
	return nil
}

func (pm *pipePreferencesModel) fill(ctx context.Context, p *pipegql.Preferences, onlyUnknown bool) diag.Diagnostics {
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

func (m *PipeModel) refreshSLA(p pipegql.Payload) {
	if m.SLA == nil {
		return
	}
	if count, unit, ok := p.SLA(); ok {
		m.SLA.Time = types.Int64Value(count)
		m.SLA.Unit = types.StringValue(unit)
	}
}

func (m *PipeModel) addSettingsVars(ctx context.Context, vars map[string]any) diag.Diagnostics {
	var diags diag.Diagnostics
	if hasValue(m.Public) {
		vars["public"] = m.Public.ValueBool()
	}
	if hasValue(m.Icon) {
		vars["icon"] = m.Icon.ValueString()
	}
	if hasValue(m.Color) {
		vars["color"] = m.Color.ValueString()
	}
	if hasValue(m.OnlyAdminCanRemoveCards) {
		vars["onlyAdminCanRemoveCards"] = m.OnlyAdminCanRemoveCards.ValueBool()
	}
	if hasValue(m.OnlyAssigneesCanEditCards) {
		vars["onlyAssigneesCanEditCards"] = m.OnlyAssigneesCanEditCards.ValueBool()
	}
	// The SLA is sent but never refreshed from the mutation response: SLADuration
	// constrains the pair so the API stores it without normalizing to a coarser
	// unit, so the configured values round-trip. Read re-derives it to catch drift.
	if m.SLA != nil {
		if secs, ok := pipegql.UnitNameToSeconds(m.SLA.Unit.ValueString()); ok {
			vars["expirationTimeByUnit"] = m.SLA.Time.ValueInt64()
			vars["expirationUnit"] = secs
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
			vars["preferences"] = pref
		}
	}
	return diags
}

func (r *PipeResource) deletePhases(ctx context.Context, ids []string) error {
	const mutation = "mutation($id:ID!){ deletePhase(input:{id:$id}){ clientMutationId success } }"
	for _, id := range ids {
		var del struct {
			DeletePhase struct {
				Success bool `json:"success"`
			} `json:"deletePhase"`
		}
		if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"id": id}, &del); err != nil {
			return fmt.Errorf("phase %s: %w", id, err)
		}
		if !del.DeletePhase.Success {
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

	mutation := "mutation($name:String!,$orgId:ID!){ createPipe(input:{name:$name, organization_id:$orgId}){ pipe{ id name } } }"
	var created struct {
		CreatePipe struct {
			Pipe struct {
				Id string `json:"id"`
			} `json:"pipe"`
		} `json:"createPipe"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"name": data.Name.ValueString(), "orgId": data.OrganizationId.ValueString()}, &created); err != nil {
		resp.Diagnostics.AddError("create pipe failed", err.Error())
		return
	}
	pipeId := created.CreatePipe.Pipe.Id
	data.Id = types.StringValue(pipeId)

	// createPipe seeds the pipe with three default phases. Fetch them alongside the
	// current settings so they can be removed and the payload reused below.
	phasesQuery := "query($id:ID!){ pipe(id:$id){ " + pipegql.Selection + " phases { id } } }"
	var phasesOut struct {
		Pipe *struct {
			pipegql.Payload
			Phases []struct {
				Id string `json:"id"`
			} `json:"phases"`
		} `json:"pipe"`
	}
	if err := r.api.DoGraphQL(ctx, phasesQuery, map[string]any{"id": pipeId}, &phasesOut); err != nil {
		resp.Diagnostics.AddError("query pipe phases failed", err.Error())
		return
	}
	if phasesOut.Pipe == nil {
		resp.Diagnostics.AddError("create pipe failed", "pipe not found right after creation")
		return
	}
	payload := phasesOut.Pipe.Payload

	ids := make([]string, len(phasesOut.Pipe.Phases))
	for i, phase := range phasesOut.Pipe.Phases {
		ids[i] = phase.Id
	}
	if err := r.deletePhases(ctx, ids); err != nil {
		resp.Diagnostics.AddError("delete phase failed", err.Error())
		return
	}

	// createPipe accepts only name and organization. Apply every other setting
	// the user configured with a single update.
	settings := map[string]any{}
	resp.Diagnostics.Append(data.addSettingsVars(ctx, settings)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(settings) > 0 {
		settings["id"] = pipeId
		var updated struct {
			UpdatePipe struct {
				Pipe pipegql.Payload `json:"pipe"`
			} `json:"updatePipe"`
		}
		if err := r.api.DoGraphQL(ctx, updatePipeMutation, settings, &updated); err != nil {
			resp.Diagnostics.AddError("update pipe failed", err.Error())
			return
		}
		payload = updated.UpdatePipe.Pipe
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

	query := "query($id:ID!){ pipe(id:$id){ " + pipegql.Selection + " organization { id } } }"
	var out struct {
		Pipe *struct {
			pipegql.Payload
			Organization *struct {
				Id string `json:"id"`
			} `json:"organization"`
		} `json:"pipe"`
	}
	if err := r.api.DoGraphQL(ctx, query, map[string]any{"id": data.Id.ValueString()}, &out); err != nil {
		resp.Diagnostics.AddError("read pipe failed", err.Error())
		return
	}
	if out.Pipe == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(data.apply(ctx, out.Pipe.Payload, false)...)
	if out.Pipe.Organization != nil {
		data.OrganizationId = types.StringValue(out.Pipe.Organization.Id)
	}
	data.refreshSLA(out.Pipe.Payload)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vars := map[string]any{"id": data.Id.ValueString(), "name": data.Name.ValueString()}
	resp.Diagnostics.Append(data.addSettingsVars(ctx, vars)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		UpdatePipe struct {
			Pipe pipegql.Payload `json:"pipe"`
		} `json:"updatePipe"`
	}
	if err := r.api.DoGraphQL(ctx, updatePipeMutation, vars, &out); err != nil {
		resp.Diagnostics.AddError("update pipe failed", err.Error())
		return
	}
	resp.Diagnostics.Append(data.apply(ctx, out.UpdatePipe.Pipe, true)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PipeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PipeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mutation := "mutation($id:ID!){ deletePipe(input:{id:$id}){ success } }"
	var out struct {
		DeletePipe struct {
			Success bool `json:"success"`
		} `json:"deletePipe"`
	}
	if err := r.api.DoGraphQL(ctx, mutation, map[string]any{"id": data.Id.ValueString()}, &out); err != nil {
		resp.Diagnostics.AddError("delete pipe failed", err.Error())
		return
	}
}

func (r *PipeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
