# Ensure an 'Accept: application/json' header is present on all
# checkpoint-api.hashicorp.com requests.
provider "http" {
  # Optional host configuration
  host {
    name = "checkpoint-api.hashicorp.com"

    request_headers = {
      Accept = "application/json"
    }
  }
}

data "http" "example" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"
}
