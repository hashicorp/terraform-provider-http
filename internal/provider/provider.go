// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type httpProviderConfig struct {
}

type httpProvider struct {
	Hostname       string
	RequestHeaders map[string]string
}

var _ provider.Provider = (*httpProvider)(nil)

func New() provider.Provider {
	return &httpProvider{}
}

type httpProviderConfigModel struct {
	Host types.List `tfsdk:"host"`
}

type httpProviderHostConfigModel struct {
	Name           types.String `tfsdk:"name"`
	RequestHeaders types.Map    `tfsdk:"request_headers"`
}

func (p *httpProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "http"
}

func (p *httpProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Configures the HTTP provider",
		Blocks: map[string]schema.Block{
			"host": schema.ListNestedBlock{
				Description: "A host-specific provider configuration.",
				Validators: []validator.List{
					listvalidator.SizeBetween(0, 1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: `The hostname for which the host configuration should
take affect. If the name matches an HTTP request URL's hostname, the provider's
host configuration takes affect (in addition to any data- or resource-specific
request configuration.`,
							Required: true,
						},
						"request_headers": schema.MapAttribute{
							Description: `A map of request header field names and values to
include in HTTP requests if/when the request URL's hostname matches the provider
host configuration name.`,
							ElementType: types.StringType,
							Optional:    true,
							Sensitive:   true,
						},
					},
				},
			},
		},
	}
}

func (p *httpProvider) Configure(ctx context.Context, req provider.ConfigureRequest, res *provider.ConfigureResponse) {
	tflog.Debug(ctx, "Configuring provider")
	//p.resetConfig()

	// Ensure these response values are set before early returns, etc.
	res.DataSourceData = p
	res.ResourceData = p

	// Load configuration into the model
	var conf httpProviderConfigModel
	res.Diagnostics.Append(req.Config.Get(ctx, &conf)...)
	if res.Diagnostics.HasError() {
		return
	}
	if conf.Host.IsNull() || conf.Host.IsUnknown() || len(conf.Host.Elements()) == 0 {
		tflog.Debug(ctx, "No host configuration detected; using provider defaults")
		return
	}

	// Load proxy configuration into model
	hostConfSlice := make([]httpProviderHostConfigModel, 1)
	res.Diagnostics.Append(conf.Host.ElementsAs(ctx, &hostConfSlice, true)...)
	if res.Diagnostics.HasError() {
		return
	}
	if len(hostConfSlice) != 1 {
		res.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Provider Proxy Configuration Handling Error",
			"The provider failed to fully load the expected host configuration. "+
				"This is likely a bug in the Terraform Provider and should be reported to the provider developers.",
		)
		return
	}
	hostConf := hostConfSlice[0]
	tflog.Debug(ctx, "Loaded provider configuration")

	// Parse the host name
	if !hostConf.Name.IsNull() && !hostConf.Name.IsUnknown() {
		tflog.Debug(ctx, "Configuring host via name", map[string]interface{}{
			"name": hostConf.Name.ValueString(),
		})

		p.Hostname = hostConf.Name.ValueString()
	}

	if !hostConf.RequestHeaders.IsNull() && !hostConf.RequestHeaders.IsUnknown() {
		tflog.Debug(ctx, "Configuring request headers")
		requestHeaders := map[string]string{}
		for name, value := range hostConf.RequestHeaders.Elements() {
			var header string
			diags := tfsdk.ValueAs(ctx, value, &header)
			res.Diagnostics.Append(diags...)
			if res.Diagnostics.HasError() {
				return
			}

			requestHeaders[name] = header
		}

		p.RequestHeaders = requestHeaders
	}

	tflog.Debug(ctx, "Provider configured")
}

func (p *httpProvider) Resources(context.Context) []func() resource.Resource {
	return nil
}

func (p *httpProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewHttpDataSource,
	}
}

// toProvider casts a generic provider.Provider reference to this specific provider.
// This can be used in DataSourceType.NewDataSource and ResourceType.NewResource calls.
func toProvider(in any) (*httpProvider, diag.Diagnostics) {
	if in == nil {
		return nil, nil
	}

	var diags diag.Diagnostics
	p, ok := in.(*httpProvider)
	if !ok {
		diags.AddError(
			"Unexpected Provider Instance Type",
			fmt.Sprintf("While creating the data source or resource, an unexpected provider type (%T) was received. "+
				"This is likely a bug in the provider code and should be reported to the provider developers.", in,
			),
		)
		return nil, diags
	}

	return p, diags
}
