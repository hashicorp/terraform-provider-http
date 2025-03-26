// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataSource_200(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Single", "foobar")
		w.Header().Add("X-Double", "1")
		w.Header().Add("X-Double", "2")
		_, err := w.Write([]byte("1.0.0"))
		if err != nil {
			t.Errorf("error writing body: %s", err)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s"
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.Content-Type", "text/plain"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Single", "foobar"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Double", "1, 2"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_200_SlashInPath(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Single", "foobar")
		w.Header().Add("X-Double", "1")
		w.Header().Add("X-Double", "2")
		_, err := w.Write([]byte("1.0.0"))
		if err != nil {
			t.Errorf("error writing body: %s", err)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s/200"
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.Content-Type", "text/plain"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Single", "foobar"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Double", "1, 2"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_404(t *testing.T) {
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
							data "http" "http_test" {
								url = "%s"
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", ""),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "404"),
				),
			},
		},
	})
}

func TestDataSource_withAuthorizationRequestHeader_200(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Zm9vOmJhcg==" {
			w.Header().Set("Content-Type", "text/plain")
			_, err := w.Write([]byte("1.0.0"))
			if err != nil {
				t.Errorf("error writing body: %s", err)
			}
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
							data "http" "http_test" {
								url = "%s"

								request_headers = {
									"Authorization" = "Zm9vOmJhcg=="
								}
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_withAuthorizationRequestHeader_403(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Zm9vOmJhcg==" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"

  								request_headers = {
    								"Authorization" = "unauthorized"
  								}
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", ""),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "403"),
				),
			},
		},
	})
}

func TestDataSource_utf8_200(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		_, err := w.Write([]byte("1.0.0"))
		if err != nil {
			t.Errorf("error writing body: %s", err)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.Content-Type", "text/plain; charset=UTF-8"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_utf16_200(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=UTF-16")
		_, err := w.Write([]byte("1.0.0"))
		if err != nil {
			t.Errorf("error writing body: %s", err)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"
							}`, testServer.URL),
				// TODO: ExpectWarning can be used once https://github.com/hashicorp/terraform-plugin-testing/pull/17
				// is merged and released.
				// ExpectWarning: regexp.MustCompile("Content-Type is not a text type. Got: application/json; charset=UTF-16"),
			},
		},
	})
}

func TestDataSource_x509cert(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("pem"))
		if err != nil {
			t.Errorf("error writing body: %s", err)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
 								url = "%s/x509-ca-cert/200"
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "pem"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_UpgradeFromVersion2_2_0(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Single", "foobar")
		w.Header().Add("X-Double", "1")
		w.Header().Add("X-Double", "2")
		_, err := w.Write([]byte("1.0.0"))
		if err != nil {
			t.Errorf("error writing body: %s", err)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"http": {
						VersionConstraint: "2.2.0",
						Source:            "hashicorp/http",
					},
				},
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s"
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.Content-Type", "text/plain"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Single", "foobar"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Double", "1, 2"),
				),
			},
			{
				ProtoV5ProviderFactories: protoV5ProviderFactories(),
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s"
							}`, testServer.URL),
				PlanOnly: true,
			},
			{
				ProtoV5ProviderFactories: protoV5ProviderFactories(),
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s"
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.Content-Type", "text/plain"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Single", "foobar"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Double", "1, 2"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_Provisioner(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A Content-Type that does not raise a warning in the Read function must be set in
		// order to prevent test failure under TF 0.14.x as warnings result in no output
		// being written which causes the local-exec command to fail with "Error:
		// local-exec provisioner command must be a non-empty string".
		// See https://github.com/hashicorp/terraform-provider-http/pull/74
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
							data "http" "http_test" {
								url = "%s"
							}
							resource "null_resource" "example" {
  								provisioner "local-exec" {
    								command = contains([201, 204], data.http.http_test.status_code)
  								}
							}`, testServer.URL),
				ExpectError: regexp.MustCompile(`Error running command 'false': exit status 1. Output:`),
			},
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s"
							}
							resource "null_resource" "example" {
  								provisioner "local-exec" {
    								command = contains([200], data.http.http_test.status_code)
  								}
							}`, testServer.URL),
				Check: resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
			},
		},
	})
}

