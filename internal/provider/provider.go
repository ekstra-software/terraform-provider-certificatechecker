// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// defaultEndpoint is the hosted Certificate Checker; self-hosted or staging setups
// override it via the endpoint attribute or CERTIFICATECHECKER_ENDPOINT.
const defaultEndpoint = "https://certificate-checker.com"

var _ provider.Provider = (*certificateCheckerProvider)(nil)

type certificateCheckerProvider struct {
	version string
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	APIToken types.String `tfsdk:"api_token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &certificateCheckerProvider{version: version}
	}
}

func (p *certificateCheckerProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "certificatechecker"
	resp.Version = p.version
}

func (p *certificateCheckerProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage the domains that Certificate Checker monitors for SSL/TLS certificate expiry. " +
			"Requires an API token (dashboard: Settings → API tokens); managing domains requires a plan with API write access.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional: true,
				Description: "Base URL of the Certificate Checker installation. Defaults to " + defaultEndpoint +
					", or the CERTIFICATECHECKER_ENDPOINT environment variable when set.",
			},
			"api_token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "Personal access token, created in the dashboard under Settings → API tokens. " +
					"May also be provided via the CERTIFICATECHECKER_API_TOKEN environment variable.",
			},
		},
	}
}

func (p *certificateCheckerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Explicit configuration wins; the environment is the fallback.
	endpoint := os.Getenv("CERTIFICATECHECKER_ENDPOINT")
	if !config.Endpoint.IsNull() {
		endpoint = config.Endpoint.ValueString()
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	token := os.Getenv("CERTIFICATECHECKER_API_TOKEN")
	if !config.APIToken.IsNull() {
		token = config.APIToken.ValueString()
	}
	if token == "" {
		resp.Diagnostics.AddError(
			"Missing API token",
			"Set the api_token provider attribute or the CERTIFICATECHECKER_API_TOKEN environment variable. "+
				"Tokens are created in the Certificate Checker dashboard under Settings → API tokens.",
		)

		return
	}

	client := NewClient(endpoint, token, p.version)
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *certificateCheckerProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewDomainResource,
	}
}

func (p *certificateCheckerProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDomainDataSource,
		NewCertificateDataSource,
	}
}
