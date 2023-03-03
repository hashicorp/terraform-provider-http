# HTTP Provider Design

The HTTP Provider offers functionality for interacting with generic HTTP servers as part of terraform configuration.
Specifically, the provider issues an HTTP GET request to the defined URL on every Terraform run.

Below we have a collection of _Goals_ and _Patterns_: they represent the guiding principles applied during the
development of this provider. Some are in place, others are ongoing processes, others are still just inspirational.

## Goals

* [_Stability over features_](.github/CONTRIBUTING.md)
* Provide managed data source to issue HTTP GET request. The underlying default
[transport](https://pkg.go.dev/net/http#Transport) uses [HTTP/1.1](https://datatracker.ietf.org/doc/html/rfc2616) for
HTTP URLs and either [HTTP/1.1](https://datatracker.ietf.org/doc/html/rfc2616) or 
[HTTP/2.0](https://datatracker.ietf.org/doc/html/rfc7540) for HTTPS URLs depending on whether the server supports
[HTTP/2.0](https://datatracker.ietf.org/doc/html/rfc7540). Non-standard protocols (e.g., 
[SPDY](https://tools.ietf.org/id/draft-ietf-httpbis-http2-00.html), 
[QUIC](https://datatracker.ietf.org/doc/html/draft-ietf-quic-transport-34)) are not supported.
* Support usage of either `http` (plaintext) or `https` (secure) requests. The current version of this provider is 
built with [Go 1.19](https://go.dev/doc/go1.19) which [supports](https://go.dev/doc/go1.18#tls10) 
[TLS/1.0](https://www.ietf.org/rfc/rfc2246.txt) ([deprecated](https://datatracker.ietf.org/doc/rfc8996/)), 
[TLS/1.1](https://datatracker.ietf.org/doc/html/rfc4346) ([deprecated](https://datatracker.ietf.org/doc/rfc8996/)), 
[TLS/1.2](https://datatracker.ietf.org/doc/html/rfc5246) and 
[TLS/1.3](https://datatracker.ietf.org/doc/html/rfc8446). TLS support will track the version of Go that the provider
is built with and will likely change over time.
* Support the supplying of request headers.
* Expose response headers returned from request.
* Expose response body as string where applicable.

## Patterns

Specific to this provider:

* Only idempotent GET requests are supported.
* Only 200 status codes are considered successful.

General to development:

* **Avoid repetition**: the entities managed can sometimes require similar pieces of logic and/or schema to be realised.
  When this happens it's important to keep the code shared in communal sections, so to avoid having to modify code in
  multiple places when they start changing.
* **Test expectations as well as bugs**: While it's typical to write tests to exercise a new functionality, it's key to
  also provide tests for issues that get identified and fixed, so to prove resolution as well as avoid regression.
* **Automate boring tasks**: Processes that are manual, repetitive and can be automated, should be. In addition to be a
  time-saving practice, this ensures consistency and reduces human error (ex. static code analysis).
* **Semantic versioning**: Adhering to HashiCorp's own
  [Versioning Specification](https://www.terraform.io/plugin/sdkv2/best-practices/versioning#versioning-specification)
  ensures we provide a consistent practitioner experience, and a clear process to deprecation and decommission.
