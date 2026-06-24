// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

type gqlReq struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type pipeState struct {
	ID                string
	Name              string
	Public            bool
	Icon              string
	Color             string
	OnlyAdmin         bool
	OnlyAssignees     bool
	ExpTimeByUnit     *int64
	ExpUnit           *int64
	InboxEmailEnabled bool
	MainTabViews      []string

	Deleted           bool
	PhaseDelCt        int
	CreatePipeCt      int
	UpdatePipeCt      int
	CreateSawSettings bool
	FailUpdate        bool
}

func (st *pipeState) resetDefaults(name string) {
	st.ID = "pipe_123"
	st.Name = name
	st.Deleted = false
	st.Public = false
	st.Icon = "pipefy"
	st.Color = "blue"
	st.OnlyAdmin = false
	st.OnlyAssignees = false
	st.ExpTimeByUnit = nil
	st.ExpUnit = nil
	st.InboxEmailEnabled = true
	st.MainTabViews = []string{"PreviousPhases"}
}

func (st *pipeState) toMap() map[string]any {
	m := map[string]any{
		"id":                            st.ID,
		"name":                          st.Name,
		"public":                        st.Public,
		"icon":                          st.Icon,
		"color":                         st.Color,
		"only_admin_can_remove_cards":   st.OnlyAdmin,
		"only_assignees_can_edit_cards": st.OnlyAssignees,
		"startFormPhaseId":              "sfp_1",
		"organization":                  map[string]any{"id": "org_1"},
		"preferences": map[string]any{
			"inboxEmailEnabled": st.InboxEmailEnabled,
			"mainTabViews":      st.MainTabViews,
		},
	}
	if st.ExpTimeByUnit != nil {
		m["expiration_time_by_unit"] = *st.ExpTimeByUnit
	} else {
		m["expiration_time_by_unit"] = nil
	}
	if st.ExpUnit != nil {
		m["expiration_unit"] = *st.ExpUnit
	} else {
		m["expiration_unit"] = nil
	}
	return m
}

func (st *pipeState) mergeSettings(vars map[string]any) {
	if v, ok := vars["public"].(bool); ok {
		st.Public = v
	}
	if v, ok := vars["icon"].(string); ok {
		st.Icon = v
	}
	if v, ok := vars["color"].(string); ok {
		st.Color = v
	}
	if v, ok := vars["onlyAdminCanRemoveCards"].(bool); ok {
		st.OnlyAdmin = v
	}
	if v, ok := vars["onlyAssigneesCanEditCards"].(bool); ok {
		st.OnlyAssignees = v
	}
	if v, ok := vars["expirationTimeByUnit"].(float64); ok {
		n := int64(v)
		st.ExpTimeByUnit = &n
	}
	if v, ok := vars["expirationUnit"].(float64); ok {
		n := int64(v)
		st.ExpUnit = &n
	}
	if p, ok := vars["preferences"].(map[string]any); ok {
		if v, ok := p["inboxEmailEnabled"].(bool); ok {
			st.InboxEmailEnabled = v
		}
		if vs, ok := p["mainTabViews"].([]any); ok {
			views := make([]string, 0, len(vs))
			for _, x := range vs {
				if s, ok := x.(string); ok {
					views = append(views, s)
				}
			}
			st.MainTabViews = views
		}
	}
}

func sawSettings(vars map[string]any) bool {
	for _, k := range []string{"public", "icon", "color", "onlyAdminCanRemoveCards", "onlyAssigneesCanEditCards", "expirationTimeByUnit", "expirationUnit", "preferences"} {
		if _, ok := vars[k]; ok {
			return true
		}
	}
	return false
}

func newPipeServer(st *pipeState) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testtoken" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"errors":[{"message":"unauthorized"}]}`)
			return
		}
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")

		write := func(payload any) {
			out, _ := json.Marshal(map[string]any{"data": payload})
			_, _ = w.Write(out)
		}

		switch q := gr.Query; {
		case strings.Contains(q, "createPipe"):
			st.CreatePipeCt++
			name, _ := gr.Variables["name"].(string)
			st.resetDefaults(name)
			if sawSettings(gr.Variables) {
				st.CreateSawSettings = true
			}
			write(map[string]any{"createPipe": map[string]any{"pipe": map[string]any{"id": st.ID, "name": st.Name}}})
		case strings.Contains(q, "updatePipe"):
			st.UpdatePipeCt++
			if st.FailUpdate {
				_, _ = io.WriteString(w, `{"errors":[{"message":"boom"}]}`)
				return
			}
			st.mergeSettings(gr.Variables)
			write(map[string]any{"updatePipe": map[string]any{"pipe": st.toMap()}})
		case strings.Contains(q, "deletePipe"):
			write(map[string]any{"deletePipe": map[string]any{"success": true}})
		case strings.Contains(q, "deletePhase"):
			st.PhaseDelCt++
			write(map[string]any{"deletePhase": map[string]any{"clientMutationId": "", "success": true}})
		case strings.Contains(q, "phases"):
			pipe := st.toMap()
			pipe["phases"] = []any{map[string]any{"id": "phase_1"}, map[string]any{"id": "phase_2"}}
			write(map[string]any{"pipe": pipe})
		case strings.Contains(q, "pipe("):
			if st.Deleted {
				write(map[string]any{"pipe": nil})
				return
			}
			write(map[string]any{"pipe": st.toMap()})
		default:
			write(map[string]any{})
		}
	}))
}

