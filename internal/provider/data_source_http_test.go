package provider

import (
	"fmt"
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

	resource.UnitTest(t, resource.TestCase{
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
