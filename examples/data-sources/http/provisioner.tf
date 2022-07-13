data "http" "example" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"

  # Optional request headers
  request_headers = {
    Accept = "application/json"
  }
}

resource "null_resource" "example" {
  provisioner "local-exec" {
    command = contains([201, 204], data.http.example.status_code)
  }
}
