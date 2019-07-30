package http

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
)

func httpResource() *schema.Resource {
	// Consider data sensitive if env variables is set to true.
	sensitive, _ := strconv.ParseBool(GetEnvOrDefault("HTTP_DATA_IS_SENSITIVE", "false"))

	return &schema.Resource{
		Create: resourceCreate,
		Read:   func(*schema.ResourceData, interface{}) error { return nil },
		Delete: func(*schema.ResourceData, interface{}) error { return nil },

		Schema: map[string]*schema.Schema{
			"url": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"method": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "GET",
				ValidateFunc: validation.StringInSlice([]string{
					"GET", "POST", "PATCH", "DELETE", "PUT", "HEAD", "OPTIONS", "CONNECT", "TRACE",
				}, true),
			},

			"response_status_code": {
				Type:     schema.TypeInt,
				Optional: true,
				ForceNew: true,
				Default:  200,
			},

			"request_headers": {
				Type:      schema.TypeMap,
				Optional:  true,
				ForceNew:  true,
				Sensitive: sensitive,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"request_body": {
				Type:      schema.TypeString,
				Optional:  true,
				ForceNew:  true,
				Sensitive: sensitive,
			},

			"triggers": {
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: true,
			},

			"body": {
				Type:      schema.TypeString,
				Computed:  true,
				Sensitive: sensitive,
			},

			"headers": {
				Type:      schema.TypeMap,
				Computed:  true,
				Sensitive: sensitive,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func resourceCreate(d *schema.ResourceData, meta interface{}) error {
	url := d.Get("url").(string)
	headers := d.Get("request_headers").(map[string]interface{})
	body := d.Get("request_body").(string)
	statusCode := d.Get("response_status_code").(int)

	client := &http.Client{}

	req, err := http.NewRequest(d.Get("method").(string), url, nil)
	if err != nil {
		return fmt.Errorf("Error creating request: %s", err)
	}

	for name, value := range headers {
		req.Header.Set(name, value.(string))
	}

	if len(body) != 0 {
		req.Body = ioutil.NopCloser(strings.NewReader(body))
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error during making a request: %s", url)
	}

	defer resp.Body.Close()

	if resp.StatusCode != statusCode {
		return fmt.Errorf("HTTP request error. Response code: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || isContentTypeAllowed(contentType) == false {
		return fmt.Errorf("Content-Type is not a text type. Got: %s", contentType)
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error while reading response body. %s", err)
	}

	d.Set("body", string(bytes))
	d.Set("headers", flattenResponseHeaders(resp.Header))
	d.SetId(time.Now().UTC().String())

	return nil
}
