package http

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func dataSourceArchive() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceArchiveRead,

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

			"files": {
				Type:     schema.TypeMap,
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
		},
	}
}

func dataSourceArchiveRead(d *schema.ResourceData, meta interface{}) error {

	url := d.Get("url").(string)
	headers := d.Get("request_headers").(map[string]interface{})

	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("Error creating request: %s", err)
	}

	for name, value := range headers {
		req.Header.Set(name, value.(string))
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error making a request: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP request error. Response code: %d", resp.StatusCode)
	}

	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	tarReader := tar.NewReader(zr)
	files := make(map[string]string)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("error while reading tar file: %s", err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			contents := new(bytes.Buffer)

			if _, err := io.Copy(contents, tarReader); err != nil {
				log.Fatal(err)
			}

			// remove leading `./` from filename
			name := strings.TrimLeft(header.Name, "./")

			files[name] = base64.StdEncoding.EncodeToString(contents.Bytes())
		default:
			return fmt.Errorf("unknown tar type: %v", header.Typeflag)
		}
	}

	if err := zr.Close(); err != nil {
		return fmt.Errorf("error while closing gzip file: %s", err)
	}

	if err = d.Set("files", files); err != nil {
		return fmt.Errorf("error setting Files: %s", err)
	}

	response_headers := make(map[string]string)
	for k, v := range resp.Header {
		// Concatenate according to RFC2616
		// cf. https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2
		response_headers[k] = strings.Join(v, ", ")
	}

	if err = d.Set("response_headers", response_headers); err != nil {
		return fmt.Errorf("Error setting HTTP Response Headers: %s", err)
	}
	d.SetId(time.Now().UTC().String())

	return nil
}
