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

type tableState struct {
	ID            string
	Name          string
	Description   *string
	Authorization *string
	Color         *string
	Icon          *string

	Deleted       bool
	CreateTableCt int
	UpdateTableCt int
}

func (st *tableState) resetDefaults(name string) {
	st.ID = "table_123"
	st.Name = name
	st.Deleted = false
	color := "blue"
	icon := "briefing"
	st.Color = &color
	st.Icon = &icon
	st.Description = nil
	st.Authorization = nil
}

func (st *tableState) applyVars(vars map[string]any) {
	if v, ok := vars["name"].(string); ok && v != "" {
		st.Name = v
	}
	if v, ok := vars["description"].(string); ok {
		st.Description = &v
	}
	if v, ok := vars["authorization"].(string); ok {
		st.Authorization = &v
	}
	if v, ok := vars["color"].(string); ok {
		st.Color = &v
	}
	if v, ok := vars["icon"].(string); ok {
		st.Icon = &v
	}
}

func (st *tableState) toMap() map[string]any {
	return map[string]any{
		"id":            st.ID,
		"name":          st.Name,
		"description":   st.Description,
		"authorization": st.Authorization,
		"color":         st.Color,
		"icon":          st.Icon,
		"organization":  map[string]any{"id": "org_1"},
	}
}

func newTableServer(st *tableState) *httptest.Server {
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
		case strings.Contains(q, "createTable"):
			st.CreateTableCt++
			name, _ := gr.Variables["name"].(string)
			st.resetDefaults(name)
			st.applyVars(gr.Variables)
			write(map[string]any{"createTable": map[string]any{"table": st.toMap()}})
		case strings.Contains(q, "updateTable"):
			st.UpdateTableCt++
			st.applyVars(gr.Variables)
			write(map[string]any{"updateTable": map[string]any{"table": st.toMap()}})
		case strings.Contains(q, "deleteTable"):
			write(map[string]any{"deleteTable": map[string]any{"success": true}})
		case strings.Contains(q, "table("):
			if st.Deleted {
				write(map[string]any{"table": nil})
				return
			}
			write(map[string]any{"table": st.toMap()})
		default:
			write(map[string]any{})
		}
	}))
}

func TestUnit_TableResource_CRUD(t *testing.T) {
	st := &tableState{}
	srv := newTableServer(st)
	defer srv.Close()

	config := func(attrs string) string {
		return `
		provider "pipefy" {
			endpoint = "` + srv.URL + `"
			token    = "testtoken"
		}

		resource "pipefy_table" "test" {
			name            = "My Table"
			organization_id = "org_1"
` + attrs + `
		}
		`
	}

	fullAttrs := `
			description   = "Tracks widgets"
			authorization = "write"
			icon          = "rocket"
			color         = "purple"
	`
	updatedAttrs := `
			description   = "Tracks widgets v2"
			authorization = "read"
			icon          = "rocket"
			color         = "green"
	`

	val := func(path tfjsonpath.Path, v knownvalue.Check) statecheck.StateCheck {
		return statecheck.ExpectKnownValue("pipefy_table.test", path, v)
	}
	expectAction := func(action plancheck.ResourceActionType) resource.ConfigPlanChecks {
		return resource.ConfigPlanChecks{
			PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("pipefy_table.test", action)},
		}
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create with only required attributes.
			{
				Config: config(``),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("id"), knownvalue.StringExact("table_123")),
					val(tfjsonpath.New("name"), knownvalue.StringExact("My Table")),
					val(tfjsonpath.New("color"), knownvalue.StringExact("blue")),
					val(tfjsonpath.New("icon"), knownvalue.StringExact("briefing")),
					val(tfjsonpath.New("description"), knownvalue.Null()),
					val(tfjsonpath.New("authorization"), knownvalue.Null()),
				},
			},
			// 2. Update: set all optional attributes.
			{
				Config:           config(fullAttrs),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionUpdate),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("description"), knownvalue.StringExact("Tracks widgets")),
					val(tfjsonpath.New("authorization"), knownvalue.StringExact("write")),
					val(tfjsonpath.New("icon"), knownvalue.StringExact("rocket")),
					val(tfjsonpath.New("color"), knownvalue.StringExact("purple")),
				},
			},
			// 3. Update: change values again (covers drift/re-apply path).
			{
				Config:           config(updatedAttrs),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionUpdate),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("description"), knownvalue.StringExact("Tracks widgets v2")),
					val(tfjsonpath.New("authorization"), knownvalue.StringExact("read")),
					val(tfjsonpath.New("color"), knownvalue.StringExact("green")),
				},
			},
			// 4. Import verification.
			{
				ResourceName:      "pipefy_table.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// 5. External deletion is detected and triggers recreate.
			{
				PreConfig:        func() { st.Deleted = true },
				Config:           config(updatedAttrs),
				ConfigPlanChecks: expectAction(plancheck.ResourceActionCreate),
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("color"), knownvalue.StringExact("green")),
				},
			},
		},
	})

	if st.CreateTableCt < 2 {
		t.Errorf("expected createTable to run on the initial create and the recreate, got %d", st.CreateTableCt)
	}
	if st.UpdateTableCt == 0 {
		t.Errorf("expected at least one updateTable call")
	}
}

func TestUnit_TableResource_InvalidAuthorization(t *testing.T) {
	srv := newTableServer(&tableState{})
	defer srv.Close()

	config := `
		provider "pipefy" {
			endpoint = "` + srv.URL + `"
			token    = "testtoken"
		}

		resource "pipefy_table" "test" {
			name            = "My Table"
			organization_id = "org_1"
			authorization   = "invalid"
		}
		`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`(?s)Attribute authorization value must be one of`),
			},
		},
	})
}