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

type gqlReq struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type pipeState struct {
	ID            string
	Name          string
	DeletedCt     int
	PhaseDelCt    int
	PhasesQueried bool
}

func TestUnit_PipeResource_CRUD(t *testing.T) {
	st := &pipeState{}
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
		case strings.Contains(q, "createPipe"):
			st.ID = "pipe_123"
			if v, ok := gr.Variables["name"].(string); ok {
				st.Name = v
			}
			_, _ = io.WriteString(w, `{"data":{"createPipe":{"pipe":{"id":"`+st.ID+`","name":"`+st.Name+`"}}}}`)
		case strings.Contains(q, "updatePipe"):
			if v, ok := gr.Variables["name"].(string); ok {
				st.Name = v
			}
			_, _ = io.WriteString(w, `{"data":{"updatePipe":{"pipe":{"id":"`+st.ID+`"}}}}`)
		case strings.Contains(q, "deletePipe"):
			st.DeletedCt++
			_, _ = io.WriteString(w, `{"data":{"deletePipe":{"success":true}}}`)
		case strings.Contains(q, "deletePhase"):
			st.PhaseDelCt++
			_, _ = io.WriteString(w, `{"data":{"deletePhase":{"clientMutationId":"","success":true}}}`)
		case strings.Contains(q, "phases"):
			st.PhasesQueried = true
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"`+st.ID+`","phases":[{"id":"phase_1"},{"id":"phase_2"}]}}}`)
		case strings.Contains(q, "pipe("):
			_, _ = io.WriteString(w, `{"data":{"pipe":{"id":"`+st.ID+`","name":"`+st.Name+`"}}}`)
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

	resource "pipefy_pipe" "test" {
		name            = "My Pipe"
		organization_id = "org_1"
	}
	`

	configDestroy := `
	provider "pipefy" {
		endpoint = "` + srv.URL + `"
		token    = "testtoken"
	}

	resource "pipefy_pipe" "test" {
		count           = 0
		name            = "My Pipe"
		organization_id = "org_1"
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
						"pipefy_pipe.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("pipe_123"),
					),
					statecheck.ExpectKnownValue(
						"pipefy_pipe.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("My Pipe"),
					),
				},
			},
			{
				Config: strings.ReplaceAll(config, "My Pipe", "Renamed"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"pipefy_pipe.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact("Renamed"),
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
	if !st.PhasesQueried {
		t.Fatalf("expected phases query to be called")
	}
	if st.PhaseDelCt != 2 {
		t.Fatalf("expected 2 phase deletions, got %d", st.PhaseDelCt)
	}
}
