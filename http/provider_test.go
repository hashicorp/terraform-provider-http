package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
)

type TestHttpMock struct {
	server *httptest.Server
}

var testProviders = map[string]terraform.ResourceProvider{
	"http": Provider(),
}

func TestProvider(t *testing.T) {
	if err := Provider().(*schema.Provider).InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func setUpMockHttpServer() *TestHttpMock {
	Server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			if r.Method == http.MethodPost {
				bodyMap := make(map[string]string)
				if err := json.NewDecoder(r.Body).Decode(&bodyMap); err != nil {
					w.WriteHeader(400)
				}
				if bodyMap["hello"] != "world" {
					w.WriteHeader(400)
				}
			}
			if r.Method == http.MethodPut {
				bodyMap := make(map[string]string)
				if err := json.NewDecoder(r.Body).Decode(&bodyMap); err != nil {
					w.WriteHeader(400)
				}
				if bodyMap["hello"] != "update" {
					w.WriteHeader(400)
				}
			}
			if r.Method == http.MethodDelete {
				w.WriteHeader(204)
				return
			}
			if r.URL.Path == "/meta_200.txt" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("1.0.0"))
			} else if r.URL.Path == "/restricted/meta_200.txt" {
				if r.Header.Get("Authorization") == "Zm9vOmJhcg==" {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("1.0.0"))
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			} else if r.URL.Path == "/utf-8/meta_200.txt" {
				w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("1.0.0"))
			} else if r.URL.Path == "/utf-16/meta_200.txt" {
				w.Header().Set("Content-Type", "application/json; charset=UTF-16")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("\"1.0.0\""))
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
