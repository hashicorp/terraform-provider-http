package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/net/http/httpproxy"
)

func New() tfsdk.Provider {
	return &provider{}
}

var _ tfsdk.Provider = (*provider)(nil)

type provider struct {
	proxyURL     *url.URL
	proxyFromEnv bool
}

func (p *provider) GetSchema(context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		Attributes: map[string]tfsdk.Attribute{
			"proxy": {
				Optional: true,
				Attributes: tfsdk.SingleNestedAttributes(map[string]tfsdk.Attribute{
					"url": {
						Type:     types.StringType,
						Optional: true,
						Validators: []tfsdk.AttributeValidator{
							UrlWithScheme(supportedProxySchemesStr()...),
							ConflictsWith(tftypes.NewAttributePath().WithAttributeName("proxy").WithAttributeName("from_env")),
						},
						MarkdownDescription: "URL used to connect to the Proxy. " +
							fmt.Sprintf("Accepted schemes are: `%s`. ", strings.Join(supportedProxySchemesStr(), "`, `")),
					},
					"username": {
						Type:     types.StringType,
						Optional: true,
						Validators: []tfsdk.AttributeValidator{
							RequiredWith(tftypes.NewAttributePath().WithAttributeName("proxy").WithAttributeName("url")),
						},
						MarkdownDescription: "Username (or Token) used for Basic authentication against the Proxy.",
					},
					"password": {
						Type:      types.StringType,
						Optional:  true,
						Sensitive: true,
						Validators: []tfsdk.AttributeValidator{
							RequiredWith(tftypes.NewAttributePath().WithAttributeName("proxy").WithAttributeName("username")),
						},
						MarkdownDescription: "Password used for Basic authentication against the Proxy.",
					},
					"from_env": {
						Type:     types.BoolType,
						Optional: true,
						Computed: true,
						Validators: []tfsdk.AttributeValidator{
							ConflictsWith(
								tftypes.NewAttributePath().WithAttributeName("proxy").WithAttributeName("url"),
								tftypes.NewAttributePath().WithAttributeName("proxy").WithAttributeName("username"),
								tftypes.NewAttributePath().WithAttributeName("proxy").WithAttributeName("password"),
							),
						},
						MarkdownDescription: "When `true` the provider will discover the proxy configuration from environment variables. " +
							"This is based upon [`http.ProxyFromEnvironment`](https://pkg.go.dev/net/http#ProxyFromEnvironment) " +
							"and it supports the same environment variables (default: `true`).",
					},
				}),
				MarkdownDescription: "Proxy used by resources and data sources that connect to external endpoints.",
			},
		},
	}, nil
}

func (p *provider) Configure(ctx context.Context, req tfsdk.ConfigureProviderRequest, res *tfsdk.ConfigureProviderResponse) {
	tflog.Debug(ctx, "Configuring provider")
	var err error

	// Load configuration into the model
	var conf providerConfigModel
	res.Diagnostics.Append(req.Config.Get(ctx, &conf)...)
	if res.Diagnostics.HasError() {
		return
	}
	if conf.Proxy.IsNull() || conf.Proxy.IsUnknown() {
		tflog.Debug(ctx, "No proxy configuration detected: using provider defaults", map[string]interface{}{
			"provider": fmt.Sprintf("%+v", p),
		})
		return
	}

	// Load proxy configuration into model
	var proxyConf providerProxyConfigModel
	diags := conf.Proxy.As(ctx, &proxyConf, types.ObjectAsOptions{})
	res.Diagnostics.Append(diags...)
	if res.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "Loaded provider configuration")

	// Parse the URL
	if !proxyConf.URL.IsNull() && !proxyConf.URL.IsUnknown() {
		tflog.Debug(ctx, "Configuring Proxy via URL", map[string]interface{}{
			"url": proxyConf.URL.Value,
		})

		p.proxyURL, err = url.Parse(proxyConf.URL.Value)
		if err != nil {
			res.Diagnostics.AddError(fmt.Sprintf("Unable to parse proxy URL %q", proxyConf.URL.Value), err.Error())
		}
	}

	if !proxyConf.Username.IsNull() && !proxyConf.Username.IsUnknown() {
		tflog.Debug(ctx, "Adding username to Proxy URL configuration", map[string]interface{}{
			"username": proxyConf.Username.Value,
		})

		// NOTE: we know that `.proxyURL` is set, as this is imposed by the provider schema
		p.proxyURL.User = url.User(proxyConf.Username.Value)
	}

	if !proxyConf.Password.IsNull() && !proxyConf.Password.IsUnknown() {
		tflog.Debug(ctx, "Adding password to Proxy URL configuration")

		// NOTE: we know that `.proxyURL.User.Username()` is set, as this is imposed by the provider schema
		p.proxyURL.User = url.UserPassword(p.proxyURL.User.Username(), proxyConf.Password.Value)
	}

	if !proxyConf.FromEnv.IsNull() && !proxyConf.FromEnv.IsUnknown() {
		tflog.Debug(ctx, "Configuring Proxy via Environment Variables")

		p.proxyFromEnv = proxyConf.FromEnv.Value
	}

	tflog.Debug(ctx, "Provider configured")
}

