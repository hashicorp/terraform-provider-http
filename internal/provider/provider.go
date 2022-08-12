package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

func New() provider.Provider {
	return &httpProvider{}
}

var _ provider.Provider = (*httpProvider)(nil)

type httpProvider struct{}

func (p *httpProvider) GetSchema(context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{}, nil
}

func (p *httpProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *httpProvider) GetResources(context.Context) (map[string]provider.ResourceType, diag.Diagnostics) {
	return map[string]provider.ResourceType{}, nil
}

func (p *httpProvider) GetDataSources(context.Context) (map[string]provider.DataSourceType, diag.Diagnostics) {
	return map[string]provider.DataSourceType{
		"http": &httpDataSourceType{},
	}, nil
}
