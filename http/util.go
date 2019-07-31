package http

import (
	"mime"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// This is to prevent potential issues w/ binary files
// and generally unprintable characters
// See https://github.com/hashicorp/terraform/pull/3858#issuecomment-156856738
func isContentTypeAllowed(contentType string) bool {

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
			return charset == "" || charset == "utf-8"
		}
	}

	return false
}

func flattenResponseHeaders(header http.Header) map[string]string {
	headers := make(map[string]string)

	for k, h := range header {
		for _, v := range h {
			// if there are multiple header values assigned, only the last one will be set
			headers[k] = v
		}
	}

	return headers
}

func flattenAction(schema interface{}, body []byte, header http.Header, action string) []map[string][]map[string]interface{} {
	var res []map[string][]map[string]interface{}

	for _, v := range schema.([]interface{}) {
		m := make(map[string][]map[string]interface{})
		for act, val := range v.(map[string]interface{}) {
			for _, a := range val.([]interface{}) {
				s := make(map[string]interface{})
				for k, res := range a.(map[string]interface{}) {
					if act == action && act != "delete" {
						// update computed fields
						if k == "headers" {
							s[k] = flattenResponseHeaders(header)
							continue
						}
						if k == "body" {
							s[k] = string(body)
							continue
						}
					}
					s[k] = res
				}
				m[act] = append(m[act], s)
			}
		}
		res = append(res, m)
	}

	return res
}

/* GetEnvOrDefault is a helper function that returns the value of the
given environment variable, if one exists, or the default value */
func GetEnvOrDefault(k string, defaultvalue string) string {
	v := os.Getenv(k)
	if v == "" {
		return defaultvalue
	}
	return v
}
