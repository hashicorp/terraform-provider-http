package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/net/http/httpproxy"
)

var _ datasource.DataSource = (*httpDataSource)(nil)

func NewHttpDataSource() datasource.DataSource {
	return &httpDataSource{}
}

type httpDataSource struct{}

func (d *httpDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	// This data source name unconventionally is equal to the provider name,
	// but it has been named this since its inception. Changing this widely
	// adopted data source name should only be done with strong consideration
	// to the practitioner burden of updating it everywhere.
	resp.TypeName = "http"
}

func (d *httpDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: `
The ` + "`http`" + ` data source makes an HTTP GET request to the given URL and exports
information about the response.

The given URL may be either an ` + "`http`" + ` or ` + "`https`" + ` URL. At present this resource
can only retrieve data from URLs that respond with ` + "`text/*`" + ` or
` + "`application/json`" + ` content types, and expects the result to be UTF-8 encoded
regardless of the returned content type header.

~> **Important** Although ` + "`https`" + ` URLs can be used, there is currently no
mechanism to authenticate the remote server except for general verification of
the server certificate's chain of trust. Data retrieved from servers not under
your control should be treated as untrustworthy.`,

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

			"request_timeout": schema.Int64Attribute{
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

			"ca_cert_pem": schema.StringAttribute{
				Description: "Certificate data of the Certificate Authority (CA) " +
					"in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("insecure")),
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
				Description: "Retry request configuration.",
				Attributes: map[string]schema.Attribute{
					"attempts": schema.Int64Attribute{
						Description: "The number of retry attempts.",
						Optional:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"min_delay": schema.Int64Attribute{
						Description: "The minimum delay between retry requests in milliseconds.",
						Optional:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"max_delay": schema.Int64Attribute{
						Description: "The maximum delay between retry requests in milliseconds.",
						Optional:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
							int64validator.AtLeastSumOf(path.MatchRelative().AtParent().AtName("min_delay")),
						},
					},
				},
			},
		},
	}
}

func (d *httpDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model modelV0
	diags := req.Config.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestURL := model.URL.ValueString()
	method := model.Method.ValueString()
	requestHeaders := model.RequestHeaders
	requestBody := strings.NewReader(model.RequestBody.ValueString())

	if method == "" {
		method = "GET"
	}

	caCertificate := model.CaCertificate

	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		resp.Diagnostics.AddError(
			"Error configuring http transport",
			"Error http: Can't configure http transport.",
		)
		return
	}

	// Prevent issues with multiple data source configurations modifying the shared transport.
	clonedTr := tr.Clone()

	// Prevent issues with tests caching the proxy configuration.
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
			resp.Diagnostics.AddError(
				"Error configuring TLS client",
				"Error tls: Can't add the CA certificate to certificate pool. Only PEM encoded certificates are supported.",
			)
			return
		}

		if clonedTr.TLSClientConfig == nil {
			clonedTr.TLSClientConfig = &tls.Config{}
		}
		clonedTr.TLSClientConfig.RootCAs = caCertPool
	}

	var retry retryModel

	if !model.Retry.IsNull() && !model.Retry.IsUnknown() {
		diags = model.Retry.As(ctx, &retry, basetypes.ObjectAsOptions{})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
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

	request, err := retryablehttp.NewRequestWithContext(ctx, method, requestURL, requestBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating request",
			fmt.Sprintf("Error creating request: %s", err),
		)
		return
	}

	for name, value := range requestHeaders.Elements() {
		var header string
		diags = tfsdk.ValueAs(ctx, value, &header)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		request.Header.Set(name, header)
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

				resp.Diagnostics.AddError(
					"Error making request",
					detail,
				)
				return
			}
		}

		resp.Diagnostics.AddError(
			"Error making request",
			fmt.Sprintf("Error making request: %s", err),
		)
		return
	}

	defer response.Body.Close()

	contentType := response.Header.Get("Content-Type")
	if !isContentTypeText(contentType) {
		resp.Diagnostics.AddWarning(
			fmt.Sprintf("Content-Type is not recognized as a text type, got %q", contentType),
			"If the content is binary data, Terraform may not properly handle the contents of the response.",
		)
	}

	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading response body",
			fmt.Sprintf("Error reading response body: %s", err),
		)
		return
	}

	responseBody := string(bytes)

	responseHeaders := make(map[string]string)
	for k, v := range response.Header {
		// Concatenate according to RFC9110 https://www.rfc-editor.org/rfc/rfc9110.html#section-5.2
		responseHeaders[k] = strings.Join(v, ", ")
	}

	respHeadersState, diags := types.MapValueFrom(ctx, types.StringType, responseHeaders)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	model.ID = types.StringValue(requestURL)
	model.ResponseHeaders = respHeadersState
	model.ResponseBody = types.StringValue(responseBody)
	model.Body = types.StringValue(responseBody)
	model.StatusCode = types.Int64Value(int64(response.StatusCode))

	diags = resp.State.Set(ctx, model)
	resp.Diagnostics.Append(diags...)
}

