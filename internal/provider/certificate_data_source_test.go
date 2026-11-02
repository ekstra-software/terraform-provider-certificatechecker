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

// readCertificate drives the data source's Read directly against the fake API, so the
// three outcomes — found, not checked yet, unknown domain — can be asserted without a
// Terraform binary.
func readCertificate(t *testing.T, api *fakeAPI, domainID string) (certificateModel, diag.Diagnostics) {
	t.Helper()

	ctx := context.Background()
	ds := &certificateDataSource{client: newTestClient(t, api)}

	var schemaResp datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

	config := certificateModel{
		DomainID:           types.StringValue(domainID),
		Subject:            types.StringNull(),
		Issuer:             types.StringNull(),
		IssuerOrganization: types.StringNull(),
		SerialNumber:       types.StringNull(),
		ValidFrom:          types.StringNull(),
		ValidTo:            types.StringNull(),
		Names:              types.ListNull(types.StringType),
		FingerprintSHA256:  types.StringNull(),
		FingerprintSHA1:    types.StringNull(),
		SignatureAlgorithm: types.StringNull(),
		KeyType:            types.StringNull(),
		KeyBits:            types.Int64Null(),
		RemoteAddress:      types.StringNull(),
	}

	// tfsdk.Config has no setter, so the configuration is encoded through a State of the same schema.
	empty := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)
	encoded := tfsdk.State{Schema: schemaResp.Schema, Raw: empty}
	if diags := encoded.Set(ctx, config); diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}

	resp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: empty}}
	ds.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: encoded.Raw}}, &resp)

	var result certificateModel
	if !resp.Diagnostics.HasError() {
		resp.State.Get(ctx, &result)
	}

	return result, resp.Diagnostics
}

func TestCertificateDataSourceReadsTheCertificate(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("example.com")
	api.check(domain, &Certificate{
		Subject: ptr("example.com"),
		Issuer:  ptr("R11"),
		ValidTo: ptr("2026-09-21T06:20:11+00:00"),
		Names:   []string{"www.example.com", "example.com"},
		KeyType: ptr("RSA"),
		KeyBits: ptr(int64(2048)),
	})

	got, diags := readCertificate(t, api, domain.ID)
	if diags.HasError() || diags.WarningsCount() != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.Subject.ValueString() != "example.com" || got.Issuer.ValueString() != "R11" {
		t.Fatalf("unexpected subject/issuer: %+v", got)
	}
	if got.KeyBits.ValueInt64() != 2048 || got.KeyType.ValueString() != "RSA" {
		t.Fatalf("unexpected key: %+v", got)
	}

	var names []string
	got.Names.ElementsAs(context.Background(), &names, false)
	if len(names) != 2 || names[0] != "www.example.com" {
		t.Fatalf("unexpected names: %v", names)
	}
	// Fields the API sent as null stay null rather than becoming empty strings.
	if !got.FingerprintSHA256.IsNull() || !got.RemoteAddress.IsNull() {
		t.Fatalf("expected null for fields the API did not send: %+v", got)
	}
}

// A domain that is still waiting for its first check is not a mistake in the configuration,
// so the read succeeds with every attribute null and a warning explaining why.
func TestCertificateDataSourceWarnsBeforeFirstCheck(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("fresh.example")

	got, diags := readCertificate(t, api, domain.ID)
	if diags.HasError() {
		t.Fatalf("expected no error before the first check, got: %v", diags)
	}
	if diags.WarningsCount() != 1 || diags.Warnings()[0].Summary() != "No successful certificate check yet" {
		t.Fatalf("expected the not-checked-yet warning, got: %v", diags)
	}
	if got.DomainID.ValueString() != domain.ID {
		t.Fatalf("domain_id must survive in state, got %+v", got.DomainID)
	}
	if !got.Subject.IsNull() || !got.ValidTo.IsNull() || !got.Names.IsNull() || !got.KeyBits.IsNull() {
		t.Fatalf("expected every certificate attribute null, got %+v", got)
	}
}

// An unknown id is a mistake to fix, so unlike the missing check it is an error — and it
// names the domain rather than the certificate, since that is what could not be found.
func TestCertificateDataSourceErrorsOnUnknownDomain(t *testing.T) {
	api := newFakeAPI()

	_, diags := readCertificate(t, api, "0198aaaa-0000-7000-8000-999999999999")
	if !diags.HasError() {
		t.Fatal("expected an error for an unknown domain")
	}
	if diags.Errors()[0].Summary() != "Domain not found" {
		t.Fatalf("unexpected error: %v", diags)
	}
}

// Against a real installation a freshly registered test hostname never gets a successful
// check, so the acceptance test covers the warning path end to end: the read succeeds and
// the attributes are null.
func TestAccCertificateDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `resource "certificatechecker_domain" "test" {
					hostname = "tfacc-certificate.example"
				}

				data "certificatechecker_certificate" "test" {
					domain_id = certificatechecker_domain.test.id
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.certificatechecker_certificate.test", "domain_id",
						"certificatechecker_domain.test", "id",
					),
					resource.TestCheckNoResourceAttr("data.certificatechecker_certificate.test", "subject"),
					resource.TestCheckNoResourceAttr("data.certificatechecker_certificate.test", "valid_to"),
				),
			},
		},
	})
}
