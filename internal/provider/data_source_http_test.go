package provider

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"

	"github.com/terraform-providers/terraform-provider-http/internal/provider/testutils"
)

func TestDataSource_HTTPViaProxyWithEnv(t *testing.T) {
	server, err := testutils.NewHTTPServer()
	if err != nil {
		t.Fatal(err)
	}

	defer server.Close()
	go server.ServeTLS()

	proxy, err := testutils.NewHTTPProxyServer()
	if err != nil {
		t.Fatal(err)
	}

	defer proxy.Close()
	go proxy.Serve()

	t.Setenv("HTTP_PROXY", fmt.Sprintf("http://%s", proxy.Address()))
	t.Setenv("HTTPS_PROXY", fmt.Sprintf("http://%s", proxy.Address()))

	fmt.Printf("proxy address = %s\n", proxy.Address())
	fmt.Printf("server address = %s\n", server.Address())

	resource.Test(t, resource.TestCase{
		ProtoV5ProviderFactories: protoV5ProviderFactories(),

		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "http" "http_test" {
						url = "https://%s"
						insecure = "true"
					}
				`, server.Address()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
					testutils.TestCheckBothServerAndProxyWereUsed(server, proxy),
				),
			},
		},
	})
}

//func TestDataSource_HTTPViaProxyWithEnv_Proxy_SHRP(t *testing.T) {
//	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		w.WriteHeader(http.StatusOK)
//	}))
//	defer svr.Close()
//
//	u, err := url.Parse(svr.URL)
//	if err != nil {
//		panic(err)
//	}
//
//	proxy := httputil.NewSingleHostReverseProxy(u)
//
//	t.Setenv("HTTP_PROXY", fmt.Sprintf("http://%s", proxy.URL))
//	t.Setenv("HTTPS_PROXY", fmt.Sprintf("http://%s", proxy.URL))
//
//	fmt.Printf("proxy address = %s\n", proxy.URL)
//	fmt.Printf("server address = %s\n", svr.URL)
//
//	resource.Test(t, resource.TestCase{
//		ProtoV5ProviderFactories: protoV5ProviderFactories(),
//
//		Steps: []resource.TestStep{
//			{
//				Config: fmt.Sprintf(`
//					data "http" "http_test" {
//						url = "%s"
//						insecure = "true"
//					}
//				`, svr.URL),
//				Check: resource.ComposeAggregateTestCheckFunc(
//					resource.TestCheckResourceAttr("data.http.http_test", "status_code", "200"),
//					//testutils.TestCheckBothServerAndProxyWereUsed(server, proxy),
//				),
//			},
//		},
//	})
//}

func TestDataSource_HTTPViaProxyWithEnv_Proxy(t *testing.T) {
	proxy, err := NewProxy("http://my-api-server.com")
	if err != nil {
		panic(err)
	}

	// handle all requests to your server using the proxy
	http.HandleFunc("/", ProxyRequestHandler(proxy))

	go func() {
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()
}

func NewProxy(targetHost string) (*httputil.ReverseProxy, error) {
	url, err := url.Parse(targetHost)
	if err != nil {
		return nil, err
	}

	return httputil.NewSingleHostReverseProxy(url), nil
}

// ProxyRequestHandler handles the http request using proxy
func ProxyRequestHandler(proxy *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	}
}
