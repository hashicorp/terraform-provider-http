package provider

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

// TODO: This test fails under Terraform 0.14. It should be uncommented when we
// are able to include Terraform version logic within acceptance tests
// (see https://github.com/hashicorp/terraform-plugin-sdk/issues/776), or when
// 0.14 is removed from the test matrix (see
// https://github.com/hashicorp/terraform-provider-http/pull/74).
//
//func TestDataSource_x509cert(t *testing.T) {
//	testHttpMock := setUpMockHttpServer()
//	defer testHttpMock.server.Close()
//
//	resource.UnitTest(t, resource.TestCase{
//		ProtoV5ProviderFactories: protoV5ProviderFactories(),
//		Steps: []resource.TestStep{
//			{
//				Config: fmt.Sprintf(`
//							data "http" "http_test" {
//  								url = "%s/x509-ca-cert/200"
//							}`, testHttpMock.server.URL),
//				Check: resource.ComposeTestCheckFunc(
//					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "pem"),
//					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
//				),
//			},
//		},
//	})
//}

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
				ExpectError: regexp.MustCompile(`.*value must be one of: \["\\"GET\\"" "\\"POST\\"" "\\"HEAD\\""`),
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
								request_timeout = 5
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
							"\"https://%s.com\": dial tcp: lookup\n"+
							"%s.com: no such host",
						uid.String(), uid.String(), uid.String(),
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
									min_delay = %d
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
									max_delay = 300
								}
							}`, svr.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
					resource.TestCheckResourceAttr("data.http.http_test", "retry.max_delay", "300"),
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
									min_delay = 300
									max_delay = 200
								}
							}`, svr.URL),
				ExpectError: regexp.MustCompile("Attribute retry.max_delay value must be at least sum of <.min_delay, got: 200"),
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
