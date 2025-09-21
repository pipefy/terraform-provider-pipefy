// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

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

type phaseState struct {
	ID        string
	Name      string
	DeletedCt int
}

func TestUnit_PhaseResource_CRUD(t *testing.T) {
	st := &phaseState{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testtoken" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"errors":[{"message":"unauthorized"}]}`)
			return
		}
		var gr gqlReq
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gr)
		w.Header().Set("Content-Type", "application/json")

		q := gr.Query
		switch {
		case strings.Contains(q, "createPhase"):
			st.ID = "phase_123"
			st.Name = gr.Variables["name"].(string)
			io.WriteString(w, `{"data":{"createPhase":{"phase":{"id":"`+st.ID+`","name":"`+st.Name+`"}}}}`)
		case strings.Contains(q, "updatePhase"):
			if v, ok := gr.Variables["name"].(string); ok {
				st.Name = v
			}
			io.WriteString(w, `{"data":{"updatePhase":{"phase":{"id":"`+st.ID+`"}}}}`)
		case strings.Contains(q, "deletePhase"):
			st.DeletedCt++
			io.WriteString(w, `{"data":{"deletePhase":{"success":true}}}`)
		case strings.Contains(q, "phase("):
			io.WriteString(w, `{"data":{"phase":{"id":"`+st.ID+`","name":"`+st.Name+`"}}}`)
		default:
			io.WriteString(w, `{"data":{}}`)
		}
	}))
	defer srv.Close()

	config := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_phase" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "My Phase"
	}
	`

	configDestroy := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_phase" "test" {
		count   = 0
		pipe_id = pipefy_pipe.p.id
		name    = "My Phase"
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
						"pipefy_phase.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("phase_123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_phase.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("My Phase"),
					),
				},
			},
			{
				Config: strings.ReplaceAll(config, "My Phase", "Renamed Phase"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_phase.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Renamed Phase"),
					),
				},
			},
			{
				Config: configDestroy,
			},
		},
	})

	if st.DeletedCt == 0 {
		t.Fatalf("expected delete mutation to be called")
	}
}
