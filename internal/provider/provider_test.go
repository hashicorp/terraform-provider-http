package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

//nolint:unparam
func protoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"http": providerserver.NewProtocol6WithError(New()),
	}
}

func TestProvider_InvalidProxyConfig(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),

		Steps: []resource.TestStep{
			{
				Config: `
					provider "http" {
						proxy = {
							url = "https://proxy.host.com"
							from_env = true
						}
					}
					resource "http" "test" {
						url = ""
					}
				`,
				ExpectError: regexp.MustCompile(`"proxy.url" cannot be specified when "proxy.from_env" is specified|"proxy.from_env" cannot be specified when "proxy.url" is specified`),
			},
			{
				Config: `
					provider "http" {
						proxy = {
							username = "user"
						}
					}
					resource "http" "test" {
						url = ""
					}
				`,
				ExpectError: regexp.MustCompile(`"proxy.url" must be specified when "proxy.username" is specified`),
			},
			{
				Config: `
					provider "http" {
						proxy = {
							password = "pwd"
						}
					}
					resource "http" "test" {
						url = ""
					}
				`,
				ExpectError: regexp.MustCompile(`"proxy.username" must be specified when "proxy.password" is specified`),
			},
			{
				Config: `
					provider "http" {
						proxy = {
							username = "user"
							password = "pwd"
						}
					}
					resource "http" "test" {
						url = ""
					}
				`,
				ExpectError: regexp.MustCompile(`"proxy.url" must be specified when "proxy.username" is specified`),
			},
			{
				Config: `
					provider "http" {
						proxy = {
							username = "user"
							from_env = true
						}
					}
					resource "http" "test" {
						url = ""
					}
				`,
				ExpectError: regexp.MustCompile(`"proxy.username" cannot be specified when "proxy.from_env" is specified|"proxy.url" must be specified when "proxy.username" is specified`),
			},
		},
	})
}
