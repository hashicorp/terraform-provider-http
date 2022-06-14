package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

//nolint:unparam
func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"http": func() (tfprotov6.ProviderServer, error) {
			return providerserver.NewProtocol6(New())(), nil
		},
	}
}