func TestDataSource_POST_200(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "text/plain")
			_, err := w.Write([]byte("created"))
			if err != nil {
				t.Errorf("error writing body: %s", err)
			}
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s"
								method = "POST"
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "created"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_HEAD_204(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("X-Single", "foobar")
			w.Header().Add("X-Double", "1")
			w.Header().Add("X-Double", "2")
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
 								url = "%s"
								method = "HEAD" 
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.Content-Type", "text/plain"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Single", "foobar"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Double", "1, 2"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", ""),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_UnsupportedMethod(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
 								url = "%s"
								method = "OPTIONS" 
							}`, testServer.URL),
				ExpectError: regexp.MustCompile(`.*value must be one of: \["GET" "POST" "HEAD"`),
			},
		},
	})
}

func TestDataSource_WithCACertificate(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"

  								ca_cert_pem = <<EOF
%s
EOF
							}`, testServer.URL, certToPEM(testServer.Certificate())),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_WithClientCert(t *testing.T) {
	certfile, keyfile := generateCert(t)
	cert, err := tls.LoadX509KeyPair(certfile, keyfile)
	require.NoError(t, err, "failed to load client certificate")
	testServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.WriteString(w, "OK\n")
		assert.NoError(t, err)
	}))
	clientCAs := x509.NewCertPool()
	clientCAs.AddCert(cert.Leaf)

	testServer.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    clientCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	testServer.StartTLS()
	defer testServer.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "http" "http_test" {
  url = "%s"
  ca_cert_pem = file("%s")
  client_cert_pem = file("%s")
  client_key_pem = file("%s")
}
`, testServer.URL, certfile, certfile, keyfile),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "OK\n"),
				),
			},
			{
				Config: fmt.Sprintf(`
data "http" "http_test" {
  url = "%s"
  ca_cert_pem = file("%s")
}
`, testServer.URL, certfile),
				ExpectError: regexp.MustCompile(`remote error: tls: certificate`),
			},
		},
	})
}

func TestDataSource_WithCACertificateFalse(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"

  								ca_cert_pem = "invalid"
							}`, testServer.URL),
				ExpectError: regexp.MustCompile(`Can't add the CA certificate to certificate pool. Only PEM encoded\ncertificates are supported.`),
			},
		},
	})
}

func TestDataSource_InsecureTrue(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"

  								insecure = true
							}`, testServer.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_InsecureFalse(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"

  								insecure = false
							}`, testServer.URL),
				ExpectError: regexp.MustCompile(
					fmt.Sprintf(
						"Error making request: GET %s giving up after 1\n"+
							"attempt\\(s\\): Get \"%s\": ",
						testServer.URL,
						testServer.URL,
					),
				),
			},
		},
	})
}

func TestDataSource_InsecureUnconfigured(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"
							}`, testServer.URL),
				ExpectError: regexp.MustCompile(
					fmt.Sprintf(
						"Error making request: GET %s giving up after 1\n"+
							"attempt\\(s\\): Get \"%s\": ",
						testServer.URL,
						testServer.URL,
					),
				),
			},
		},
	})
}

func TestDataSource_UnsupportedInsecureCaCert(t *testing.T) {
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
 								url = "%s"
								insecure = true
								ca_cert_pem = "invalid"
							}`, testServer.URL),
				ExpectError: regexp.MustCompile(`Attribute "insecure" cannot be specified when "ca_cert_pem" is specified`),
			},
		},
	})
}

func TestDataSource_HostRequestHeaderOverride_200(t *testing.T) {
	altHost := "alt-test-host"

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != altHost {
			w.WriteHeader(400)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Single", "foobar")
		w.Header().Add("X-Double", "1")
		w.Header().Add("X-Double", "2")
		_, err := w.Write([]byte("1.0.0"))
		if err != nil {
			t.Errorf("error writing body: %s", err)
		}
	}))
	defer testServer.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s"
								request_headers = {
									"Host" = "%s"
								}
							}`, testServer.URL, altHost),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.Content-Type", "text/plain"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Single", "foobar"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Double", "1, 2"),
				),
			},
		},
	})
}

// testProxiedURL is a hardcoded URL used in acceptance testing where it is
// expected that a locally started HTTP proxy will handle the request.
//
// Neither localhost nor the loopback interface (127.0.0.1) can be used for the
// address of the server as httpproxy/proxy.go will ignore these addresses.
//
// References:
//   - https://cs.opensource.google/go/x/net/+/internal-branch.go1.19-vendor:http/httpproxy/proxy.go;l=181
//   - https://cs.opensource.google/go/x/net/+/internal-branch.go1.19-vendor:http/httpproxy/proxy.go;l=186
const testProxiedURL = "http://terraform-provider-http-test-proxy"

func TestDataSource_HTTPViaProxyWithEnv(t *testing.T) {
	proxyRequests := 0
	serverRequests := 0

	// Content-Type is set to text/plain otherwise the http data source issues a warning which
	// causes Terraform 0.14 to not write any data to state.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverRequests++
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	}))

	defer server.Close()

	serverURL, err := url.Parse(server.URL)

	if err != nil {
		t.Fatalf("error parsing server URL: %s", err)
	}

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests++
		httputil.NewSingleHostReverseProxy(serverURL).ServeHTTP(w, r)
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)

	resource.Test(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),

		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "http" "http_test" {
						url = "%s"
					}
				`, testProxiedURL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
					checkServerAndProxyRequestCount(&proxyRequests, &serverRequests),
				),
			},
		},
	})
}

