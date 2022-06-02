package provider

import (
	"context"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSource() *schema.Resource {
	return &schema.Resource{
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
		ReadContext: dataSourceRead,

		Schema: map[string]*schema.Schema{
			"url": {
				Description: "The URL for the request. Supported schemes are `http` and `https`.",
				Type:        schema.TypeString,
				Required:    true,
			},

			"request_headers": {
				Description: "A map of request header field names and values.",
				Type:        schema.TypeMap,
				Optional:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"body": {
				Description: "The response body returned as a string. " +
					"**NOTE**: This is deprecated, use `response_body` instead.",
				Type:       schema.TypeString,
				Computed:   true,
				Deprecated: "Use response_body instead",
			},

			"response_body": {
				Description: "The response body returned as a string.",
				Type:        schema.TypeString,
				Computed:    true,
			},

			"response_headers": {
				Description: `A map of response header field names and values.` +
					` Duplicate headers are concatenated with according to [RFC2616](https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2).`,
				Type:     schema.TypeMap,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func dataSourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (diags diag.Diagnostics) {
	url := d.Get("url").(string)
	headers := d.Get("request_headers").(map[string]interface{})

	client := &http.Client{}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return append(diags, diag.Errorf("Error creating request: %s", err)...)
	}

	for name, value := range headers {
		req.Header.Set(name, value.(string))
	}

	resp, err := client.Do(req)
	if err != nil {
		return append(diags, diag.Errorf("Error making request: %s", err)...)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return append(diags, diag.Errorf("HTTP request error. Response code: %d", resp.StatusCode)...)
	}

	contentType := resp.Header.Get("Content-Type")
	if !isContentTypeText(contentType) {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  fmt.Sprintf("Content-Type is not recognized as a text type, got %q", contentType),
			Detail:   "If the content is binary data, Terraform may not properly handle the contents of the response.",
		})
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return append(diags, diag.FromErr(err)...)
	}

	responseHeaders := make(map[string]string)
	for k, v := range resp.Header {
		// Concatenate according to RFC2616
		// cf. https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2
		responseHeaders[k] = strings.Join(v, ", ")
	}

	if err = d.Set("body", string(bytes)); err != nil {
		return append(diags, diag.Errorf("Error setting HTTP response body: %s", err)...)
	}

	if err = d.Set("response_body", string(bytes)); err != nil {
		return append(diags, diag.Errorf("Error setting HTTP response body: %s", err)...)
	}

	if err = d.Set("response_headers", responseHeaders); err != nil {
		return append(diags, diag.Errorf("Error setting HTTP response headers: %s", err)...)
	}

	// set ID as something more stable than time
	d.SetId(url)

	return diags
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
