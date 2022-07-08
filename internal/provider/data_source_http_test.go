package provider

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/elazarl/goproxy"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestDataSource_200(t *testing.T) {
	testHttpMock := setUpMockHttpServer()
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
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
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
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
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
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
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
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
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
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
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
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

func TestDataSource_x509cert(t *testing.T) {
	testHttpMock := setUpMockHttpServer()
	defer testHttpMock.server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							data "http" "http_test" {
  								url = "%s/x509-ca-cert/200"
							}`, testHttpMock.server.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "response_body", "pem"),
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
				),
			},
		},
	})
}

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
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Config: fmt.Sprintf(`
							data "http" "http_test" {
								url = "%s/200"
							}`, testHttpMock.server.URL),
				PlanOnly: true,
			},
			{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
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

func TestDataSource_HTTPViaProxy(t *testing.T) {
	t.Parallel()

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	proxy := httptest.NewServer(goproxy.NewProxyHttpServer())
	defer proxy.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),

		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					provider "http" {
						proxy = {
							url = "%s"
						}
					}
					data "http" "http_test" {
						url = "%s"
					}
				`, proxy.URL, svr.URL),
				Check: resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
			},
		},
	})
}

func TestDataSource_HTTPViaProxyWithBasicAuthConfig(t *testing.T) {
	t.Parallel()

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	p := goproxy.NewProxyHttpServer()
	p.OnRequest().DoFunc(proxyAuth())

	proxy := httptest.NewServer(p)
	defer proxy.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),

		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					provider "http" {
						proxy = {
							url = "%s"
							username = "correctUsername"
						}
					}
					data "http" "http_test" {
						url = "%s"
					}
				`, proxy.URL, svr.URL),
				Check: resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
			},
			{
				Config: fmt.Sprintf(`
					provider "http" {
						proxy = {
							url = "%s"
							username = "incorrectUsername"
						}
					}
					data "http" "http_test" {
						url = "%s"
					}
				`, proxy.URL, svr.URL),
				Check: resource.TestCheckResourceAttr("data.http.http_test", "status_code", "407"),
			},
			{
				Config: fmt.Sprintf(`
					provider "http" {
						proxy = {
							url = "%s"
							username = "correctUsername"
							password = "correctPassword"
						}
					}
					data "http" "http_test" {
						url = "%s"
					}
				`, proxy.URL, svr.URL),
				Check: resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
			},
			{
				Config: fmt.Sprintf(`
					provider "http" {
						proxy = {
							url = "%s"
							username = "correctUsername"
							password = "incorrectPassword"
						}
					}
					data "http" "http_test" {
						url = "%s"
					}
				`, proxy.URL, svr.URL),
				Check: resource.TestCheckResourceAttr("data.http.http_test", "status_code", "407"),
			},
		},
	})
}

func TestDataSource_HTTPViaProxyWithEnv(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	p := goproxy.NewProxyHttpServer()
	p.OnRequest().DoFunc(proxyAuth())

	proxy := httptest.NewServer(p)
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),

		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					provider "http" {
						proxy = {
							from_env = true
						}
					}
					data "http" "http_test" {
						url = "%s"
					}
				`, svr.URL),
				Check: resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
			},
		},
	})
}

func TestDataSource_HTTPNoProxyAvailable(t *testing.T) {
	t.Parallel()

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer svr.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),

		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					provider "http" {
						proxy = {
							url = "http://not-a-real-proxy.com"
						}
					}
					data "http" "http_test" {
						url = "%s"
					}
				`, svr.URL),
				ExpectError: regexp.MustCompile(
					fmt.Sprintf(`Error making request: Get "%s": proxyconnect tcp: dial\ntcp: lookup not-a-real-proxy.com: no such host`,
						svr.URL,
					),
				),
			},
		},
	})
}

func proxyAuth() func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		auth := r.Header.Get("Proxy-Authorization")
		if auth == "" {
			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusForbidden, "")
		}

		c, err := base64.StdEncoding.DecodeString(auth[len("Basic "):])
		if err != nil {
			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusProxyAuthRequired, "")
		}

		cs := string(c)
		username, password, ok := strings.Cut(cs, ":")
		if !ok {
			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusProxyAuthRequired, "")
		}

		if username == "correctUsername" && password == "" || password == "correctPassword" {
			return r, nil
		}

		return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusProxyAuthRequired, "")

	}
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
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}),
	)

	return &TestHttpMock{
		server: Server,
	}
}
