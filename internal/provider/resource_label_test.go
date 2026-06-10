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

type labelState struct {
	ID        string
	Name      string
	Color     string
	DeletedCt int
}

func TestUnit_LabelResource_CRUD(t *testing.T) {
	st := &labelState{}
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
		case strings.Contains(q, "createLabel"):
			st.ID = "label_123"
			if v, ok := gr.Variables["name"].(string); ok {
				st.Name = v
			}
			if v, ok := gr.Variables["color"].(string); ok {
				st.Color = v
			}
			_, _ = io.WriteString(w, `{"data":{"createLabel":{"label":{"id":"`+st.ID+`","name":"`+st.Name+`","color":"`+st.Color+`"}}}}`)
		case strings.Contains(q, "updateLabel"):
			if v, ok := gr.Variables["name"].(string); ok {
				st.Name = v
			}
			if v, ok := gr.Variables["color"].(string); ok {
				st.Color = v
			}
			_, _ = io.WriteString(w, `{"data":{"updateLabel":{"label":{"id":"`+st.ID+`","name":"`+st.Name+`","color":"`+st.Color+`"}}}}`)
		case strings.Contains(q, "deleteLabel"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deleteLabel":{"success":true}}}`)
		case strings.Contains(q, "createPipe"):
			_, _ = io.WriteString(w, `{"data":{"createPipe":{"pipe":{"id":"pipe_1","name":"My Pipe"}}}}`)
		case strings.Contains(q, "updatePipe"):
			_, _ = io.WriteString(w, `{"data":{"updatePipe":{"pipe":{"id":"pipe_1"}}}}`)
		case strings.Contains(q, "deletePipe"):
			_, _ = io.WriteString(w, `{"data":{"deletePipe":{"success":true}}}`)
		case strings.Contains(q, "deletePhase"):
			_, _ = io.WriteString(w, `{"data":{"deletePhase":{"clientMutationId":"","success":true}}}`)
		case strings.Contains(q, "phases"):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"pipe_1","startFormPhaseId":"sfp_1","phases":[]}}}`)
		case strings.Contains(q, "labels"):
			if st.ID == "" {
				_, _ = io.WriteString(w, `{"data":{"pipe":{"labels":[]}}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"pipe":{"labels":[{"id":"`+st.ID+`","name":"`+st.Name+`","color":"`+st.Color+`"}]}}}`)
		case strings.Contains(q, "pipe("):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"pipe_1","name":"My Pipe","startFormPhaseId":"sfp_1"}}}`)
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

	resource "pipefy_label" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Urgent"
		color   = "#FF0000"
	}
	`

	configUpdated := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "p" {
		name            = "My Pipe"
		organization_id = "org_1"
	}

	resource "pipefy_label" "test" {
		pipe_id = pipefy_pipe.p.id
		name    = "Critical"
		color   = "#990000"
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

	resource "pipefy_label" "test" {
		count   = 0
		pipe_id = pipefy_pipe.p.id
		name    = "Critical"
		color   = "#990000"
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
						"pipefy_label.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("label_123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_label.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Urgent"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_label.test",
						tfjsonpath.New("color"),
						knownvalue.StringExact("#FF0000"),
					),
				},
			},
			{
				Config: configUpdated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_label.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Critical"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_label.test",
						tfjsonpath.New("color"),
						knownvalue.StringExact("#990000"),
					),
				},
			},
			{
				Config: configDestroy,
			},
		},
	})

	if st.DeletedCt == 0 {
		t.Fatalf("expected deleteLabel mutation to be called")
	}
}
