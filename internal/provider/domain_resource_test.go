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
				),
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