func (p *provider) GetResources(context.Context) (map[string]tfsdk.ResourceType, diag.Diagnostics) {
	return map[string]tfsdk.ResourceType{}, nil
}

func (p *provider) GetDataSources(context.Context) (map[string]tfsdk.DataSourceType, diag.Diagnostics) {
	return map[string]tfsdk.DataSourceType{
		"http": &httpDataSourceType{},
	}, nil
}

// ProxyScheme represents url schemes supported when providing proxy configuration to this provider.
type ProxyScheme string

const (
	HTTPProxy   ProxyScheme = "http"
	HTTPSProxy  ProxyScheme = "https"
	SOCKS5Proxy ProxyScheme = "socks5"
)

func (p ProxyScheme) String() string {
	return string(p)
}

// supportedProxySchemes returns an array of ProxyScheme currently supported by this provider.
func supportedProxySchemes() []ProxyScheme {
	return []ProxyScheme{
		HTTPProxy,
		HTTPSProxy,
		SOCKS5Proxy,
	}
}

// supportedProxySchemesStr returns the same content of supportedProxySchemes but as a slice of string.
func supportedProxySchemesStr() []string {
	supported := supportedProxySchemes()
	supportedStr := make([]string, len(supported))
	for i := range supported {
		supportedStr[i] = string(supported[i])
	}
	return supportedStr
}

type providerConfigModel struct {
	Proxy types.Object `tfsdk:"proxy"` //< providerProxyConfigModel
}

type providerProxyConfigModel struct {
	URL      types.String `tfsdk:"url"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	FromEnv  types.Bool   `tfsdk:"from_env"`
}

// isProxyConfigured returns true if a proxy configuration was detected as part of provider.Configure.
func (p *provider) isProxyConfigured() bool {
	return p.proxyURL != nil || p.proxyFromEnv
}

// proxyForRequestFunc is an adapter that returns the configured proxy.
//
// It works by returning a function that, given an *http.Request,
// provides the http.Client with the *url.URL to the proxy.
//
// It will return nil if there is no proxy configured.
func (p *provider) proxyForRequestFunc(ctx context.Context) func(_ *http.Request) (*url.URL, error) {
	if !p.isProxyConfigured() {
		tflog.Debug(ctx, "Proxy not configured")
		return nil
	}

	if p.proxyURL != nil {
		tflog.Debug(ctx, "Proxy via URL")
		return func(_ *http.Request) (*url.URL, error) {
			tflog.Debug(ctx, "Using proxy (URL)", map[string]interface{}{
				"proxy": p.proxyURL,
			})
			return p.proxyURL, nil
		}
	}

	if p.proxyFromEnv {
		tflog.Debug(ctx, "Proxy via ENV")
		return func(req *http.Request) (*url.URL, error) {
			// NOTE: this is based upon `http.ProxyFromEnvironment`,
			// but it avoids a memoization optimization (i.e. fetching environment variables once)
			// that causes issues when testing the provider.
			proxyURL, err := httpproxy.FromEnvironment().ProxyFunc()(req.URL)
			if err != nil {
				return nil, err
			}

			tflog.Debug(ctx, "Using proxy (ENV)", map[string]interface{}{
				"proxy": proxyURL,
			})
			return proxyURL, err
		}
	}

	return nil
}