func TestUnit_PipeResource_CRUD(t *testing.T) {
	st := &pipeState{}
	srv := newPipeServer(st)
	defer srv.Close()

	config := func(attrs string) string {
		return `
		provider "pipefy" {
			endpoint = "` + srv.URL + `"
			token    = "testtoken"
		}

		resource "pipefy_pipe" "test" {
			name            = "My Pipe"
			organization_id = "org_1"
` + attrs + `
		}
		`
	}

	fullAttrs := `
			public                        = true
			icon                          = "rocket"
			color                         = "purple"
			only_admin_can_remove_cards   = true
			only_assignees_can_edit_cards = false
			preferences = {
				inbox_email_enabled = true
				main_tab_views      = ["PreviousPhases", "Comments"]
			}
			sla = {
				time = 7
				unit = "days"
			}
	`
	updatedAttrs := `
			public                        = true
			icon                          = "rocket"
			color                         = "green"
			only_admin_can_remove_cards   = true
			only_assignees_can_edit_cards = false
			preferences = {
				inbox_email_enabled = true
				main_tab_views      = ["PreviousPhases", "Comments"]
			}
			sla = {
				time = 5
				unit = "hours"
			}
	`

	val := func(path tfjsonpath.Path, v knownvalue.Check) statecheck.StateCheck {
		return statecheck.ExpectKnownValue("pipefy_pipe.test", path, v)
	}
	expectAction := func(action plancheck.ResourceActionType) resource.ConfigPlanChecks {
		return resource.ConfigPlanChecks{
			PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("pipefy_pipe.test", action)},
		}
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(``),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("id"), knownvalue.StringExact("pipe_123")),
					val(tfjsonpath.New("name"), knownvalue.StringExact("My Pipe")),
					val(tfjsonpath.New("icon"), knownvalue.StringExact("pipefy")),
					val(tfjsonpath.New("color"), knownvalue.StringExact("blue")),
					val(tfjsonpath.New("public"), knownvalue.Bool(false)),
					val(tfjsonpath.New("start_form_phase_id"), knownvalue.StringExact("sfp_1")),
					val(tfjsonpath.New("preferences"), knownvalue.Null()),
					val(tfjsonpath.New("sla"), knownvalue.Null()),
				},
			},
			{
				Config:           config(fullAttrs),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionUpdate),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("icon"), knownvalue.StringExact("rocket")),
					val(tfjsonpath.New("color"), knownvalue.StringExact("purple")),
					val(tfjsonpath.New("public"), knownvalue.Bool(true)),
					val(tfjsonpath.New("only_admin_can_remove_cards"), knownvalue.Bool(true)),
					val(tfjsonpath.New("only_assignees_can_edit_cards"), knownvalue.Bool(false)),
					val(tfjsonpath.New("preferences").AtMapKey("inbox_email_enabled"), knownvalue.Bool(true)),
					val(tfjsonpath.New("preferences").AtMapKey("main_tab_views"), knownvalue.ListExact([]knownvalue.Check{
						knownvalue.StringExact("PreviousPhases"),
						knownvalue.StringExact("Comments"),
					})),
					val(tfjsonpath.New("sla").AtMapKey("time"), knownvalue.Int64Exact(7)),
					val(tfjsonpath.New("sla").AtMapKey("unit"), knownvalue.StringExact("days")),
				},
			},
			{
				Config:           config(updatedAttrs),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionUpdate),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("color"), knownvalue.StringExact("green")),
					val(tfjsonpath.New("sla").AtMapKey("time"), knownvalue.Int64Exact(5)),
					val(tfjsonpath.New("sla").AtMapKey("unit"), knownvalue.StringExact("hours")),
				},
			},
			{
				PreConfig:        func() { st.Color = "orange" },
				Config:           config(updatedAttrs),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionUpdate),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("color"), knownvalue.StringExact("green")),
				},
			},
			{
				PreConfig:        func() { st.Deleted = true },
				Config:           config(updatedAttrs),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionCreate),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("color"), knownvalue.StringExact("green")),
					val(tfjsonpath.New("sla").AtMapKey("unit"), knownvalue.StringExact("hours")),
				},
			},
			{
				PreConfig:        func() { st.ExpTimeByUnit = nil; st.ExpUnit = nil },
				Config:           config(updatedAttrs),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionUpdate),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("sla").AtMapKey("time"), knownvalue.Int64Exact(5)),
					val(tfjsonpath.New("sla").AtMapKey("unit"), knownvalue.StringExact("hours")),
				},
			},
			{
				ResourceName:            "pipefy_pipe.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"preferences", "sla"},
			},
		},
	})

	if st.CreateSawSettings {
		t.Errorf("createPipe must not receive settings; they belong to the follow-up updatePipe")
	}
	if st.CreatePipeCt < 2 {
		t.Errorf("expected createPipe to run on the initial create and the recreate, got %d", st.CreatePipeCt)
	}
	if st.UpdatePipeCt == 0 {
		t.Errorf("expected at least one updatePipe call")
	}
	if st.PhaseDelCt < 2 {
		t.Errorf("expected the two auto-created phases to be deleted, got %d", st.PhaseDelCt)
	}
}

func TestUnit_PipeResource_PartialCreateTracksPipe(t *testing.T) {
	st := &pipeState{}
	srv := newPipeServer(st)
	defer srv.Close()

	config := `
		provider "pipefy" {
			endpoint = "` + srv.URL + `"
			token    = "testtoken"
		}

		resource "pipefy_pipe" "test" {
			name            = "My Pipe"
			organization_id = "org_1"
			color           = "purple"
		}
		`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { st.FailUpdate = true },
				Config:      config,
				ExpectError: regexp.MustCompile("update pipe failed"),
			},
			{
				PreConfig: func() { st.FailUpdate = false },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("pipefy_pipe.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
			},
		},
	})
}
