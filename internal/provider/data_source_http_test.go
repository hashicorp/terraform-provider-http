package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestDataSource_HTTPViaProxyWithEnv___(t *testing.T) {
	proxyRequests := 0
	serverRequests := 0
	pReqPtr := &proxyRequests
	sReqPtr := &serverRequests

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*sReqPtr++
		w.WriteHeader(http.StatusOK)
	}))

	defer backend.Close()

	backendURLStr := strings.Replace(backend.URL, "127.0.0.1", "backend", -1)
	backendURL, err := url.Parse(backendURLStr)
	if err != nil {
		t.Fatal(err)
	}

	proxy := func(u *url.URL) http.Handler {
		pBackendURLStr := strings.Replace(u.String(), "backend", "127.0.0.1", -1)
		pBackendURL, err := url.Parse(pBackendURLStr)
		if err != nil {
			t.Fatal(err)
		}

		p := httputil.NewSingleHostReverseProxy(pBackendURL)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*pReqPtr++
			p.ServeHTTP(w, r)
		})
	}(backendURL)

	frontend := httptest.NewServer(proxy)
	defer frontend.Close()

	t.Setenv("HTTP_PROXY", frontend.URL)
	t.Setenv("HTTPS_PROXY", frontend.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),

		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "http" "http_test" {
						url = "%s"
						insecure = "true"
					}
				`, backendURLStr),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
					CheckServerAndProxyRequestCount(pReqPtr, sReqPtr),
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
