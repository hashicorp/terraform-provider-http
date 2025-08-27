resource "http" "example" {
  url = "https://checkpoint-api.hashicorp.com/v1/check/terraform"

  request_headers = {
    Accept = "application/json"
  }
}

output "example_status_code" {
  value = http.example.status_code
}


