package main

import (
	"github.com/hashicorp/terraform/plugin"
	"github.com/terraform-providers/terraform-provider-http/http"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: http.Provider})
}
