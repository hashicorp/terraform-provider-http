package provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func New() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{},

		DataSourcesMap: map[string]*schema.Resource{
			"http": dataSource(),
		},

		ResourcesMap: map[string]*schema.Resource{},
	}
}
