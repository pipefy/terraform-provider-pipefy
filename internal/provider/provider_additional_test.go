// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerpkg "github.com/pipefy/terraform-provider-pipefy/internal/provider"
)

func TestProvider_Metadata_TypeName(t *testing.T) {
	prov := providerpkg.New("test")()
	resp := &frameworkprovider.MetadataResponse{}
	prov.Metadata(t.Context(), frameworkprovider.MetadataRequest{}, resp)
	if resp.TypeName != "pipefy" {
		t.Fatalf("expected provider type name 'pipefy', got %q", resp.TypeName)
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
