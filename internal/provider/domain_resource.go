// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*domainResource)(nil)
	_ resource.ResourceWithConfigure   = (*domainResource)(nil)
	_ resource.ResourceWithImportState = (*domainResource)(nil)

	// The API accepts a pasted URL and normalizes it server-side, but Terraform would
	// then see a different hostname come back than was configured. Requiring a bare
	// hostname up front keeps config and state identical.
	hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)
)

type domainResource struct {
	client *Client
}

type domainModel struct {
	ID                  types.String `tfsdk:"id"`
	Hostname            types.String `tfsdk:"hostname"`
	UptimeChecks        types.Bool   `tfsdk:"uptime_checks"`
	DaysUntilExpiration types.Int64  `tfsdk:"days_until_expiration"`
	Level               types.String `tfsdk:"level"`
	LastCheckedAt       types.String `tfsdk:"last_checked_at"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

func NewDomainResource() resource.Resource {
	return &domainResource{}
}

func (r *domainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (r *domainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A domain monitored by Certificate Checker. Domains are immutable apart from their uptime " +
			"setting: changing the hostname replaces the domain (and starts a fresh check history). " +
			"Requires a plan with API write access.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifier of the domain.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Required: true,
				Description: "Bare hostname to monitor, e.g. `example.com` — no scheme or path. " +
					"The certificate served on port 443 is checked.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(hostnamePattern, "must be a bare hostname such as example.com (no scheme, path or port)"),
				},
			},
			"uptime_checks": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Whether this domain is pinged for uptime, on top of its certificate check. " +
					"Defaults to `true`; set it to `false` for a hostname that serves no website, such as a mail host. " +
					"Leaving it out adopts whatever the account has set, so existing configurations keep working unchanged.",
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
				Description: "Time of the latest successful certificate check (ISO 8601, UTC). Null until the first successful check.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Time the domain was registered (ISO 8601, UTC).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *domainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *Client, got %T", req.ProviderData))

		return
	}

	r.client = client
}

func (r *domainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unknown means the configuration left uptime_checks out, so the flag is not sent and
	// the API's own default applies; the response fills the computed value back in.
	var uptimeChecks *bool
	if !plan.UptimeChecks.IsUnknown() && !plan.UptimeChecks.IsNull() {
		value := plan.UptimeChecks.ValueBool()
		uptimeChecks = &value
	}

	domain, err := r.client.CreateDomain(ctx, plan.Hostname.ValueString(), uptimeChecks)
	if err != nil {
		resp.Diagnostics.Append(apiDiagnostic("creating domain", err))

		return
	}

	plan.fill(domain)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *domainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state domainModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain, err := r.client.GetDomain(ctx, state.ID.ValueString())
	if err != nil {
		// Gone (or no longer ours) means: drop from state so Terraform plans a re-create.
		if IsNotFound(err) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.Append(apiDiagnostic("reading domain", err))

		return
	}

	state.fill(domain)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update only ever runs for uptime_checks: the hostname forces replacement and everything
// else is computed, so that flag is the single attribute an in-place change can concern.
func (r *domainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state domainModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The id is never part of a change, and the plan may carry it as unknown.
	plan.ID = state.ID

	if plan.UptimeChecks.IsUnknown() || plan.UptimeChecks.IsNull() {
		// A configuration that dropped the attribute keeps the value the account has.
		plan.UptimeChecks = state.UptimeChecks
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

		return
	}

	domain, err := r.client.UpdateDomain(ctx, state.ID.ValueString(), plan.UptimeChecks.ValueBool())
	if err != nil {
		resp.Diagnostics.Append(apiDiagnostic("updating domain", err))

		return
	}

	plan.fill(domain)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *domainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state domainModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteDomain(ctx, state.ID.ValueString()); err != nil && !IsNotFound(err) {
		resp.Diagnostics.Append(apiDiagnostic("deleting domain", err))
	}
}

func (r *domainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// fill copies the API representation into the Terraform model. An API that does not
// report uptime_enabled at all reads as on, which is the server-side default — the
// attribute is computed, so it must end up known either way.
func (m *domainModel) fill(domain *Domain) {
	m.ID = types.StringValue(domain.ID)
	m.Hostname = types.StringValue(domain.Hostname)
	m.UptimeChecks = types.BoolValue(domain.UptimeEnabled == nil || *domain.UptimeEnabled)
	m.DaysUntilExpiration = types.Int64PointerValue(domain.DaysUntilExpiration)
	m.Level = types.StringPointerValue(domain.Level)
	m.LastCheckedAt = types.StringPointerValue(domain.LastCheckedAt)
	m.CreatedAt = types.StringPointerValue(domain.CreatedAt)
}

// apiDiagnostic translates API errors into actionable diagnostics: the tier gate,
// the temporary kill switch and the rate limit each get their own explanation.
func apiDiagnostic(action string, err error) diag.Diagnostic {
	if apiErr, ok := err.(*APIError); ok {
		switch apiErr.StatusCode {
		case http.StatusForbidden:
			return diag.NewErrorDiagnostic(
				"Plan does not allow this API operation",
				fmt.Sprintf("While %s, the API refused: %s\n\nManaging domains requires a Certificate Checker plan with API write access (Business and up).", action, apiErr.Message),
			)
		case http.StatusServiceUnavailable:
			return diag.NewErrorDiagnostic(
				"Certificate Checker API is temporarily disabled",
				fmt.Sprintf("While %s: %s\n\nThis is a temporary, server-side switch — your configuration and state are fine. Try again later.", action, apiErr.Message),
			)
		case http.StatusTooManyRequests:
			return diag.NewErrorDiagnostic(
				"Certificate Checker API rate limit reached",
				fmt.Sprintf("While %s: %s\n\nThe API allows 60 requests per minute per account. Wait a moment and retry.", action, apiErr.Message),
			)
		}
	}

	return diag.NewErrorDiagnostic(fmt.Sprintf("Error %s", action), err.Error())
}
