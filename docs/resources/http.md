---
page_title: "http Resource - terraform-provider-http"
subcategory: ""
description: |-
  The http resource makes an HTTP request to the given URL and exports
  information about the response.
  The given URL may be either an http or https URL. This resource
  will issue a warning if the result is not UTF-8 encoded.
  ~> Important Although https URLs can be used, there is currently no
  mechanism to authenticate the remote server except for general verification of
  the server certificate's chain of trust. Data retrieved from servers not under
  your control should be treated as untrustworthy.
  By default, there are no retries. Configuring the retry block will result in
  retries if an error is returned by the client (e.g., connection errors) or if
  a 5xx-range (except 501) status code is received. For further details see
  go-retryablehttp https://pkg.go.dev/github.com/hashicorp/go-retryablehttp.
---

# http (Resource)

The `http` resource makes an HTTP request to the given URL and exports
information about the response.

The given URL may be either an `http` or `https` URL. This resource
will issue a warning if the result is not UTF-8 encoded.

~> **Important** Although `https` URLs can be used, there is currently no
mechanism to authenticate the remote server except for general verification of
the server certificate's chain of trust. Data retrieved from servers not under
your control should be treated as untrustworthy.

By default, there are no retries. Configuring the retry block will result in
retries if an error is returned by the client (e.g., connection errors) or if 
a 5xx-range (except 501) status code is received. For further details see 
[go-retryablehttp](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp).

## Example Usage

```terraform
# The following example shows how to issue an HTTP request supplying
# an optional request header.
resource "http" "example" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"

  # Optional request headers
  request_headers = {
    Accept = "application/json"
  }
}

# The following example shows how to issue an HTTP HEAD request.
resource "http" "example_head" {
  url    = "https://checkpoint-api.hashicorp.com/v1/check/terraform"
  method = "HEAD"
}

# The following example shows how to issue an HTTP POST request
# supplying an optional request body.
resource "http" "example_post" {
  url    = "https://checkpoint-api.hashicorp.com/v1/check/terraform"
  method = "POST"

  # Optional request body
  request_body = "request body"
}
```

## Controlling when the request is sent

Use the resource argument `when` to control whether the HTTP request is executed during apply operations or only during destroy:

- `apply` (default): request is executed during create and update.
- `destroy`: request is executed only during resource destruction.

```terraform
# Example 1: HTTP request on apply (default behavior)
resource "http" "example_apply" {
  url = "https://httpbin.org/get"

  request_headers = {
    Accept = "application/json"
  }

  # This is the default behavior - request is sent during apply
  when = "apply"
}

# Example 2: HTTP request only on destroy
resource "http" "example_destroy" {
  url    = "https://httpbin.org/delete"
  method = "DELETE"

  request_headers = {
    Accept = "application/json"
  }

  # Request is only sent during resource destruction
  when = "destroy"
}

# Example 3: Default behavior (no when attribute specified)
resource "http" "example_default" {
  url = "https://httpbin.org/get"

  request_headers = {
    Accept = "application/json"
  }

  # No "when" attribute specified - defaults to "apply"
}

output "example_apply_status_code" {
  value = http.example_apply.status_code
}

output "example_destroy_status_code" {
  value = http.example_destroy.status_code
}

output "example_default_status_code" {
  value = http.example_default.status_code
}
```

## Usage with Postcondition

Note: Pre/postconditions validate values during apply. They are only meaningful when the resource executes the HTTP request during apply, i.e., when `when = "apply"` (default). If `when = "destroy"`, these conditions will not evaluate against a fresh request result.

