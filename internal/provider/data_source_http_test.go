package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestDataSource_200(t *testing.T) {
	testHttpMock := setUpMockHttpServer()
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
	testHttpMock := setUpMockHttpServer()
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
	testHttpMock := setUpMockHttpServer()
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
	testHttpMock := setUpMockHttpServer()
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
	testHttpMock := setUpMockHttpServer()
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
	testHttpMock := setUpMockHttpServer()
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
	testHttpMock := setUpMockHttpServer()
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
	testHttpMock := setUpMockHttpServer()

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
	testHttpMock := setUpMockHttpServer()

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
	testHttpMock := setUpMockHttpServer()

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
				ExpectError: regexp.MustCompile(`.*Value must be one of: \["\\"GET\\"" "\\"POST\\"" "\\"HEAD\\""`),
			},
		},
	})
}

func TestDataSource_ResponseBodyText(t *testing.T) {
	t.Parallel()

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`你好世界`)) // Hello world
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	resource.UnitTest(t, resource.TestCase{
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
	t.Parallel()

	// 1 x 1 transparent gif pixel.
	const transPixel = "\x47\x49\x46\x38\x39\x61\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\x00\x00\x00\x21\xF9\x04\x01\x00\x00\x00\x00\x2C\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02\x44\x01\x00\x3B"

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write([]byte(transPixel))
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	resource.UnitTest(t, resource.TestCase{
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

type TestHttpMock struct {
	server *httptest.Server
}

func setUpMockHttpServer() *TestHttpMock {
	Server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}),
	)

	return &TestHttpMock{
		server: Server,
	}
}
