// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*certificateDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*certificateDataSource)(nil)
)

type certificateDataSource struct {
	client *Client
}

type certificateModel struct {
	DomainID           types.String `tfsdk:"domain_id"`
	Subject            types.String `tfsdk:"subject"`
	Issuer             types.String `tfsdk:"issuer"`
	IssuerOrganization types.String `tfsdk:"issuer_organization"`
	SerialNumber       types.String `tfsdk:"serial_number"`
	ValidFrom          types.String `tfsdk:"valid_from"`
	ValidTo            types.String `tfsdk:"valid_to"`
	Names              types.List   `tfsdk:"names"`
	FingerprintSHA256  types.String `tfsdk:"fingerprint_sha256"`
	FingerprintSHA1    types.String `tfsdk:"fingerprint_sha1"`
	SignatureAlgorithm types.String `tfsdk:"signature_algorithm"`
	KeyType            types.String `tfsdk:"key_type"`
	KeyBits            types.Int64  `tfsdk:"key_bits"`
	RemoteAddress      types.String `tfsdk:"remote_address"`
}

func NewCertificateDataSource() datasource.DataSource {
	return &certificateDataSource{}
}

func (d *certificateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (d *certificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Read the certificate a monitored domain currently serves: the one behind its `days_until_expiration`, " +
			"as seen by the most recent successful check. Until a domain has had one successful check, every attribute is null " +
			"and the read carries a warning rather than an error — normal right after the domain was added. " +
			"Works on every plan with API access (read-only is enough).",
		Attributes: map[string]schema.Attribute{
			"domain_id": schema.StringAttribute{
				Required:    true,
				Description: "Identifier of the domain, from the `certificatechecker_domain` resource or data source.",
			},
			"subject": schema.StringAttribute{
				Computed:    true,
				Description: "Common name the certificate was issued for.",
			},
			"issuer": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the issuing certificate authority, such as `R11`.",
			},
			"issuer_organization": schema.StringAttribute{
				Computed:    true,
				Description: "Organization behind the issuer, such as `Let's Encrypt`.",
			},
			"serial_number": schema.StringAttribute{
				Computed:    true,
				Description: "Serial number, lowercase hex without separators.",
			},
			"valid_from": schema.StringAttribute{
				Computed:    true,
				Description: "Start of the validity period (ISO 8601, UTC).",
			},
			"valid_to": schema.StringAttribute{
				Computed:    true,
				Description: "End of the validity period (ISO 8601, UTC): the expiry date the day counts are based on.",
			},
			"names": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Every hostname the certificate covers: the subject alternative names plus the common name, without duplicates.",
			},
			"fingerprint_sha256": schema.StringAttribute{
				Computed:    true,
				Description: "SHA-256 fingerprint, lowercase hex without separators.",
			},
			"fingerprint_sha1": schema.StringAttribute{
				Computed:    true,
				Description: "SHA-1 fingerprint, lowercase hex without separators.",
			},
			"signature_algorithm": schema.StringAttribute{
				Computed:    true,
				Description: "Signature algorithm, such as `RSA-SHA256` or `ecdsa-with-SHA384`.",
			},
			"key_type": schema.StringAttribute{
				Computed:    true,
				Description: "Type of the public key: `RSA`, `EC`, `DSA` or `DH`. Null when the check did not record it.",
			},
			"key_bits": schema.Int64Attribute{
				Computed:    true,
				Description: "Size of the public key in bits. Null when the check did not record it.",
			},
			"remote_address": schema.StringAttribute{
				Computed:    true,
				Description: "IP address and port that presented this certificate, as `ip:port`.",
			},
		},
	}
}

func (d *certificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *certificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config certificateModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.DomainID.ValueString()

	// The certificate endpoint answers 404 both for an unknown domain and for one without a
	// successful check. Looking the domain up first keeps those apart: a typo in the id is a
	// mistake to fix, a domain that is still waiting for its first check is not.
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

	certificate, err := d.client.GetCertificate(ctx, id)
	if IsNotFound(err) {
		resp.Diagnostics.AddWarning(
			"No successful certificate check yet",
			fmt.Sprintf("Domain %s (%s) has not had a successful certificate check yet, so every certificate attribute is null. "+
				"The first check is queued when a domain is added; run again once it has completed. "+
				"If this persists, the checks are failing — the dashboard shows why.", domain.Hostname, id),
		)
		certificate = &Certificate{}
	} else if err != nil {
		resp.Diagnostics.Append(apiDiagnostic("reading certificate", err))

		return
	}

	resp.Diagnostics.Append(config.fill(ctx, certificate)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

// fill copies the API representation into the Terraform model. A nil slice of names reads
// as null, like the scalars: the API sends null for a check whose payload cannot be read.
func (m *certificateModel) fill(ctx context.Context, certificate *Certificate) diag.Diagnostics {
	m.Subject = types.StringPointerValue(certificate.Subject)
	m.Issuer = types.StringPointerValue(certificate.Issuer)
	m.IssuerOrganization = types.StringPointerValue(certificate.IssuerOrganization)
	m.SerialNumber = types.StringPointerValue(certificate.SerialNumber)
	m.ValidFrom = types.StringPointerValue(certificate.ValidFrom)
	m.ValidTo = types.StringPointerValue(certificate.ValidTo)
	m.FingerprintSHA256 = types.StringPointerValue(certificate.FingerprintSHA256)
	m.FingerprintSHA1 = types.StringPointerValue(certificate.FingerprintSHA1)
	m.SignatureAlgorithm = types.StringPointerValue(certificate.SignatureAlgorithm)
	m.KeyType = types.StringPointerValue(certificate.KeyType)
	m.KeyBits = types.Int64PointerValue(certificate.KeyBits)
	m.RemoteAddress = types.StringPointerValue(certificate.RemoteAddress)

	if certificate.Names == nil {
		m.Names = types.ListNull(types.StringType)

		return nil
	}

	names, diags := types.ListValueFrom(ctx, types.StringType, certificate.Names)
	m.Names = names

	return diags
}
