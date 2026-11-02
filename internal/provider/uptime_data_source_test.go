// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// readUptime drives the data source's Read directly against the fake API, so every outcome
// can be asserted without a Terraform binary — the same approach as readCertificate.
func readUptime(t *testing.T, api *fakeAPI, domainID string) (uptimeModel, diag.Diagnostics) {
	t.Helper()

	ctx := context.Background()
	ds := &uptimeDataSource{client: newTestClient(t, api)}

	var schemaResp datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

	config := uptimeModel{
		DomainID:              types.StringValue(domainID),
		Enabled:               types.BoolNull(),
		Paused:                types.BoolNull(),
		Status:                types.StringNull(),
		Since:                 types.StringNull(),
		LastCheckedAt:         types.StringNull(),
		Uptime30d:             types.Float64Null(),
		Uptime90d:             types.Float64Null(),
		ResponseTimeAverageMs: types.Int64Null(),
		ResponseTimeMinMs:     types.Int64Null(),
		ResponseTimeMaxMs:     types.Int64Null(),
	}

	// tfsdk.Config has no setter, so the configuration is encoded through a State of the same schema.
	empty := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)
	encoded := tfsdk.State{Schema: schemaResp.Schema, Raw: empty}
	if diags := encoded.Set(ctx, config); diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}

	resp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: empty}}
	ds.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: encoded.Raw}}, &resp)

	var result uptimeModel
	if !resp.Diagnostics.HasError() {
		resp.State.Get(ctx, &result)
	}

	return result, resp.Diagnostics
}

func TestUptimeDataSourceReadsTheSummary(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("example.com")
	api.ping(domain, &Uptime{
		Enabled:       true,
		Status:        "down",
		Since:         ptr("2026-07-29T06:20:11+00:00"),
		LastCheckedAt: ptr("2026-07-29T08:35:11+00:00"),
		Percentages:   UptimePercentages{Last30Days: ptr(99.87), Last90Days: ptr(99.91)},
		ResponseTime:  &UptimeResponseTime{AverageMs: ptr(int64(182)), MinMs: ptr(int64(97)), MaxMs: ptr(int64(1240))},
	})

	got, diags := readUptime(t, api, domain.ID)
	if diags.HasError() || diags.WarningsCount() != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !got.Enabled.ValueBool() || got.Paused.ValueBool() || got.Status.ValueString() != "down" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got.Since.ValueString() != "2026-07-29T06:20:11+00:00" || got.LastCheckedAt.ValueString() != "2026-07-29T08:35:11+00:00" {
		t.Fatalf("unexpected timestamps: %+v", got)
	}
	if got.Uptime30d.ValueFloat64() != 99.87 || got.Uptime90d.ValueFloat64() != 99.91 {
		t.Fatalf("unexpected percentages: %+v", got)
	}
	if got.ResponseTimeAverageMs.ValueInt64() != 182 || got.ResponseTimeMinMs.ValueInt64() != 97 || got.ResponseTimeMaxMs.ValueInt64() != 1240 {
		t.Fatalf("unexpected response times: %+v", got)
	}
}

// A down period that started with a timeout has a state but no response yet: the response
// times are null while the percentages and the timestamps are not.
func TestUptimeDataSourceFlattensANullResponseTime(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("timeout.example")
	api.ping(domain, &Uptime{
		Enabled:     true,
		Status:      "down",
		Since:       ptr("2026-07-29T06:20:11+00:00"),
		Percentages: UptimePercentages{Last30Days: ptr(float64(0))},
	})

	got, diags := readUptime(t, api, domain.ID)
	if diags.HasError() || diags.WarningsCount() != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !got.ResponseTimeAverageMs.IsNull() || !got.ResponseTimeMinMs.IsNull() || !got.ResponseTimeMaxMs.IsNull() {
		t.Fatalf("expected null response times, got %+v", got)
	}
	if got.Uptime30d.ValueFloat64() != 0 || !got.Uptime90d.IsNull() {
		t.Fatalf("expected 0 over 30 days and null over 90, got %+v", got)
	}
}

