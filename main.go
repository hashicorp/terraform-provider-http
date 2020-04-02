package main

import (
	"github.com/hashicorp/terraform-plugin-sdk/plugin"
	"github.com/hashicorp/terraform-provider-http/http"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: http.Provider})
}
