// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func TestResource_200(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Single", "foobar")
		w.Header().Add("X-Double", "1")
		w.Header().Add("X-Double", "2")
		_, _ = w.Write([]byte("1.0.0"))
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
                    resource "http" "http_test" {
                        url = "%s"
                    }`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("http.http_test", "response_headers.Content-Type", "text/plain"),
					resource.TestCheckResourceAttr("http.http_test", "response_headers.X-Single", "foobar"),
					resource.TestCheckResourceAttr("http.http_test", "response_headers.X-Double", "1, 2"),
					resource.TestCheckResourceAttr("http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestResource_200_SlashInPath(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("1.0.0"))
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
                    resource "http" "http_test" {
                        url = "%s/200"
                    }`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("http.http_test", "response_headers.Content-Type", "text/plain"),
					resource.TestCheckResourceAttr("http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestResource_404(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
                    resource "http" "http_test" {
                        url = "%s"
                    }`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("http.http_test", "response_body", ""),
					resource.TestCheckResourceAttr("http.http_test", "status_code", "404"),
				),
			},
		},
	})
}

func TestResource_withAuthorizationRequestHeader_200(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Zm9vOmJhcg==" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("1.0.0"))
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
                    resource "http" "http_test" {
                        url = "%s"

                        request_headers = {
                            "Authorization" = "Zm9vOmJhcg=="
                        }
                    }`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestResource_POST_200(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("created"))
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
                    resource "http" "http_test" {
                        url    = "%s"
                        method = "POST"

                        request_body = "request body"
                    }`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("http.http_test", "response_body", "created"),
					resource.TestCheckResourceAttr("http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestResource_Provisioner(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		ExternalProviders: map[string]resource.ExternalProvider{
			"null": {
				VersionConstraint: "3.1.1",
				Source:            "hashicorp/null",
			},
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
                    resource "http" "http_test" {
                        url = "%s"
                    }
                    resource "null_resource" "example" {
                        provisioner "local-exec" {
                            command = contains([201, 204], http.http_test.status_code)
                        }
                    }`, testServer.URL),
				ExpectError: regexp.MustCompile(`Error running command 'false': exit status 1. Output:`),
			},
			{
				Config: fmt.Sprintf(`
                    resource "http" "http_test" {
                        url = "%s"
                    }
                    resource "null_resource" "example" {
                        provisioner "local-exec" {
                            command = contains([200], http.http_test.status_code)
                        }
                    }`, testServer.URL),
				Check: resource.TestCheckResourceAttr("http.http_test", "status_code", "200"),
			},
		},
	})
}

func TestResource_x509cert_skip_on_tf_014(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pem"))
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBetween(tfversion.Version0_14_0, tfversion.Version0_15_0),
		},
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
                    resource "http" "http_test" {
                        url = "%s/x509-ca-cert/200"
                    }`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("http.http_test", "response_body", "pem"),
					resource.TestCheckResourceAttr("http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestResource_WhenAttribute_Apply(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("test response"))
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "http" "http_test" {
						url = "%s"
						when = "apply"
					}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("http.http_test", "response_body", "test response"),
					resource.TestCheckResourceAttr("http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestResource_WhenAttribute_Destroy(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("test response"))
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "http" "http_test" {
						url = "%s"
						when = "destroy"
					}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("http.http_test", "response_body", ""),
					resource.TestCheckResourceAttr("http.http_test", "status_code", "0"),
				),
			},
		},
	})
}

func TestResource_WhenAttribute_Default(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("test response"))
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "http" "http_test" {
						url = "%s"
					}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("http.http_test", "response_body", "test response"),
					resource.TestCheckResourceAttr("http.http_test", "status_code", "200"),
				),
			},
		},
	})
}
