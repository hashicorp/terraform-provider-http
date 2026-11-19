// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestEphemeral_200(t *testing.T) {
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
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
							ephemeral "http" "http_test" {
								url = "%s"
							}
							provider "echo" {
								data = ephemeral.http.http_test
							}
							resource "echo" "out" {}`, testServer.URL),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.out",
						tfjsonpath.New("data").AtMapKey("response_body"),
						knownvalue.StringExact("1.0.0"),
					),
					statecheck.ExpectKnownValue("echo.out",
						tfjsonpath.New("data").AtMapKey("status_code"),
						knownvalue.Int64Exact(200),
					),
					statecheck.ExpectKnownValue("echo.out",
						tfjsonpath.New("data").AtMapKey("response_headers").AtMapKey("Content-Type"),
						knownvalue.StringExact("text/plain"),
					),
					statecheck.ExpectKnownValue("echo.out",
						tfjsonpath.New("data").AtMapKey("response_headers").AtMapKey("X-Single"),
						knownvalue.StringExact("foobar"),
					),
					statecheck.ExpectKnownValue("echo.out",
						tfjsonpath.New("data").AtMapKey("response_headers").AtMapKey("X-Double"),
						knownvalue.StringExact("1, 2"),
					),
				},
			},
		},
	})
}
