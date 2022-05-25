package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestDataSource_http200(t *testing.T) {
	testHttpMock := setUpMockHttpServer()

	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: testProviders(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(testDataSourceConfigBasic, testHttpMock.server.URL, 200),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "body", "1.0.0"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Single", "foobar"),
					resource.TestCheckResourceAttr("data.http.http_test", "response_headers.X-Double", "1, 2"),
				),
			},
		},
	})
}

func TestDataSource_http404(t *testing.T) {
	testHttpMock := setUpMockHttpServer()

	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: testProviders(),
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(testDataSourceConfigBasic, testHttpMock.server.URL, 404),
				ExpectError: regexp.MustCompile("HTTP request error. Response code: 404"),
			},
		},
	})
}

func TestDataSource_withHeaders200(t *testing.T) {
	testHttpMock := setUpMockHttpServer()

	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: testProviders(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(testDataSourceConfigWithHeaders, testHttpMock.server.URL, 200),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "body", "1.0.0"),
				),
			},
		},
	})
}

func TestDataSource_utf8(t *testing.T) {
	testHttpMock := setUpMockHttpServer()

	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: testProviders(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(testDataSourceConfigUTF8, testHttpMock.server.URL, 200),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "body", "1.0.0"),
				),
			},
		},
	})
}

func TestDataSource_utf16(t *testing.T) {
	testHttpMock := setUpMockHttpServer()

	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: testProviders(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(testDataSourceConfigUTF16, testHttpMock.server.URL, 200),
				// This should now be a warning, but unsure how to test for it...
				//ExpectWarning: regexp.MustCompile("Content-Type is not a text type. Got: application/json; charset=UTF-16"),
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
//   url = "%s/x509/cert.pem"
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

const testDataSourceConfigBasic = `
data "http" "http_test" {
  url = "%s/meta_%d.txt"
}
`

const testDataSourceConfigWithHeaders = `
data "http" "http_test" {
  url = "%s/restricted/meta_%d.txt"

  request_headers = {
    "Authorization" = "Zm9vOmJhcg=="
  }
}
`

const testDataSourceConfigUTF8 = `
data "http" "http_test" {
  url = "%s/utf-8/meta_%d.txt"
}
`

const testDataSourceConfigUTF16 = `
data "http" "http_test" {
  url = "%s/utf-16/meta_%d.txt"
}
`

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
			if r.URL.Path == "/meta_200.txt" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("1.0.0"))
			} else if r.URL.Path == "/restricted/meta_200.txt" {
				if r.Header.Get("Authorization") == "Zm9vOmJhcg==" {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("1.0.0"))
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			} else if r.URL.Path == "/utf-8/meta_200.txt" {
				w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("1.0.0"))
			} else if r.URL.Path == "/utf-16/meta_200.txt" {
				w.Header().Set("Content-Type", "application/json; charset=UTF-16")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("\"1.0.0\""))
			} else if r.URL.Path == "/x509/cert.pem" {
				w.Header().Set("Content-Type", "application/x-x509-ca-cert")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("pem"))
			} else if r.URL.Path == "/meta_404.txt" {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}),
	)

	return &TestHttpMock{
		server: Server,
	}
}
