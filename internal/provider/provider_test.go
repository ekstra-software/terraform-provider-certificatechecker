// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories serves the provider in-process for acceptance tests.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"certificatechecker": providerserver.NewProtocol6WithError(New("test")()),
}

/*
Acceptance tests (TF_ACC=1) run against a real Certificate Checker — use a local Sail
environment with a test account on a plan with API write access, never production:

	CERTIFICATECHECKER_ENDPOINT=http://certificate-checker.localhost \
	CERTIFICATECHECKER_API_TOKEN=<token> TF_ACC=1 go test ./... -v
*/
func testAccPreCheck(t *testing.T) {
	t.Helper()

	if os.Getenv("CERTIFICATECHECKER_API_TOKEN") == "" {
		t.Fatal("CERTIFICATECHECKER_API_TOKEN must be set for acceptance tests")
	}
	if os.Getenv("CERTIFICATECHECKER_ENDPOINT") == "" {
		t.Fatal("CERTIFICATECHECKER_ENDPOINT must be set for acceptance tests (never run them against production)")
	}
}