func TestDataSource_Timeout(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(10) * time.Millisecond)
	}))
	defer svr.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"
								request_timeout_ms = 5
							}`, svr.URL),
				ExpectError: regexp.MustCompile(`request exceeded the specified timeout: 5ms`),
			},
		},
	})
}

func TestDataSource_Retry(t *testing.T) {
	uid := uuid.New()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "https://%s.com"
								retry {
									attempts = 1
								}
							}`, uid.String()),
				ExpectError: regexp.MustCompile(
					fmt.Sprintf(
						"Error making request: GET https://%s.com\n"+
							"giving up after 2 attempt\\(s\\): Get\n"+
							"\"https://%s.com\": dial tcp: lookup",
						uid.String(), uid.String(),
					),
				),
			},
		},
	})
}

func TestDataSource_MinDelay(t *testing.T) {
	var timeOfFirstRequest, timeOfSecondRequest int64
	minDelay := 200

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")

		if timeOfFirstRequest == 0 {
			timeOfFirstRequest = time.Now().UnixNano() / int64(time.Millisecond)
			w.WriteHeader(http.StatusBadGateway)
		} else {
			timeOfSecondRequest = time.Now().UnixNano() / int64(time.Millisecond)
		}
	}))
	defer svr.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"
								retry {
									attempts = 1
									min_delay_ms = %d
								}
							}`, svr.URL, minDelay),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
					checkMinDelay(&timeOfFirstRequest, &timeOfSecondRequest, minDelay),
				),
			},
		},
	})
}

// TestDataSource_MaxDelay does not evaluate the maximum delay between requests owing to the
// non-deterministic behaviour of request duration in different environments (e.g., CI).
func TestDataSource_MaxDelay(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
	}))
	defer svr.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"
								retry {
									attempts = 1
									max_delay_ms = 300
								}
							}`, svr.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
					resource.TestCheckResourceAttr("data.http.http_test", "retry.max_delay_ms", "300"),
				),
			},
		},
	})
}

func TestDataSource_MaxDelayAtLeastEqualToMinDelay(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer svr.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s"
								retry {
									attempts = 1
									min_delay_ms = 300
									max_delay_ms = 200
								}
							}`, svr.URL),
				ExpectError: regexp.MustCompile("Attribute retry.max_delay_ms value must be at least sum of <.min_delay_ms,\ngot: 200"),
			},
		},
	})
}

// Reference: https://github.com/hashicorp/terraform-provider-http/issues/388
func TestDataSource_RequestBody(t *testing.T) {
	t.Parallel()

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")

		requestBody, err := io.ReadAll(r.Body)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`Request Body Read Error: ` + err.Error()))

			return
		}

		// If the request body is empty or a test string, return a 200 OK with a response body.
		if len(requestBody) == 0 || string(requestBody) == "test" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`test response body`))

			return
		}

		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`request body (` + string(requestBody) + `) was not empty or "test"`))
	}))
	defer svr.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "http" "test" {
						url = "%s"
					}`, svr.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.test", "status_code", "200"),
				),
			},
			{
				Config: fmt.Sprintf(`
					data "http" "test" {
						request_body = "test"
						url          = %q
					}`, svr.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.test", "status_code", "200"),
				),
			},
			{
				Config: fmt.Sprintf(`
					data "http" "test" {
						request_body = "not-test"
						url          = %q
					}`, svr.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.test", "status_code", "400"),
				),
			},
		},
	})
}

func TestDataSource_ResponseBodyText(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`你好世界`)) // Hello world
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s"
							}`, svr.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "你好世界"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_body_base64", "5L2g5aW95LiW55WM"),
				),
			},
		},
	})
}

func TestDataSource_ResponseBodyBinary(t *testing.T) {
	// 1 x 1 transparent gif pixel.
	const transPixel = "\x47\x49\x46\x38\x39\x61\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\x00\x00\x00\x21\xF9\x04\x01\x00\x00\x00\x00\x2C\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02\x44\x01\x00\x3B"

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write([]byte(transPixel))
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	resource.ParallelTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s"
							}`, svr.URL),
				Check: resource.ComposeTestCheckFunc(
					// Note the replacement character in the string representation in `response_body`.
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "GIF89a\x01\x00\x01\x00�\x00\x00\x00\x00\x00\x00\x00\x00!�\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_body_base64", "R0lGODlhAQABAIAAAAAAAAAAACH5BAEAAAAALAAAAAABAAEAAAICRAEAOw=="),
				),
			},
		},
	})
}

func checkServerAndProxyRequestCount(proxyRequestCount, serverRequestCount *int) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if *proxyRequestCount != *serverRequestCount {
			return fmt.Errorf("expected proxy and server request count to match: proxy was %d, while server was %d", *proxyRequestCount, *serverRequestCount)
		}

		return nil
	}
}

// certToPEM is a utility function returns a PEM encoded x509 Certificate.
func certToPEM(cert *x509.Certificate) string {
	certPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))

	return strings.Trim(certPem, "\n")
}

func checkMinDelay(timeOfFirstRequest, timeOfSecondRequest *int64, minDelay int) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if *timeOfFirstRequest != *timeOfSecondRequest {
			diff := *timeOfSecondRequest - *timeOfFirstRequest

			if diff < int64(minDelay) {
				return fmt.Errorf("expected delay between requests to be at least: %dms, was actually: %dms", minDelay, diff)
			}
		}

		return nil
	}
}