// A domain that has not been pinged yet is not a mistake in the configuration, so the read
// succeeds with the measurements null and a warning explaining why.
func TestUptimeDataSourceWarnsBeforeFirstPing(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("fresh.example")

	got, diags := readUptime(t, api, domain.ID)
	if diags.HasError() {
		t.Fatalf("expected no error before the first ping, got: %v", diags)
	}
	if diags.WarningsCount() != 1 || diags.Warnings()[0].Summary() != "No uptime measurement yet" {
		t.Fatalf("expected the not-pinged-yet warning, got: %v", diags)
	}
	if got.DomainID.ValueString() != domain.ID || !got.Enabled.ValueBool() || got.Status.ValueString() != "unknown" {
		t.Fatalf("expected an enabled domain in the unknown state, got %+v", got)
	}
	if !got.Since.IsNull() || !got.LastCheckedAt.IsNull() || !got.Uptime30d.IsNull() || !got.ResponseTimeAverageMs.IsNull() {
		t.Fatalf("expected every measurement null, got %+v", got)
	}
}

// With uptime checks switched off the unknown state is expected and permanent, so the
// warning says that rather than promising a first ping.
func TestUptimeDataSourceWarnsWhenChecksAreOff(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("quiet.example")
	off := false
	domain.UptimeEnabled = &off

	got, diags := readUptime(t, api, domain.ID)
	if diags.HasError() {
		t.Fatalf("expected no error, got: %v", diags)
	}
	if diags.WarningsCount() != 1 || diags.Warnings()[0].Summary() != "Uptime checks are off" {
		t.Fatalf("expected the checks-off warning, got: %v", diags)
	}
	if got.Enabled.ValueBool() || got.Status.ValueString() != "unknown" {
		t.Fatalf("expected a disabled domain in the unknown state, got %+v", got)
	}
}

// A paused domain keeps its last state, so the values are real but stale; the warning says so
// and takes precedence over the unknown-state ones.
func TestUptimeDataSourceWarnsWhenPaused(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("paused.example")
	api.ping(domain, &Uptime{
		Enabled:     true,
		Paused:      true,
		Status:      "up",
		Since:       ptr("2026-07-29T06:20:11+00:00"),
		Percentages: UptimePercentages{Last30Days: ptr(100.0), Last90Days: ptr(100.0)},
	})

	got, diags := readUptime(t, api, domain.ID)
	if diags.HasError() {
		t.Fatalf("expected no error, got: %v", diags)
	}
	if diags.WarningsCount() != 1 || diags.Warnings()[0].Summary() != "Uptime checks are paused" {
		t.Fatalf("expected the paused warning, got: %v", diags)
	}
	if !got.Paused.ValueBool() || got.Status.ValueString() != "up" || got.Uptime30d.ValueFloat64() != 100 {
		t.Fatalf("expected the last seen values with paused set, got %+v", got)
	}
}

// An unknown id is a mistake to fix, so it is an error that names the domain.
func TestUptimeDataSourceErrorsOnUnknownDomain(t *testing.T) {
	api := newFakeAPI()

	_, diags := readUptime(t, api, "0198aaaa-0000-7000-8000-999999999999")
	if !diags.HasError() {
		t.Fatal("expected an error for an unknown domain")
	}
	if diags.Errors()[0].Summary() != "Domain not found" {
		t.Fatalf("unexpected error: %v", diags)
	}
}

// Against a real installation a freshly registered test hostname may or may not have been
// pinged by the time the read runs, so the acceptance test asserts what holds either way:
// the read succeeds, the flag is on and a state is present.
func TestAccUptimeDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `resource "certificatechecker_domain" "test" {
					hostname = "tfacc-uptime.example"
				}

				data "certificatechecker_uptime" "test" {
					domain_id = certificatechecker_domain.test.id
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.certificatechecker_uptime.test", "domain_id",
						"certificatechecker_domain.test", "id",
					),
					resource.TestCheckResourceAttr("data.certificatechecker_uptime.test", "enabled", "true"),
					resource.TestCheckResourceAttr("data.certificatechecker_uptime.test", "paused", "false"),
					resource.TestCheckResourceAttrSet("data.certificatechecker_uptime.test", "status"),
				),
			},
		},
	})
}
