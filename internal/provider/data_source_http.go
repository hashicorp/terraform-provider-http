package provider

import (
	"context"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ tfsdk.DataSourceType = (*httpDataSourceType)(nil)

type httpDataSourceType struct{}

func (d *httpDataSourceType) GetSchema(context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
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

		Attributes: map[string]tfsdk.Attribute{
			"id": {
				Description: "The ID of this resource.",
				Type:        types.StringType,
				Computed:    true,
			},

			"url": {
				Description: "The URL for the request. Supported schemes are `http` and `https`.",
				Type:        types.StringType,
				Required:    true,
			},

			"method": {
				Description: "The HTTP Method for the request. " +
					"Allowed methods are a subset of methods defined in [RFC7231](https://datatracker.ietf.org/doc/html/rfc7231#section-4.3) namely, " +
					"`GET`, `HEAD`, and `POST`. `POST` support is only intended for read-only URLs, such as submitting a search.",
				Type:     types.StringType,
				Optional: true,
				Validators: []tfsdk.AttributeValidator{
					stringvalidator.OneOf([]string{
						http.MethodGet,
						http.MethodPost,
						http.MethodHead,
					}...),
				},
			},

			"request_headers": {
				Description: "A map of request header field names and values.",
				Type: types.MapType{
					ElemType: types.StringType,
				},
				Optional: true,
			},

			"request_body": {
				Description: "The request body as a string.",
				Type:        types.StringType,
				Optional:    true,
			},

			"response_body": {
				Description: "The response body returned as a string.",
				Type:        types.StringType,
				Computed:    true,
			},

			"body": {
				Description: "The response body returned as a string. " +
					"**NOTE**: This is deprecated, use `response_body` instead.",
				Type:               types.StringType,
				Computed:           true,
				DeprecationMessage: "Use response_body instead",
			},

			"response_headers": {
				Description: `A map of response header field names and values.` +
					` Duplicate headers are concatenated according to [RFC2616](https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2).`,
				Type: types.MapType{
					ElemType: types.StringType,
				},
				Computed: true,
			},

			"status_code": {
				Description: `The HTTP response status code.`,
				Type:        types.Int64Type,
				Computed:    true,
			},
		},
	}, nil
}

func (d *httpDataSourceType) NewDataSource(context.Context, tfsdk.Provider) (tfsdk.DataSource, diag.Diagnostics) {
	return &httpDataSource{}, nil
}

var _ tfsdk.DataSource = (*httpDataSource)(nil)

type httpDataSource struct{}

func (d *httpDataSource) Read(ctx context.Context, req tfsdk.ReadDataSourceRequest, resp *tfsdk.ReadDataSourceResponse) {
	var model modelV0
	diags := req.Config.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	url := model.URL.Value
	method := model.Method.Value
	requestHeaders := model.RequestHeaders
	requestBody := strings.NewReader(model.RequestBody.Value)

	if method == "" {
		method = "GET"
	}

	client := &http.Client{}

	request, err := http.NewRequestWithContext(ctx, method, url, requestBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating request",
			fmt.Sprintf("Error creating request: %s", err),
		)
		return
	}

	for name, value := range requestHeaders.Elems {
		var header string
		diags = tfsdk.ValueAs(ctx, value, &header)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		request.Header.Set(name, header)
	}

	response, err := client.Do(request)
	if err != nil {
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

	bytes, err := ioutil.ReadAll(response.Body)
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
		// Concatenate according to RFC2616
		// cf. https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2
		responseHeaders[k] = strings.Join(v, ", ")
	}

	respHeadersState := types.Map{}

	diags = tfsdk.ValueFrom(ctx, responseHeaders, types.Map{ElemType: types.StringType}.Type(ctx), &respHeadersState)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	model.ID = types.String{Value: url}
	model.ResponseHeaders = respHeadersState
	model.ResponseBody = types.String{Value: responseBody}
	model.Body = types.String{Value: responseBody}
	model.StatusCode = types.Int64{Value: int64(response.StatusCode)}

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
	ResponseHeaders types.Map    `tfsdk:"response_headers"`
	ResponseBody    types.String `tfsdk:"response_body"`
	Body            types.String `tfsdk:"body"`
	StatusCode      types.Int64  `tfsdk:"status_code"`
}
