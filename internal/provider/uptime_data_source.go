// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*uptimeDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*uptimeDataSource)(nil)
)

type uptimeDataSource struct {
	client *Client
}

type uptimeModel struct {
	DomainID              types.String  `tfsdk:"domain_id"`
	Enabled               types.Bool    `tfsdk:"enabled"`
	Paused                types.Bool    `tfsdk:"paused"`
	Status                types.String  `tfsdk:"status"`
	Since                 types.String  `tfsdk:"since"`
	LastCheckedAt         types.String  `tfsdk:"last_checked_at"`
	Uptime30d             types.Float64 `tfsdk:"uptime_30d"`
	Uptime90d             types.Float64 `tfsdk:"uptime_90d"`
	ResponseTimeAverageMs types.Int64   `tfsdk:"response_time_average_ms"`
	ResponseTimeMinMs     types.Int64   `tfsdk:"response_time_min_ms"`
	ResponseTimeMaxMs     types.Int64   `tfsdk:"response_time_max_ms"`
}

func NewUptimeDataSource() datasource.DataSource {
	return &uptimeDataSource{}
}

func (d *uptimeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_uptime"
}

func (d *uptimeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Read the uptime summary of a monitored domain, as the Uptime section of its dashboard page shows it: " +
			"the current state and since when it holds, the uptime percentage over the last 30 and 90 days, and the response " +
			"times of the current period. The read always succeeds; before the first ping the state is `unknown` and the " +
			"measurements are null, with a warning saying why. Every value changes as pings come in, so read it where a " +
			"fresh value is wanted — an output, a check — and not as input to a resource. Works on every plan with API " +
			"access (read-only is enough).",
		Attributes: map[string]schema.Attribute{
			"domain_id": schema.StringAttribute{
				Required:    true,
				Description: "Identifier of the domain, from the `certificatechecker_domain` resource or data source.",
			},
			"enabled": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether this domain is pinged for uptime at all; the `uptime_checks` setting of the domain.",
			},
			"paused": schema.BoolAttribute{
				Computed: true,
				Description: "Whether the pings are paused because the plan allows fewer domains than the account has. " +
					"The state is then the last one seen before the pause.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "The current state: `up`, `down`, or `unknown` while uptime checks are off or none has run yet.",
			},
			"since": schema.StringAttribute{
				Computed:    true,
				Description: "When the current state began (ISO 8601, UTC). Null until the first ping.",
			},
			"last_checked_at": schema.StringAttribute{
				Computed:    true,
				Description: "When the domain was last pinged (ISO 8601, UTC). Null until the first ping.",
			},
			"uptime_30d": schema.Float64Attribute{
				Computed: true,
				Description: "Share of the observed time over the last 30 days the domain was up, in percent. " +
					"Moments without a ping count for neither side. Null while nothing was observed in the window.",
			},
			"uptime_90d": schema.Float64Attribute{
				Computed:    true,
				Description: "The same over the last 90 days.",
			},
			"response_time_average_ms": schema.Int64Attribute{
				Computed:    true,
				Description: "Average response time in the current period, in milliseconds. Null while no ping in this period got a response.",
			},
			"response_time_min_ms": schema.Int64Attribute{
				Computed:    true,
				Description: "Fastest response in the current period, in milliseconds. Null while no ping in this period got a response.",
			},
			"response_time_max_ms": schema.Int64Attribute{
				Computed:    true,
				Description: "Slowest response in the current period, in milliseconds. Null while no ping in this period got a response.",
			},
		},
	}
}

func (d *uptimeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *uptimeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config uptimeModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.DomainID.ValueString()

	// The summary endpoint answers 404 only for an unknown domain, but looking the domain up
	// first keeps the diagnostics in line with the certificate data source and gives the
	// warnings below a hostname to name.
	domain, err := d.client.GetDomain(ctx, id)
	if IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Domain not found",
			fmt.Sprintf("No domain with id %s exists on this account.", id),
		)

		return
	}
	if err != nil {
		resp.Diagnostics.Append(apiDiagnostic("looking up domain", err))

		return
	}

	uptime, err := d.client.GetUptime(ctx, id)
	if err != nil {
		resp.Diagnostics.Append(apiDiagnostic("reading uptime", err))

		return
	}

	// Neither is a mistake in the configuration, so both are warnings: the values are real,
	// they are just not what a reader expects from a monitored domain.
	switch {
	case uptime.Paused:
		resp.Diagnostics.AddWarning(
			"Uptime checks are paused",
			fmt.Sprintf("Domain %s (%s) is over the plan's domain limit, so it is not pinged and the uptime values are the last ones seen before the pause. "+
				"Remove other domains or move to a larger plan to resume.", domain.Hostname, id),
		)
	case uptime.Status == "unknown" && !uptime.Enabled:
		resp.Diagnostics.AddWarning(
			"Uptime checks are off",
			fmt.Sprintf("Domain %s (%s) is not pinged because its uptime checks are switched off, so the uptime attributes are null. "+
				"Set `uptime_checks = true` on the domain to start them.", domain.Hostname, id),
		)
	case uptime.Status == "unknown":
		resp.Diagnostics.AddWarning(
			"No uptime measurement yet",
			fmt.Sprintf("Domain %s (%s) has not been pinged yet, so the uptime attributes are null. "+
				"The first ping follows within the plan's check interval; run again once it has happened.", domain.Hostname, id),
		)
	}

	config.fill(uptime)

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

// fill copies the API representation into the Terraform model. The response times come as
// one object that is null as a whole until a ping got a response; flattened here, each of the
// three is null in that case.
func (m *uptimeModel) fill(uptime *Uptime) {
	m.Enabled = types.BoolValue(uptime.Enabled)
	m.Paused = types.BoolValue(uptime.Paused)
	m.Status = types.StringValue(uptime.Status)
	m.Since = types.StringPointerValue(uptime.Since)
	m.LastCheckedAt = types.StringPointerValue(uptime.LastCheckedAt)
	m.Uptime30d = types.Float64PointerValue(uptime.Percentages.Last30Days)
	m.Uptime90d = types.Float64PointerValue(uptime.Percentages.Last90Days)

	if uptime.ResponseTime == nil {
		m.ResponseTimeAverageMs = types.Int64Null()
		m.ResponseTimeMinMs = types.Int64Null()
		m.ResponseTimeMaxMs = types.Int64Null()

		return
	}

	m.ResponseTimeAverageMs = types.Int64PointerValue(uptime.ResponseTime.AverageMs)
	m.ResponseTimeMinMs = types.Int64PointerValue(uptime.ResponseTime.MinMs)
	m.ResponseTimeMaxMs = types.Int64PointerValue(uptime.ResponseTime.MaxMs)
}