[Precondition and Postcondition](https://www.terraform.io/language/expressions/custom-conditions)
checks are available with Terraform v1.2.0 and later.

```terraform
resource "http" "example_postcondition" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"

  request_headers = {
    Accept = "application/json"
  }

  # Ensure the request runs at apply for postconditions
  when = "apply"

  lifecycle {
    postcondition {
      condition     = contains([201, 204], self.status_code)
      error_message = "Status code invalid"
    }
  }
}

output "postcondition_status_code" {
  value = http.example_postcondition.status_code
}
```

## Usage with Precondition

Note: Pre/postconditions validate values during apply. They are only meaningful when the resource executes the HTTP request during apply, i.e., when `when = "apply"` (default). If `when = "destroy"`, these conditions will not evaluate against a fresh request result.

[Precondition and Postcondition](https://www.terraform.io/language/expressions/custom-conditions)
checks are available with Terraform v1.2.0 and later.

```terraform
resource "http" "example_precondition" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"

  request_headers = {
    Accept = "application/json"
  }

  # Ensure the request runs at apply for preconditions
  when = "apply"

  lifecycle {
    precondition {
      condition     = contains([200, 201, 204], self.status_code)
      error_message = "Unexpected status code"
    }
  }
}

output "precondition_status_code" {
  value = http.example_precondition.status_code
}
```

## Usage with Provisioner

[Failure Behaviour](https://www.terraform.io/language/resources/provisioners/syntax#failure-behavior)
can be leveraged within a provisioner in order to raise an error and stop applying.

```terraform
resource "http" "example" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"

  request_headers = {
    Accept = "application/json"
  }
}

resource "null_resource" "example" {
  # On success, this will attempt to execute the true command in the
  # shell environment running terraform.
  # On failure, this will attempt to execute the false command in the
  # shell environment running terraform.
  provisioner "local-exec" {
    command = contains([201, 204], http.example.status_code)
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `url` (String) The URL for the request. Supported schemes are `http` and `https`.

### Optional

- `ca_cert_pem` (String) Certificate Authority (CA) in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.
- `client_cert_pem` (String) Client certificate in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.
- `client_key_pem` (String) Client key in [PEM (RFC 1421)](https://datatracker.ietf.org/doc/html/rfc1421) format.
- `insecure` (Boolean) Disables verification of the server's certificate chain and hostname. Defaults to `false`
- `method` (String) The HTTP Method for the request. Allowed methods are a subset of methods defined in [RFC7231](https://datatracker.ietf.org/doc/html/rfc7231#section-4.3) namely, `GET`, `HEAD`, and `POST`. `POST` support is only intended for read-only URLs, such as submitting a search.
- `request_body` (String) The request body as a string.
- `request_headers` (Map of String) A map of request header field names and values.
- `request_timeout_ms` (Number) The request timeout in milliseconds.
- `retry` (Block, Optional) Retry request configuration. By default there are no retries. Configuring this block will result in retries if an error is returned by the client (e.g., connection errors) or if a 5xx-range (except 501) status code is received. For further details see [go-retryablehttp](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp). (see [below for nested schema](#nestedblock--retry))
- `when` (String) When to send the HTTP request. Valid values are `apply` (default) and `destroy`. When set to `apply`, the request is sent during resource creation and updates. When set to `destroy`, the request is only sent during resource destruction.

### Read-Only

- `body` (String, Deprecated) The response body returned as a string. **NOTE**: This is deprecated, use `response_body` instead.
- `id` (String) The URL used for the request.
- `response_body` (String) The response body returned as a string.
- `response_body_base64` (String) The response body encoded as base64 (standard) as defined in [RFC 4648](https://datatracker.ietf.org/doc/html/rfc4648#section-4).
- `response_headers` (Map of String) A map of response header field names and values. Duplicate headers are concatenated according to [RFC2616](https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2).
- `status_code` (Number) The HTTP response status code.

<a id="nestedblock--retry"></a>
### Nested Schema for `retry`

Optional:

- `attempts` (Number) The number of times the request is to be retried. For example, if 2 is specified, the request will be tried a maximum of 3 times.
- `max_delay_ms` (Number) The maximum delay between retry requests in milliseconds.
- `min_delay_ms` (Number) The minimum delay between retry requests in milliseconds.


