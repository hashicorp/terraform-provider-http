---
layout: "http"
page_title: "Provider: HTTP"
sidebar_current: "docs-http-index"
description: |-
  The HTTP provider interacts with HTTP servers.
---

# HTTP Provider

The HTTP provider is a utility provider for interacting with generic HTTP
servers as part of a Terraform configuration.

## Configuration Reference

The following environment variables are supported:

* `HTTP_DATA_IS_SENSITIVE` - (Optional) By default the request/response body
  and headers aren't considered as sensitive. If you want to hide data's value
  in terraform output you would need to set environment variable
  `HTTP_DATA_IS_SENSITIVE=true`.
