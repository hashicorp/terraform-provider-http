---
layout: "http"
page_title: "HTTP ReSource"
sidebar_current: "docs-http-resource"
description: |-
  Make an HTTP request and retrieves the content at an HTTP or HTTPS URL.
---

# `http` Resource

The `http` resource makes an HTTP request to the given URL and exports
information about the response.

The given URL may be either an `http` or `https` URL. At present this resource
can only retrieve data from URLs that respond with `text/*` or
`application/json` content types, and expects the result to be UTF-8 encoded
regardless of the returned content type header.

~> **Important** Although `https` URLs can be used, there is currently no
mechanism to authenticate the remote server except for general verification of
the server certificate's chain of trust. Data retrieved from servers not under
your control should be treated as untrustworthy.

~> **Note** The `terraform destroy` command destroys the `http` state, but not
the remote HTTP object.

## Example Usage

```hcl
resource "http" "example" {
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

* `method` - (Optional) The HTTP request method to be used. Can either be `GET`,
  `POST`, `PATCH`, `DELETE`, `PUT`, `HEAD`, `OPTIONS`, `CONNECT` or `TRACE`.
  Defaults to `GET`.

* `response_status_code` - (Optional) The expected HTTP response status code. If
  the HTTP response status code doesn't corespond the expected one, the data
  source will return an error. Defaults to `200`.

* `request_headers` - (Optional) A map of strings representing additional HTTP
  headers to include in the request.

* `request_body` - (Optional) The request body to be sent. E.g. within the HTTP
  POST request.

* `triggers` - (Optional) A map of arbitrary strings that, when changed, will
  force the HTTP resource to re-create.

## Attributes Reference

The following attributes are exported:

* `body` - The raw body of the HTTP response.
* `headers` - The map of strings representing the HTTP response headers. If
  there are multiple header values returned in response for the same header,
  only the last one will be reflected in the map.
