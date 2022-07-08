# This example makes an HTTP request to `website.com`
# via an HTTP proxy at `corporate.proxy.service`.

provider "http" {
  proxy {
    url = "https://corporate.proxy.service"
  }
}

data "http" "test" {
  url = "https://website.com"
}
