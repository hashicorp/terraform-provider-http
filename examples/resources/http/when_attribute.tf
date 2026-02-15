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