// This is to prevent potential issues w/ binary files
// and generally unprintable characters
// See https://github.com/hashicorp/terraform/pull/3858#issuecomment-156856738
func isContentTypeText(contentType string) bool {

	parsedType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	allowedContentTypes := []*regexp.Regexp{
		regexp.MustCompile("^text/.+"),
		regexp.MustCompile("^application/json$"),
		regexp.MustCompile(`^application/samlmetadata\+xml`),
	}

	for _, r := range allowedContentTypes {
		if r.MatchString(parsedType) {
			charset := strings.ToLower(params["charset"])
			return charset == "" || charset == "utf-8" || charset == "us-ascii"
		}
	}

	return false
}

type modelV0 struct {
	ID              types.String `tfsdk:"id"`
	URL             types.String `tfsdk:"url"`
	Method          types.String `tfsdk:"method"`
	RequestHeaders  types.Map    `tfsdk:"request_headers"`
	RequestBody     types.String `tfsdk:"request_body"`
	RequestTimeout  types.Int64  `tfsdk:"request_timeout"`
	Retry           types.Object `tfsdk:"retry"`
	ResponseHeaders types.Map    `tfsdk:"response_headers"`
	CaCertificate   types.String `tfsdk:"ca_cert_pem"`
	Insecure        types.Bool   `tfsdk:"insecure"`
	ResponseBody    types.String `tfsdk:"response_body"`
	Body            types.String `tfsdk:"body"`
	StatusCode      types.Int64  `tfsdk:"status_code"`
}

type retryModel struct {
	Attempts types.Int64 `tfsdk:"attempts"`
	MinDelay types.Int64 `tfsdk:"min_delay"`
	MaxDelay types.Int64 `tfsdk:"max_delay"`
}

var _ retryablehttp.LeveledLogger = levelledLogger{}

// levelledLogger is used to log messages from retryablehttp.Client to tflog.
type levelledLogger struct {
	ctx context.Context
}

func (l levelledLogger) Error(msg string, keysAndValues ...interface{}) {
	tflog.Error(l.ctx, msg, l.additionalFields(keysAndValues))
}

func (l levelledLogger) Info(msg string, keysAndValues ...interface{}) {
	tflog.Info(l.ctx, msg, l.additionalFields(keysAndValues))
}

func (l levelledLogger) Debug(msg string, keysAndValues ...interface{}) {
	tflog.Debug(l.ctx, msg, l.additionalFields(keysAndValues))
}

func (l levelledLogger) Warn(msg string, keysAndValues ...interface{}) {
	tflog.Warn(l.ctx, msg, l.additionalFields(keysAndValues))
}

func (l levelledLogger) additionalFields(keysAndValues []interface{}) map[string]interface{} {
	additionalFields := make(map[string]interface{}, len(keysAndValues))

	for i := 0; i+1 < len(keysAndValues); i += 2 {
		additionalFields[fmt.Sprint(keysAndValues[i])] = keysAndValues[i+1]
	}

	return additionalFields
}
