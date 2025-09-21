// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
)

func TestProvider_Metadata_TypeName(t *testing.T) {
	p := &ScaffoldingProvider{version: "test"}
	resp := &frameworkprovider.MetadataResponse{}
	p.Metadata(context.Background(), frameworkprovider.MetadataRequest{}, resp)
	if resp.TypeName != "pipefy" {
		t.Fatalf("expected provider type name 'pipefy', got %q", resp.TypeName)
	}
}

func TestProvider_Schema_HasAttributes(t *testing.T) {
	p := &ScaffoldingProvider{version: "test"}
	resp := &frameworkprovider.SchemaResponse{}
	p.Schema(context.Background(), frameworkprovider.SchemaRequest{}, resp)
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
