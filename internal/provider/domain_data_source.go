// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
)

var (
	_ datasource.DataSource                     = (*domainDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*domainDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*domainDataSource)(nil)
)

type domainDataSource struct {
	client *Client
}

func NewDomainDataSource() datasource.DataSource {
	return &domainDataSource{}
}

func (d *domainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (d *domainDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a monitored domain — by id or by hostname — to read its certificate expiry status. " +
			"Works on every plan with API access (read-only is enough).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Identifier of the domain. Provide either id or hostname.",
			},
			"hostname": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Hostname of the domain. Provide either id or hostname.",
			},
			"days_until_expiration": schema.Int64Attribute{
				Computed:    true,
				Description: "Days until the certificate expires, per the account's expiration strategy. Null until the first successful check.",
			},
			"level": schema.StringAttribute{
				Computed:    true,
				Description: "Expiration level zone: info, warning, error, critical or fatal. Null until the first successful check.",
			},
			"last_checked_at": schema.StringAttribute{
				Computed:    true,
				Description: "Time of the latest successful certificate check (ISO 8601, UTC).",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Time the domain was registered (ISO 8601, UTC).",
			},
		},
	}
}

func (d *domainDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("hostname"),
		),
	}
}

func (d *domainDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *Client, got %T", req.ProviderData))

		return
	}

	d.client = client
}

func (d *domainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config domainModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		domain *Domain
		err    error
	)

	if !config.ID.IsNull() {
		domain, err = d.client.GetDomain(ctx, config.ID.ValueString())
		if IsNotFound(err) {
			resp.Diagnostics.AddError(
				"Domain not found",
				fmt.Sprintf("No domain with id %s exists on this account.", config.ID.ValueString()),
			)

			return
		}
	} else {
		domain, err = d.client.FindDomainByHostname(ctx, config.Hostname.ValueString())
		if err == nil && domain == nil {
			resp.Diagnostics.AddError(
				"Domain not found",
				fmt.Sprintf("No domain with hostname %q exists on this account.", config.Hostname.ValueString()),
			)

			return
		}
	}

	if err != nil {
		resp.Diagnostics.Append(apiDiagnostic("looking up domain", err))

		return
	}

	config.fill(domain)
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
