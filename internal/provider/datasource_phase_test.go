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

func TestUnit_PhaseDataSource_Read(t *testing.T) {
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
		w.Header().Set("Content-Type", "application/json")

		q := gr.Query
		switch {
		// Answer only what the query selected. The double used to volunteer a
		// pipe object nobody asked for, which is how a pipe_id that never
		// populated against the real API passed CI.
		case strings.Contains(q, "phase("):
			if !strings.Contains(q, "repo_id") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"errors":[{"message":"query did not select repo_id"}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"phase":{"id":"phase_123","name":"My Phase","repo_id":302825965}}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	data "pipefy_phase" "test" {
		id = "phase_123"
	}
	`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.pipefy_phase.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("phase_123"),
					),
					statecheck.ExpectKnownValue(
						"data.pipefy_phase.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("My Phase"),
					),
					statecheck.ExpectKnownValue(
						"data.pipefy_phase.test",
						tfjsonpath.New("pipe_id"),
						knownvalue.StringExact("302825965"),
					),
				},
			},
		},
	})
}
