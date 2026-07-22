// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ ephemeral.EphemeralResource = (*httpEphemeralResource)(nil)

func NewHttpEphemeralResource() ephemeral.EphemeralResource {
	return &httpEphemeralResource{}
}

type httpEphemeralResource struct{}

func (d *httpEphemeralResource) Metadata(_ context.Context, _ ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = "http"
}

func (d *httpEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `
The ` + "`http`" + ` ephemeral resource makes an HTTP GET request to the given URL and exports
information about the response.

The given URL may be either an ` + "`http`" + ` or ` + "`https`" + ` URL. This resource
will issue a warning if the result is not UTF-8 encoded.

~> **Important** Although ` + "`https`" + ` URLs can be used, there is currently no
mechanism to authenticate the remote server except for general verification of
the server certificate's chain of trust. Data retrieved from servers not under
your control should be treated as untrustworthy.

By default, there are no retries. Configuring the retry block will result in
retries if an error is returned by the client (e.g., connection errors) or if 
a 5xx-range (except 501) status code is received. For further details see 
[go-retryablehttp](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp).
`,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The URL used for the request.",
				Computed:    true,
			},

			"url": schema.StringAttribute{
				Description: "The URL for the request. Supported schemes are `http` and `https`.",
				Required:    true,
			},

			"method": schema.StringAttribute{
				Description: "The HTTP Method for the request. " +
					"Allowed methods are a subset of methods defined in [RFC7231](https://datatracker.ietf.org/doc/html/rfc7231#section-4.3) namely, " +
					"`GET`, `HEAD`, and `POST`. `POST` support is only intended for read-only URLs, such as submitting a search.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf([]string{
						http.MethodGet,
						http.MethodPost,
						http.MethodHead,
					}...),
				},
			},

			"request_headers": schema.MapAttribute{
				Description: "A map of request header field names and values.",
				ElementType: types.StringType,
				Optional:    true,
			},

			"request_body": schema.StringAttribute{
				Description: "The request body as a string.",
				Optional:    true,
			},

			"request_timeout_ms": schema.Int64Attribute{
				Description: "The request timeout in milliseconds.",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},

			"response_body": schema.StringAttribute{
				Description: "The response body returned as a string.",
				Computed:    true,
			},

			"body": schema.StringAttribute{
				Description: "The response body returned as a string. " +
					"**NOTE**: This is deprecated, use `response_body` instead.",
				Computed:           true,
				DeprecationMessage: "Use response_body instead",
			},

			"response_body_base64": schema.StringAttribute{
				Description: "The response body encoded as base64 (standard) as defined in [RFC 4648](https://datatracker.ietf.org/doc/html/rfc4648#section-4).",
				Computed:    true,
			},

			"ca_cert_pem": schema.StringAttribute{
				Description: "Certificate Authority (CA) " +
					"in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("insecure")),
				},
			},

			"client_cert_pem": schema.StringAttribute{
				Description: "Client certificate " +
					"in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("client_key_pem")),
				},
			},

			"client_key_pem": schema.StringAttribute{
				Description: "Client key " +
					"in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("client_cert_pem")),
				},
			},

			"insecure": schema.BoolAttribute{
				Description: "Disables verification of the server's certificate chain and hostname. Defaults to `false`",
				Optional:    true,
			},

			"response_headers": schema.MapAttribute{
				Description: `A map of response header field names and values.` +
					` Duplicate headers are concatenated according to [RFC2616](https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2).`,
				ElementType: types.StringType,
				Computed:    true,
			},

			"status_code": schema.Int64Attribute{
				Description: `The HTTP response status code.`,
				Computed:    true,
			},
		},

		Blocks: map[string]schema.Block{
			"retry": schema.SingleNestedBlock{
				Description: "Retry request configuration. By default there are no retries. Configuring this block will result in " +
					"retries if an error is returned by the client (e.g., connection errors) or if a 5xx-range (except 501) status code is received. " +
					"For further details see [go-retryablehttp](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp).",
				Attributes: map[string]schema.Attribute{
					"attempts": schema.Int64Attribute{
						Description: "The number of times the request is to be retried. For example, if 2 is specified, the request will be tried a maximum of 3 times.",
						Optional:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"min_delay_ms": schema.Int64Attribute{
						Description: "The minimum delay between retry requests in milliseconds.",
						Optional:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"max_delay_ms": schema.Int64Attribute{
						Description: "The maximum delay between retry requests in milliseconds.",
						Optional:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
							int64validator.AtLeastSumOf(path.MatchRelative().AtParent().AtName("min_delay_ms")),
						},
					},
				},
			},
		},
	}
}

func (d *httpEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var model modelV0
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(doRequest(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Result.Set(ctx, model)...)
}
