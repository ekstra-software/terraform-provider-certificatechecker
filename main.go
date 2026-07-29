// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"log"

	"github.com/ekstra-software/terraform-provider-certificatechecker/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version is set by goreleaser at release time (-X main.version=...).
var version = "dev"

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/ekstra-software/certificatechecker",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
