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

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestDataSource_200(t *testing.T) {
	testHttpMock := setUpMockHttpServer(false)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s/200"
							}`, testHttpMock.server.URL),
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
	testHttpMock := setUpMockHttpServer(false)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s/404"
							}`, testHttpMock.server.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", ""),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "404"),
				),
			},
		},
	})
}

func TestDataSource_withAuthorizationRequestHeader_200(t *testing.T) {
	testHttpMock := setUpMockHttpServer(false)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s/restricted"

								request_headers = {
									"Authorization" = "Zm9vOmJhcg=="
								}
							}`, testHttpMock.server.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_withAuthorizationRequestHeader_403(t *testing.T) {
	testHttpMock := setUpMockHttpServer(false)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s/restricted"

  								request_headers = {
    								"Authorization" = "unauthorized"
  								}
							}`, testHttpMock.server.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", ""),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "403"),
				),
			},
		},
	})
}

func TestDataSource_utf8_200(t *testing.T) {
	testHttpMock := setUpMockHttpServer(false)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s/utf-8/200"
							}`, testHttpMock.server.URL),
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
	testHttpMock := setUpMockHttpServer(false)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s/utf-16/200"
							}`, testHttpMock.server.URL),
				// This should now be a warning, but unsure how to test for it...
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
	testHttpMock := setUpMockHttpServer(false)
	defer testHttpMock.server.Close()

	resource.Test(t, resource.TestCase{
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
								url = "%s/200"
							}`, testHttpMock.server.URL),
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
								url = "%s/200"
							}`, testHttpMock.server.URL),
				PlanOnly: true,
			},
			{
				ProtoV5ProviderFactories: protoV5ProviderFactories(),
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s/200"
							}`, testHttpMock.server.URL),
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
	t.Parallel()

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A Content-Type that does not raise a warning in the Read function must be set in
		// order to prevent test failure under TF 0.14.x as warnings result in no output
		// being written which causes the local-exec command to fail with "Error:
		// local-exec provisioner command must be a non-empty string".
		// See https://github.com/hashicorp/terraform-provider-http/pull/74
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	resource.Test(t, resource.TestCase{
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
							}`, svr.URL),
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
							}`, svr.URL),
				Check: resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
			},
		},
	})
}

func TestDataSource_POST_201(t *testing.T) {
	testHttpMock := setUpMockHttpServer(false)

	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
 								url = "%s/create"
								method = "POST" 
							}`, testHttpMock.server.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "created"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "201"),
				),
			},
		},
	})
}

func TestDataSource_HEAD_204(t *testing.T) {
	testHttpMock := setUpMockHttpServer(false)

	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
 								url = "%s/head"
								method = "HEAD" 
							}`, testHttpMock.server.URL),
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
	testHttpMock := setUpMockHttpServer(false)

	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
 								url = "%s/200"
								method = "OPTIONS" 
							}`, testHttpMock.server.URL),
				ExpectError: regexp.MustCompile(`.*value must be one of: \["\\"GET\\"" "\\"POST\\"" "\\"HEAD\\""`),
			},
		},
	})
}

func TestDataSource_WithCACertificate(t *testing.T) {
	testHttpMock := setUpMockHttpServer(true)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s/200"

  								ca_cert_pem = <<EOF
%s
EOF
							}`, testHttpMock.server.URL, CertToPEM(testHttpMock.server.Certificate())),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_WithCACertificateFalse(t *testing.T) {
	testHttpMock := setUpMockHttpServer(true)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s/200"

  								ca_cert_pem = "invalid"
							}`, testHttpMock.server.URL),
				ExpectError: regexp.MustCompile(`Can't add the CA certificate to certificate pool. Only PEM encoded\ncertificates are supported.`),
			},
		},
	})
}

func TestDataSource_InsecureTrue(t *testing.T) {
	testHttpMock := setUpMockHttpServer(true)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s/200"

  								insecure = true
							}`, testHttpMock.server.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

func TestDataSource_InsecureFalse(t *testing.T) {
	testHttpMock := setUpMockHttpServer(true)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s/200"

  								insecure = false
							}`, testHttpMock.server.URL),
				ExpectError: regexp.MustCompile(fmt.Sprintf(`Error making request: Get "%s/200": x509: `, testHttpMock.server.URL)),
			},
		},
	})
}

func TestDataSource_InsecureUnconfigured(t *testing.T) {
	testHttpMock := setUpMockHttpServer(true)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s/200"
							}`, testHttpMock.server.URL),
				ExpectError: regexp.MustCompile(fmt.Sprintf(`Error making request: Get "%s/200": x509: `, testHttpMock.server.URL)),
			},
		},
	})
}

func TestDataSource_UnsupportedInsecureCaCert(t *testing.T) {
	testHttpMock := setUpMockHttpServer(true)
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
 								url = "%s/200"
								insecure = true
								ca_cert_pem = "invalid"
							}`, testHttpMock.server.URL),
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

	resource.UnitTest(t, resource.TestCase{
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
					CheckServerAndProxyRequestCount(&proxyRequests, &serverRequests),
				),
			},
		},
	})
}

func CheckServerAndProxyRequestCount(proxyRequestCount, serverRequestCount *int) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if *proxyRequestCount != *serverRequestCount {
			return fmt.Errorf("expected proxy and server request count to match: proxy was %d, while server was %d", *proxyRequestCount, *serverRequestCount)
		}

		return nil
	}
}

type TestHttpMock struct {
	server *httptest.Server
}

func setUpMockHttpServer(tls bool) *TestHttpMock {
	var Server *httptest.Server

	if tls {
		Server = httptest.NewTLSServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpReqHandler(w, r)
			}),
		)
	} else {
		Server = httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpReqHandler(w, r)
			}),
		)
	}

	return &TestHttpMock{
		server: Server,
	}
}

func httpReqHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Add("X-Single", "foobar")
	w.Header().Add("X-Double", "1")
	w.Header().Add("X-Double", "2")

	switch r.URL.Path {
	case "/200":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1.0.0"))
	case "/restricted":
		if r.Header.Get("Authorization") == "Zm9vOmJhcg==" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("1.0.0"))
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	case "/utf-8/200":
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1.0.0"))
	case "/utf-16/200":
		w.Header().Set("Content-Type", "application/json; charset=UTF-16")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1.0.0"))
	case "/x509-ca-cert/200":
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pem"))
	case "/create":
		if r.Method == "POST" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("created"))
		}
	case "/head":
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// CertToPEM is a utility function returns a PEM encoded x509 Certificate.
func CertToPEM(cert *x509.Certificate) string {
	certPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))

	return strings.Trim(certPem, "\n")
}
