package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestDataSource_200(t *testing.T) {
	testHttpMock := setUpMockHttpServer()

	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: testProviders(),
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
		ProviderFactories: testProviders(),
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
		ProviderFactories: testProviders(),
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
		ProviderFactories: testProviders(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "http" "http_test" {
  url = "%s/restricted"

  request_headers = {
    "Authorization" = "unauthorized"
  }
}
`, testHttpMock.server.URL),
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
		ProviderFactories: testProviders(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "http" "http_test" {
  url = "%s/utf-8/200"
}
`, testHttpMock.server.URL),
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
		ProviderFactories: testProviders(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "http" "http_test" {
  url = "%s/utf-16/200"
}
`, testHttpMock.server.URL),
				// This should now be a warning, but unsure how to test for it...
				// ExpectWarning: regexp.MustCompile("Content-Type is not a text type. Got: application/json; charset=UTF-16"),
			},
		},
	})
}

// TODO: This test fails under Terraform 0.14. It should be uncommented when we
// are able to include Terraform version logic within acceptance tests, or when
// 0.14 is removed from the test matrix.
// See https://github.com/hashicorp/terraform-provider-http/pull/74
//
// const testDataSourceConfig_x509cert = `
// data "http" "http_test" {
//   url = "%s/x509-ca-cert/200"
// }

// output "body" {
//   value = "${data.http.http_test.body}"
// }
// `

// func TestDataSource_x509cert(t *testing.T) {
// 	testHttpMock := setUpMockHttpServer()

// 	defer testHttpMock.server.Close()

// 	resource.UnitTest(t, resource.TestCase{
// 		Providers: testProviders,
// 		Steps: []resource.TestStep{
// 			{
// 				Config: fmt.Sprintf(testDataSourceConfig_x509cert, testHttpMock.server.URL),
// 				Check: func(s *terraform.State) error {
// 					_, ok := s.RootModule().Resources["data.http.http_test"]
// 					if !ok {
// 						return fmt.Errorf("missing data resource")
// 					}

// 					outputs := s.RootModule().Outputs

// 					if outputs["body"].Value != "pem" {
// 						return fmt.Errorf(
// 							`'body' output is %s; want 'pem'`,
// 							outputs["body"].Value,
// 						)
// 					}

// 					return nil
// 				},
// 			},
// 		},
// 	})
// }

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
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}),
	)

	return &TestHttpMock{
		server: Server,
	}
}
