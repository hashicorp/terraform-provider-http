# This example makes an HTTP request to `website.com`
# via an HTTP proxy defined through environment variables (see
# https://pkg.go.dev/net/http#ProxyFromEnvironment for details).

provider "http" {
  proxy {
    from_env = true
  }
}

data "http" "test" {
  url = "https://website.com"
}
