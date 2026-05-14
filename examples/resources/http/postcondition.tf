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


