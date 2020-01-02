---
page_title: "HTTP Data Source"
description: |-
  Retrieves the content at an HTTP or HTTPS URL.
---

# `http` Data Source

The `http` data source makes an HTTP GET request to the given URL and exports
information about the response.

The given URL may be either an `http` or `https` URL. At present this resource
can only retrieve data from URLs that respond with `text/*` or
`application/json` content types, and expects the result to be UTF-8 encoded
regardless of the returned content type header.

~> **Important** Although `https` URLs can be used, there is currently no
mechanism to authenticate the remote server except for general verification of
the server certificate's chain of trust. Data retrieved from servers not under
your control should be treated as untrustworthy.

## Example Usage

```hcl
data "http" "example" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"

  # Optional request headers
  request_headers = {
    Accept = "application/json"
  }
}
```

## Argument Reference

The following arguments are supported:

* `url` - (Required) The URL to request data from. This URL must respond with
  a `200 OK` response and a `text/*` or `application/json` Content-Type.

* `request_headers` - (Optional) A map of strings representing additional HTTP
  headers to include in the request.

* `ca_certificate` - (Optional) PEM-encoded root certificates bundle for TLS authentication.

* `insecure` - (Optional) Whether server should be accessed without verifying the TLS certificate. Defaults to false.

## Attributes Reference

The following attributes are exported:

* `body` - The raw body of the HTTP response.

* `response_headers` - A map of strings representing the response HTTP headers.
  Duplicate headers are contatenated with `, ` according to
  [RFC2616](https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2)
