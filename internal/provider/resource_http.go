// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rs "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"golang.org/x/net/http/httpproxy"
)

var _ resource.Resource = (*httpResource)(nil)

func NewHttpResource() resource.Resource {
	return &httpResource{}
}

type httpResource struct{}

func (r *httpResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	// Resource name matches the data source name intentionally.
	resp.TypeName = "http"
}

func (r *httpResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = rs.Schema{
		Description: `
The ` + "`http`" + ` resource makes an HTTP request to the given URL and exports
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

		Attributes: map[string]rs.Attribute{
			"id": rs.StringAttribute{
				Description: "The URL used for the request.",
				Computed:    true,
			},

			"url": rs.StringAttribute{
				Description: "The URL for the request. Supported schemes are `http` and `https`.",
				Required:    true,
			},

			"method": rs.StringAttribute{
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

			"request_headers": rs.MapAttribute{
				Description: "A map of request header field names and values.",
				ElementType: types.StringType,
				Optional:    true,
			},

			"request_body": rs.StringAttribute{
				Description: "The request body as a string.",
				Optional:    true,
			},

			"request_timeout_ms": rs.Int64Attribute{
				Description: "The request timeout in milliseconds.",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},

			"response_body": rs.StringAttribute{
				Description: "The response body returned as a string.",
				Computed:    true,
			},

			"body": rs.StringAttribute{
				Description: "The response body returned as a string. " +
					"**NOTE**: This is deprecated, use `response_body` instead.",
				Computed:           true,
				DeprecationMessage: "Use response_body instead",
			},

			"response_body_base64": rs.StringAttribute{
				Description: "The response body encoded as base64 (standard) as defined in [RFC 4648](https://datatracker.ietf.org/doc/html/rfc4648#section-4).",
				Computed:    true,
			},

			"ca_cert_pem": rs.StringAttribute{
				Description: "Certificate Authority (CA) " +
					"in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("insecure")),
				},
			},

			"client_cert_pem": rs.StringAttribute{
				Description: "Client certificate " +
					"in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("client_key_pem")),
				},
			},

			"client_key_pem": rs.StringAttribute{
				Description: "Client key " +
					"in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("client_cert_pem")),
				},
			},

			"insecure": rs.BoolAttribute{
				Description: "Disables verification of the server's certificate chain and hostname. Defaults to `false`",
				Optional:    true,
			},

			"when": rs.StringAttribute{
				Description: "When to send the HTTP request. Valid values are `apply` (default) and `destroy`. " +
					"When set to `apply`, the request is sent during resource creation and updates. " +
					"When set to `destroy`, the request is only sent during resource destruction.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf([]string{
						"apply",
						"destroy",
					}...),
				},
			},

			"response_headers": rs.MapAttribute{
				Description: `A map of response header field names and values.` +
					` Duplicate headers are concatenated according to [RFC2616](https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2).`,
				ElementType: types.StringType,
				Computed:    true,
			},

			"status_code": rs.Int64Attribute{
				Description: `The HTTP response status code.`,
				Computed:    true,
			},
		},

		Blocks: map[string]rs.Block{
			"retry": rs.SingleNestedBlock{
				Description: "Retry request configuration. By default there are no retries. Configuring this block will result in " +
					"retries if an error is returned by the client (e.g., connection errors) or if a 5xx-range (except 501) status code is received. " +
					"For further details see [go-retryablehttp](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp).",
				Attributes: map[string]rs.Attribute{
					"attempts": rs.Int64Attribute{
						Description: "The number of times the request is to be retried. For example, if 2 is specified, the request will be tried a maximum of 3 times.",
						Optional:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"min_delay_ms": rs.Int64Attribute{
						Description: "The minimum delay between retry requests in milliseconds.",
						Optional:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"max_delay_ms": rs.Int64Attribute{
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

func (r *httpResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
}

func (r *httpResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model modelV0
	diags := req.Plan.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only perform request if "when" is set to "apply" (default behavior when not specified)
	whenValue := "apply"
	if !model.When.IsNull() && !model.When.IsUnknown() {
		whenValue = model.When.ValueString()
	}

	if whenValue == "apply" {
		if err := r.performRequest(ctx, &model, &resp.Diagnostics); err != nil {
			return
		}
	} else {
		// Set default values for computed fields when not making request
		model.ID = types.StringValue(model.URL.ValueString())

		// Create an empty map for response headers
		emptyHeaders := make(map[string]attr.Value)
		model.ResponseHeaders = types.MapValueMust(types.StringType, emptyHeaders)

		model.ResponseBody = types.StringValue("")
		model.Body = types.StringValue("")
		model.ResponseBodyBase64 = types.StringValue("")
		model.StatusCode = types.Int64Value(0)
	}

	diags = resp.State.Set(ctx, model)
	resp.Diagnostics.Append(diags...)
}

func (r *httpResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model modelV0
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// No HTTP request is performed during read operations
	// Ensure computed fields are properly set if they're null/unknown
	if model.ID.IsNull() || model.ID.IsUnknown() {
		model.ID = types.StringValue(model.URL.ValueString())
	}
	if model.ResponseHeaders.IsNull() || model.ResponseHeaders.IsUnknown() {
		emptyHeaders := make(map[string]attr.Value)
		model.ResponseHeaders = types.MapValueMust(types.StringType, emptyHeaders)
	}
	if model.ResponseBody.IsNull() || model.ResponseBody.IsUnknown() {
		model.ResponseBody = types.StringValue("")
	}
	if model.Body.IsNull() || model.Body.IsUnknown() {
		model.Body = types.StringValue("")
	}
	if model.ResponseBodyBase64.IsNull() || model.ResponseBodyBase64.IsUnknown() {
		model.ResponseBodyBase64 = types.StringValue("")
	}
	if model.StatusCode.IsNull() || model.StatusCode.IsUnknown() {
		model.StatusCode = types.Int64Value(0)
	}

	diags = resp.State.Set(ctx, model)
	resp.Diagnostics.Append(diags...)
}

func (r *httpResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Preserve computed fields across updates; reflect config changes and optionally perform request
	var plan, state modelV0
	var diags diag.Diagnostics

	// Read desired configuration from plan
	d := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read prior state to retain computed fields when needed
	d = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	whenValue := "apply"
	if !plan.When.IsNull() && !plan.When.IsUnknown() {
		whenValue = plan.When.ValueString()
	}

	// Begin with desired config (plan)
	model := plan

	if whenValue == "apply" {
		if err := r.performRequest(ctx, &model, &resp.Diagnostics); err != nil {
			return
		}
	} else {
		// Keep previous computed fields when not issuing a request
		model.ID = state.ID
		model.ResponseHeaders = state.ResponseHeaders
		model.ResponseBody = state.ResponseBody
		model.Body = state.Body
		model.ResponseBodyBase64 = state.ResponseBodyBase64
		model.StatusCode = state.StatusCode
	}

	diags = resp.State.Set(ctx, model)
	resp.Diagnostics.Append(diags...)
}

func (r *httpResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model modelV0
	diags := req.State.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only perform request if "when" is set to "destroy"
	whenValue := "apply"
	if !model.When.IsNull() && !model.When.IsUnknown() {
		whenValue = model.When.ValueString()
	}

	if whenValue == "destroy" {
		if err := r.performRequest(ctx, &model, &resp.Diagnostics); err != nil {
			return
		}
	}
}

func (r *httpResource) performRequest(ctx context.Context, model *modelV0, diags *diag.Diagnostics) error {
	requestURL := model.URL.ValueString()
	method := model.Method.ValueString()
	requestHeaders := model.RequestHeaders

	if method == "" {
		method = http.MethodGet
	}

	caCertificate := model.CaCertificate

	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		diags.AddError(
			"Error configuring http transport",
			"Error http: Can't configure http transport.",
		)
		return fmt.Errorf("transport clone")
	}

	clonedTr := tr.Clone()

	clonedTr.Proxy = func(req *http.Request) (*url.URL, error) {
		return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
	}

	if clonedTr.TLSClientConfig == nil {
		clonedTr.TLSClientConfig = &tls.Config{}
	}

	if !model.Insecure.IsNull() {
		if clonedTr.TLSClientConfig == nil {
			clonedTr.TLSClientConfig = &tls.Config{}
		}
		clonedTr.TLSClientConfig.InsecureSkipVerify = model.Insecure.ValueBool()
	}

	// Use `ca_cert_pem` cert pool
	if !caCertificate.IsNull() {
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM([]byte(caCertificate.ValueString())); !ok {
			diags.AddError(
				"Error configuring TLS client",
				"Error tls: Can't add the CA certificate to certificate pool. Only PEM encoded certificates are supported.",
			)
			return fmt.Errorf("bad ca cert")
		}

		if clonedTr.TLSClientConfig == nil {
			clonedTr.TLSClientConfig = &tls.Config{}
		}
		clonedTr.TLSClientConfig.RootCAs = caCertPool
	}

	if !model.ClientCert.IsNull() && !model.ClientKey.IsNull() {
		cert, err := tls.X509KeyPair([]byte(model.ClientCert.ValueString()), []byte(model.ClientKey.ValueString()))
		if err != nil {
			diags.AddError(
				"error creating x509 key pair",
				fmt.Sprintf("error creating x509 key pair from provided pem blocks\n\nError: %s", err),
			)
			return err
		}
		clonedTr.TLSClientConfig.Certificates = []tls.Certificate{cert}
	}

	var retry retryModel
	if !model.Retry.IsNull() && !model.Retry.IsUnknown() {
		if d := model.Retry.As(ctx, &retry, basetypes.ObjectAsOptions{}); d.HasError() {
			diags.Append(d...)
			return fmt.Errorf("retry decode")
		}
	}

	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient.Transport = clonedTr

	var timeout time.Duration

	if model.RequestTimeout.ValueInt64() > 0 {
		timeout = time.Duration(model.RequestTimeout.ValueInt64()) * time.Millisecond
		retryClient.HTTPClient.Timeout = timeout
	}

	retryClient.Logger = levelledLogger{ctx}
	retryClient.RetryMax = int(retry.Attempts.ValueInt64())

	if !retry.MinDelay.IsNull() && !retry.MinDelay.IsUnknown() && retry.MinDelay.ValueInt64() >= 0 {
		retryClient.RetryWaitMin = time.Duration(retry.MinDelay.ValueInt64()) * time.Millisecond
	}

	if !retry.MaxDelay.IsNull() && !retry.MaxDelay.IsUnknown() && retry.MaxDelay.ValueInt64() >= 0 {
		retryClient.RetryWaitMax = time.Duration(retry.MaxDelay.ValueInt64()) * time.Millisecond
	}

	request, err := retryablehttp.NewRequestWithContext(ctx, method, requestURL, nil)
	if err != nil {
		diags.AddError(
			"Error creating request",
			fmt.Sprintf("Error creating request: %s", err),
		)
		return err
	}

	if !model.RequestBody.IsNull() {
		err = request.SetBody(strings.NewReader(model.RequestBody.ValueString()))

		if err != nil {
			diags.AddError(
				"Error Setting Request Body",
				"An unexpected error occurred while setting the request body: "+err.Error(),
			)

			return err
		}
	}

	for name, value := range requestHeaders.Elements() {
		var header string
		d := tfsdk.ValueAs(ctx, value, &header)
		diags.Append(d...)
		if diags.HasError() {
			return fmt.Errorf("header decode")
		}

		request.Header.Set(name, header)
		if strings.ToLower(name) == "host" {
			request.Host = header
		}
	}

	response, err := retryClient.Do(request)
	if err != nil {
		target := &url.Error{}
		if errors.As(err, &target) {
			if target.Timeout() {
				detail := fmt.Sprintf("timeout error: %s", err)

				if timeout > 0 {
					detail = fmt.Sprintf("request exceeded the specified timeout: %s, err: %s", timeout.String(), err)
				}

				diags.AddError(
					"Error making request",
					detail,
				)
				return err
			}
		}

		diags.AddError(
			"Error making request",
			fmt.Sprintf("Error making request: %s", err),
		)
		return err
	}

	defer response.Body.Close()

	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		diags.AddError(
			"Error reading response body",
			fmt.Sprintf("Error reading response body: %s", err),
		)
		return err
	}

	if !utf8.Valid(bytes) {
		diags.AddWarning(
			"Response body is not recognized as UTF-8",
			"Terraform may not properly handle the response_body if the contents are binary.",
		)
	}

	responseBody := string(bytes)
	responseBodyBase64Std := base64.StdEncoding.EncodeToString(bytes)

	responseHeaders := make(map[string]string)
	for k, v := range response.Header {
		// Concatenate according to RFC9110 https://www.rfc-editor.org/rfc/rfc9110.html#section-5.2
		responseHeaders[k] = strings.Join(v, ", ")
	}

	respHeadersState, d := types.MapValueFrom(ctx, types.StringType, responseHeaders)
	diags.Append(d...)
	if diags.HasError() {
		return fmt.Errorf("headers state")
	}

	model.ID = types.StringValue(requestURL)
	model.ResponseHeaders = respHeadersState
	model.ResponseBody = types.StringValue(responseBody)
	model.Body = types.StringValue(responseBody)
	model.ResponseBodyBase64 = types.StringValue(responseBodyBase64Std)
	model.StatusCode = types.Int64Value(int64(response.StatusCode))

	return nil
}
