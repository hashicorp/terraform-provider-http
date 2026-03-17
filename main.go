// Copyright IBM Corp. 2017, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/terraform-providers/terraform-provider-http/internal/provider"
)

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", true, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{
		Address: "registry.terraform.io/hashicorp/http",
		Debug:   debug,
		// TODO: This is a breaking change, so we would need to add an additional changelog + bump a major version indicating that
		// only Terraform 1.0+ is supported for this version of the provider. Earlier versions of Terraform would need to pin
		// the provider version to 3.5.0
		ProtocolVersion: 6,
	})
	if err != nil {
		log.Fatal(err)
	}
}
