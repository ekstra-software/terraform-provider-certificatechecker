// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDomainResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create, and read the computed attributes back.
			{
				Config: `resource "certificatechecker_domain" "test" {
					hostname = "tfacc-create.example"
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("certificatechecker_domain.test", "hostname", "tfacc-create.example"),
					resource.TestCheckResourceAttrSet("certificatechecker_domain.test", "id"),
					resource.TestCheckResourceAttrSet("certificatechecker_domain.test", "created_at"),
					// Omitted in the configuration, so the server default lands in state.
					resource.TestCheckResourceAttr("certificatechecker_domain.test", "uptime_checks", "true"),
				),
			},
			// Turning uptime checks off is an in-place update, not a replacement.
			{
				Config: `resource "certificatechecker_domain" "test" {
					hostname      = "tfacc-create.example"
					uptime_checks = false
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("certificatechecker_domain.test", "uptime_checks", "false"),
					resource.TestCheckResourceAttr("certificatechecker_domain.test", "hostname", "tfacc-create.example"),
				),
			},
			// Dropping the attribute again keeps the account's value: no diff to plan.
			{
				Config: `resource "certificatechecker_domain" "test" {
					hostname = "tfacc-create.example"
				}`,
				Check: resource.TestCheckResourceAttr("certificatechecker_domain.test", "uptime_checks", "false"),
			},
			// Import by id.
			{
				ResourceName:      "certificatechecker_domain.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// A hostname change forces replacement (domains are immutable).
			{
				Config: `resource "certificatechecker_domain" "test" {
					hostname = "tfacc-replace.example"
				}`,
				Check: resource.TestCheckResourceAttr("certificatechecker_domain.test", "hostname", "tfacc-replace.example"),
			},
		},
	})
}

// A hostname that serves no website can be registered with its uptime checks already off,
// so a declarative configuration never has to apply twice to get there.
func TestAccDomainResourceCreatedWithUptimeChecksOff(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `resource "certificatechecker_domain" "mail" {
					hostname      = "tfacc-mail.example"
					uptime_checks = false
				}`,
				Check: resource.TestCheckResourceAttr("certificatechecker_domain.mail", "uptime_checks", "false"),
			},
		},
	})
}

func TestAccDomainDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `resource "certificatechecker_domain" "test" {
					hostname = "tfacc-lookup.example"
				}

				data "certificatechecker_domain" "by_hostname" {
					hostname = certificatechecker_domain.test.hostname
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.certificatechecker_domain.by_hostname", "id",
						"certificatechecker_domain.test", "id",
					),
				),
			},
		},
	})
}
