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

type fieldState struct {
	ID        string
	Label     string
	DeletedCt int
}

func TestUnit_FieldResource_CRUD(t *testing.T) {
	st := &fieldState{}
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
		case strings.Contains(q, "createPhaseField"):
			st.ID = "field_123"
			if v, ok := gr.Variables["label"].(string); ok {
				st.Label = v
			}
			_, _ = io.WriteString(w, `{"data":{"createPhaseField":{"phase_field":{"id":"`+st.ID+`","label":"`+st.Label+`"}}}}`)
		case strings.Contains(q, "updatePhaseField"):
			if v, ok := gr.Variables["label"].(string); ok {
				st.Label = v
			}
			_, _ = io.WriteString(w, `{"data":{"updatePhaseField":{"phase_field":{"id":"`+st.ID+`"}}}}`)
		case strings.Contains(q, "deletePhaseField"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deletePhaseField":{"success":true}}}`)
		case strings.Contains(q, "phase("):
			_, _ = io.WriteString(w, `{"data":{"phase":{"fields":[{"id":"`+st.ID+`","label":"`+st.Label+`"}]}}}`)
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

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_phase" "ph" {
		pipe_id = pipefy_pipe.p.id
		name    = "My Phase"
	}

	resource "pipefy_field" "test" {
		phase_id = pipefy_phase.ph.id
		type     = "short_text"
		label    = "My Field"
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

	resource "pipefy_phase" "ph" {
		pipe_id = pipefy_pipe.p.id
		name    = "My Phase"
	}

	resource "pipefy_field" "test" {
		count    = 0
		phase_id = pipefy_phase.ph.id
		type     = "short_text"
		label    = "My Field"
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
						"pipefy_field.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("field_123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("label"),
						knownvalue.StringExact("My Field"),
					),
				},
			},
			{
				Config: strings.ReplaceAll(config, "My Field", "Renamed Field"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_field.test",
						tfjsonpath.New("label"),
						knownvalue.StringExact("Renamed Field"),
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
