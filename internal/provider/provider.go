// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func New() provider.Provider {
	return &httpProvider{}
}

var _ provider.ProviderWithEphemeralResources = (*httpProvider)(nil)

type httpProvider struct{}

func (p *httpProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "http"
}

func (p *httpProvider) Schema(context.Context, provider.SchemaRequest, *provider.SchemaResponse) {
}

func (p *httpProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *httpProvider) Resources(context.Context) []func() resource.Resource {
	return nil
}

func (p *httpProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewHttpDataSource,
	}
}

func (p *httpProvider) EphemeralResources(_ context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{
		NewHttpEphemeralResource,
	}
}
