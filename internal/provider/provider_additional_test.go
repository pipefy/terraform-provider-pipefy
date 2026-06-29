// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	providerpkg "github.com/pipefy/terraform-provider-pipefy/internal/provider"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
)

func TestProvider_Metadata_TypeName(t *testing.T) {
	prov := providerpkg.New("test")()
	resp := &frameworkprovider.MetadataResponse{}
	prov.Metadata(t.Context(), frameworkprovider.MetadataRequest{}, resp)
	if resp.TypeName != "pipefy" {
		t.Fatalf("expected provider type name 'pipefy', got %q", resp.TypeName)
	}
}

func TestProvider_Configure_SetsTraceID(t *testing.T) {
	prov := providerpkg.New("test")()
	ctx := t.Context()

	schemaResp := &frameworkprovider.SchemaResponse{}
	prov.Schema(ctx, frameworkprovider.SchemaRequest{}, schemaResp)

	raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
		"endpoint":      tftypes.NewValue(tftypes.String, nil),
		"token":         tftypes.NewValue(tftypes.String, nil),
		"client_id":     tftypes.NewValue(tftypes.String, nil),
		"client_secret": tftypes.NewValue(tftypes.String, nil),
		"token_url":     tftypes.NewValue(tftypes.String, nil),
	})
	t.Setenv("PIPEFY_TOKEN", "test-token")

	resp := &frameworkprovider.ConfigureResponse{}
	prov.Configure(ctx, frameworkprovider.ConfigureRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	api, ok := resp.ResourceData.(*client.ApiClient)
	if !ok {
		t.Fatalf("ResourceData = %T, want *client.ApiClient", resp.ResourceData)
	}
	if api.TraceID == "" {
		t.Fatalf("expected Configure to set a trace-id, got empty")
	}
}

func TestProvider_Schema_HasAttributes(t *testing.T) {
	prov := providerpkg.New("test")()
	resp := &frameworkprovider.SchemaResponse{}
	prov.Schema(t.Context(), frameworkprovider.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	if _, ok := attrs["endpoint"]; !ok {
		t.Fatalf("expected endpoint attribute in provider schema")
	}
	if _, ok := attrs["token"]; !ok {
		t.Fatalf("expected token attribute in provider schema")
	}
	if _, ok := attrs["client_id"]; !ok {
		t.Fatalf("expected client_id attribute in provider schema")
	}
	if _, ok := attrs["client_secret"]; !ok {
		t.Fatalf("expected client_secret attribute in provider schema")
	}
	if _, ok := attrs["token_url"]; !ok {
		t.Fatalf("expected token_url attribute in provider schema")
	}
}
