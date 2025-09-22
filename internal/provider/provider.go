// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/datasources"
	"github.com/pipefy/terraform-provider-pipefy/internal/provider/resources"
	"golang.org/x/oauth2/clientcredentials"
)

// Ensure ScaffoldingProvider satisfies various provider interfaces.
var _ provider.Provider = &ScaffoldingProvider{}
var _ provider.ProviderWithEphemeralResources = &ScaffoldingProvider{}

// ScaffoldingProvider defines the provider implementation.
type ScaffoldingProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// ScaffoldingProviderModel describes the provider data model.
type ScaffoldingProviderModel struct {
	Endpoint     types.String `tfsdk:"endpoint"`
	Token        types.String `tfsdk:"token"`
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
	TokenURL     types.String `tfsdk:"token_url"`
}

func (p *ScaffoldingProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "pipefy"
	resp.Version = p.version
}

func (p *ScaffoldingProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Pipefy GraphQL endpoint. Defaults to https://api.pipefy.com/graphql",
				Optional:            true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Pipefy API token. Can also be set via PIPEFY_TOKEN environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "Service Account Client ID. Can also be set via PIPEFY_CLIENT_ID environment variable.",
				Optional:            true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "Service Account Client Secret. Can also be set via PIPEFY_CLIENT_SECRET environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"token_url": schema.StringAttribute{
				MarkdownDescription: "Service Account Token Endpoint URL. Defaults to https://app.pipefy.com/oauth/token. Can also be set via PIPEFY_TOKEN_URL environment variable.",
				Optional:            true,
			},
		},
	}
}

func (p *ScaffoldingProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ScaffoldingProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := "https://api.pipefy.com/graphql"
	if !data.Endpoint.IsNull() && !data.Endpoint.IsUnknown() {
		endpoint = data.Endpoint.ValueString()
	}

	token := os.Getenv("PIPEFY_TOKEN")
	if !data.Token.IsNull() && !data.Token.IsUnknown() {
		token = data.Token.ValueString()
	}

	clientID := os.Getenv("PIPEFY_CLIENT_ID")
	if !data.ClientID.IsNull() && !data.ClientID.IsUnknown() {
		clientID = data.ClientID.ValueString()
	}

	clientSecret := os.Getenv("PIPEFY_CLIENT_SECRET")
	if !data.ClientSecret.IsNull() && !data.ClientSecret.IsUnknown() {
		clientSecret = data.ClientSecret.ValueString()
	}

	// Default to Pipefy's OAuth token endpoint
	tokenURL := "https://app.pipefy.com/oauth/token"
	if !data.TokenURL.IsNull() && !data.TokenURL.IsUnknown() {
		tokenURL = data.TokenURL.ValueString()
	} else if os.Getenv("PIPEFY_TOKEN_URL") != "" {
		tokenURL = os.Getenv("PIPEFY_TOKEN_URL")
	}

	var httpClient *http.Client
	var apiToken string

	// Prefer static token when provided; otherwise use OAuth client credentials if configured
	if token != "" {
		httpClient = &http.Client{Timeout: 30 * time.Second}
		apiToken = token
	} else if clientID != "" && clientSecret != "" {
		cfg := &clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     tokenURL,
			Scopes:       []string{},
		}

		httpClient = cfg.Client(context.Background())
	} else {
		resp.Diagnostics.AddError(
			"Authentication configuration error",
			"Provide either a static token via 'token' (or PIPEFY_TOKEN) or Service Account credentials via 'client_id' and 'client_secret' (token_url defaults to https://app.pipefy.com/oauth/token).",
		)
		return
	}

	api := &client.ApiClient{HTTP: httpClient, Endpoint: endpoint, Token: apiToken}

	resp.DataSourceData = api
	resp.ResourceData = api
}

func (p *ScaffoldingProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewPipeResource,
		resources.NewPhaseResource,
		resources.NewFieldResource,
	}
}

func (p *ScaffoldingProvider) EphemeralResources(ctx context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{}
}

func (p *ScaffoldingProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewPipeDataSource,
		datasources.NewPhaseDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ScaffoldingProvider{
			version: version,
		}
	}
}
