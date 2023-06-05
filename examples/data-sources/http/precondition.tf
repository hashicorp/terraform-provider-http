# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "http" "example" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"

  # Optional request headers
  request_headers = {
    Accept = "application/json"
  }
}

resource "random_uuid" "example" {
  lifecycle {
    precondition {
      condition     = contains([201, 204], data.http.example.status_code)
      error_message = "Status code invalid"
    }
  }
}
