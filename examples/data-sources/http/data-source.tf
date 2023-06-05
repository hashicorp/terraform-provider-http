# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# The following example shows how to issue an HTTP GET request supplying
# an optional request header.
data "http" "example" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"

  # Optional request headers
  request_headers = {
    Accept = "application/json"
  }
}

# The following example shows how to issue an HTTP HEAD request.
data "http" "example_head" {
  url    = "https://checkpoint-api.hashicorp.com/v1/check/terraform"
  method = "HEAD"
}

# The following example shows how to issue an HTTP POST request
# supplying an optional request body.
data "http" "example_post" {
  url    = "https://checkpoint-api.hashicorp.com/v1/check/terraform"
  method = "POST"

  # Optional request body
  request_body = "request body"
}
