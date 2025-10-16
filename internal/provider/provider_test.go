// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

//nolint:unparam
func protoV5ProviderFactories() map[string]func() (tfprotov5.ProviderServer, error) {
	return map[string]func() (tfprotov5.ProviderServer, error){
		"http": providerserver.NewProtocol5WithError(New()),
	}
}

func TestProvider_InvalidHostConfig(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
					provider "http" {
						host {
						}
					}
					data "http" "test" {
						url = "https://host.com"
					}
				`,
				ExpectError: regexp.MustCompile(`The argument "name" is required, but no definition was found`),
			},
			{
				Config: `
					provider "http" {
						host {
							name = "host.com"
						}
					}
					data "http" "test" {
						url = "https://host.com"
					}
				`,
			},
			{
				Config: `
					provider "http" {
						host {
							name = "host.com"
							request_headers = {
								foo = "bar"
							}
						}
						host {
							name = "host.com"
							request_headers = {
								foo = "bar"
							}
						}
					}
					data "http" "test" {
						url = "https://host.com"
					}
				`,
				ExpectError: regexp.MustCompile(`Attribute host list must contain at least 0 elements and at most 1 elements`),
			},
			{
				Config: `
					provider "http" {
						host {
							name = "host.com"
							request_headers = {
								foo = "bar"
							}
						}
					}
					data "http" "test" {
						url = "https://host.com"
					}
				`,
			},
		},
	})
}
