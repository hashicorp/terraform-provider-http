package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceRead,

		Schema: map[string]*schema.Schema{
			"url": {
				Type:     schema.TypeString,
				Required: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"request_headers": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"timeout": {
				Type:     schema.TypeInt,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeInt,
				},
			},

			"body": {
				Type:     schema.TypeString,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"response_headers": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"ca_certificate": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"insecure": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

func dataSourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (diags diag.Diagnostics) {
	url := d.Get("url").(string)
	headers := d.Get("request_headers").(map[string]interface{})
	caCert := d.Get("ca_certificate").(string)
	timeout := d.Get("timeout").(int)

	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: d.Get("insecure").(bool),
		},
	}

	// Use `ca_certificate` cert pool
	if caCert != "" {
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM([]byte(caCert)); !ok {
			return append(diags, diag.Errorf("Error tls: Can't add the CA certificate to certificate pool")...)
		}

		tr.TLSClientConfig.RootCAs = caCertPool
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(timeout) * time.Second, // defaults to 0
	}

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
	if contentType == "" || isContentTypeText(contentType) == false {
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

	d.Set("body", string(bytes))
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
		regexp.MustCompile("^application/samlmetadata\\+xml"),
	}

	for _, r := range allowedContentTypes {
		if r.MatchString(parsedType) {
			charset := strings.ToLower(params["charset"])
			return charset == "" || charset == "utf-8" || charset == "us-ascii"
		}
	}

	return false
}
