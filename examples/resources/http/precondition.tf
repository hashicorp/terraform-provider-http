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


