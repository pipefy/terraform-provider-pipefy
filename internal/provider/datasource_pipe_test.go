// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func TestUnit_PipeDataSource_Read(t *testing.T) {
	var lastQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testtoken" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"errors":[{"message":"unauthorized"}]}`)
			return
		}
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		lastQuery = gr.Query
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(gr.Query, "pipe(") {
			pipe := map[string]any{
				"id":                            "pipe_123",
				"name":                          "My Pipe",
				"public":                        true,
				"icon":                          "rocket",
				"color":                         "purple",
				"only_admin_can_remove_cards":   true,
				"only_assignees_can_edit_cards": false,
				"expiration_time_by_unit":       3,
				"expiration_unit":               86400,
				"startFormPhaseId":              "sfp_1",
				"organization":                  map[string]any{"id": "org_1"},
				"preferences": map[string]any{
					"inboxEmailEnabled": false,
					"mainTabViews":      []any{"PreviousPhases", "Comments"},
				},
			}
			out, _ := json.Marshal(map[string]any{"data": map[string]any{"pipe": pipe}})
			_, _ = w.Write(out)
			return
		}
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	data "pipefy_pipe" "test" {
		id = "pipe_123"
	}
	`

	val := func(path tfjsonpath.Path, v knownvalue.Check) statecheck.StateCheck {
		return statecheck.ExpectKnownValue("data.pipefy_pipe.test", path, v)
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					val(tfjsonpath.New("name"), knownvalue.StringExact("My Pipe")),
					val(tfjsonpath.New("organization_id"), knownvalue.StringExact("org_1")),
					val(tfjsonpath.New("public"), knownvalue.Bool(true)),
					val(tfjsonpath.New("icon"), knownvalue.StringExact("rocket")),
					val(tfjsonpath.New("color"), knownvalue.StringExact("purple")),
					val(tfjsonpath.New("only_admin_can_remove_cards"), knownvalue.Bool(true)),
					val(tfjsonpath.New("sla").AtMapKey("time"), knownvalue.Int64Exact(3)),
					val(tfjsonpath.New("sla").AtMapKey("unit"), knownvalue.StringExact("days")),
					val(tfjsonpath.New("preferences").AtMapKey("inbox_email_enabled"), knownvalue.Bool(false)),
					val(tfjsonpath.New("preferences").AtMapKey("main_tab_views"), knownvalue.ListExact([]knownvalue.Check{
						knownvalue.StringExact("PreviousPhases"),
						knownvalue.StringExact("Comments"),
					})),
				},
			},
		},
	})
	for _, field := range []string{"public", "organization", "icon", "color", "expiration_unit", "preferences"} {
		if !strings.Contains(lastQuery, field) {
			t.Errorf("data source query must select %q; query was: %s", field, lastQuery)
		}
	}
}
