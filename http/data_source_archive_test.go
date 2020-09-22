package http

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

type TestArchiveHttpMock struct {
	server *httptest.Server
}

const testDataSourceArchiveConfig_basic = `
data "http_archive" "http_test" {
  url = "%s/meta_%d.tar.gz"
}

output "file_aaa" {
  value = base64decode(data.http_archive.http_test.files["aaa"])
}

output "file_hw_txt" {
  value = base64decode(data.http_archive.http_test.files["hw.txt"])
}

output "response_headers" {
  value = data.http_archive.http_test.response_headers
}
`

func TestDataSourceArchive_http200(t *testing.T) {
	TestArchiveHttpMock := setUpMockArchiveHttpServer()

	defer TestArchiveHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		Providers: testProviders,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(testDataSourceArchiveConfig_basic, TestArchiveHttpMock.server.URL, 200),
				Check: func(s *terraform.State) error {
					_, ok := s.RootModule().Resources["data.http_archive.http_test"]
					if !ok {
						return fmt.Errorf("missing data resource")
					}

					outputs := s.RootModule().Outputs

					expectedAaa := "aaaaaaaaaaaaaaaaa\n"
					if outputs["file_aaa"].Value != expectedAaa {
						return fmt.Errorf(
							`'file_aaa' output is %s; want %s`,
							outputs["file_aaa"].Value,
							expectedAaa,
						)
					}

					expectedHw := "Hello world!\n"
					if outputs["file_hw_txt"].Value != expectedHw {
						return fmt.Errorf(
							`'body' output is %s; want %s`,
							outputs["body"].Value,
							expectedHw,
						)
					}

					response_headers := outputs["response_headers"].Value.(map[string]interface{})

					if response_headers["X-Single"].(string) != "foobar" {
						return fmt.Errorf(
							`'X-Single' response header is %s; want 'foobar'`,
							response_headers["X-Single"].(string),
						)
					}

					if response_headers["X-Double"].(string) != "1, 2" {
						return fmt.Errorf(
							`'X-Double' response header is %s; want '1, 2'`,
							response_headers["X-Double"].(string),
						)
					}

					return nil
				},
			},
		},
	})
}

func TestDataSourceArchive_http404(t *testing.T) {
	TestArchiveHttpMock := setUpMockArchiveHttpServer()

	defer TestArchiveHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		Providers: testProviders,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(testDataSourceArchiveConfig_basic, TestArchiveHttpMock.server.URL, 404),
				ExpectError: regexp.MustCompile("HTTP request error. Response code: 404"),
			},
		},
	})
}

const testDataSourceArchiveConfig_withHeaders = `
data "http_archive" "http_test" {
  url = "%s/restricted/meta_%d.tar.gz"

  request_headers = {
    "Authorization" = "Zm9vOmJhcg=="
  }
}

output "file_hw_txt" {
  value = base64decode(data.http_archive.http_test.files["hw.txt"])
}
`

func TestDataSourceArchive_withHeaders200(t *testing.T) {
	TestArchiveHttpMock := setUpMockArchiveHttpServer()

	defer TestArchiveHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		Providers: testProviders,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(testDataSourceArchiveConfig_withHeaders, TestArchiveHttpMock.server.URL, 200),
				Check: func(s *terraform.State) error {
					_, ok := s.RootModule().Resources["data.http_archive.http_test"]
					if !ok {
						return fmt.Errorf("missing data resource")
					}

					outputs := s.RootModule().Outputs

					expectedHw := "Hello world!\n"
					if outputs["file_hw_txt"].Value != expectedHw {
						return fmt.Errorf(
							`'body' output is %s; want %s`,
							outputs["body"].Value,
							expectedHw,
						)
					}

					return nil
				},
			},
		},
	})
}

func setUpMockArchiveHttpServer() *TestArchiveHttpMock {
	data, err := base64.StdEncoding.DecodeString("H4sIALKQaV8AA+2UTQrDIBCFXfcU9gI6jn9X6DWEJHQhDaSW9PgVE1rIwq5MKZlv8xYOzHMeM0Ky5gCAt5YXdYsCmkVXuNJeo1aAVnNQCp1h3La3xtjjnsKUrXR9uNXqctkwVN7Xf7z1TxAyhNC4R56HM6aSP+Inf1Py94iMQ2NfhYPnH7acfu2I2BMhr7NIz9Syx9f9V3Zz//MV8LT/e3DpYxz5PE6xO9PqEwRBHIcX+hulWQAOAAA=")

	if err != nil {
		log.Fatal(err)
	}

	Server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			w.Header().Set("Content-Type", "application/gzip")
			w.Header().Add("X-Single", "foobar")
			w.Header().Add("X-Double", "1")
			w.Header().Add("X-Double", "2")
			if r.URL.Path == "/meta_200.tar.gz" {
				w.WriteHeader(http.StatusOK)
				w.Write(data)
			} else if r.URL.Path == "/restricted/meta_200.tar.gz" {
				if r.Header.Get("Authorization") == "Zm9vOmJhcg==" {
					w.WriteHeader(http.StatusOK)
					w.Write(data)
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			} else if r.URL.Path == "/meta_404.tar.gz" {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}),
	)

	return &TestArchiveHttpMock{
		server: Server,
	}
}
