// Copyright IBM Corp. 2017, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework/statestore"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func TestHTTPStateStore(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(storedState)
			if err != nil {
				return
			}
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config:               testAccStateStoreConfig(server.URL, "", "", ""),
			},
		},
	})
}

func TestHTTPStateStore_WithLocking(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var currentLock *statestore.LockInfo
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(storedState)
			if err != nil {
				return
			}

		case "POST":
			if currentLock != nil && r.URL.Query().Get("ID") != currentLock.ID {
				w.WriteHeader(http.StatusConflict)
				return
			}
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)

		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)

		case "LOCK":
			if currentLock != nil {
				w.WriteHeader(http.StatusLocked)
				err := json.NewEncoder(w).Encode(currentLock)
				if err != nil {
					return
				}
				return
			}

			var lockInfo statestore.LockInfo
			body, _ := io.ReadAll(r.Body)
			err := json.Unmarshal(body, &lockInfo)
			if err != nil {
				return
			}
			currentLock = &lockInfo
			w.WriteHeader(http.StatusOK)

		case "UNLOCK":
			lockID := lockIDFromRequest(r)
			if currentLock == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if currentLock.ID != lockID {
				w.WriteHeader(http.StatusConflict)
				return
			}
			currentLock = nil
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				VerifyStateStoreLock: true,
				Config:               testAccStateStoreConfig(server.URL, server.URL, "", ""),
			},
		},
	})
}

func TestHTTPStateStore_WithBasicAuth(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var mu sync.Mutex
	expectedUser := "testuser"
	expectedPass := "testpass"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		// Check basic auth
		user, pass, ok := r.BasicAuth()
		if !ok || user != expectedUser || pass != expectedPass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(storedState)
			if err != nil {
				return
			}
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config:               testAccStateStoreConfig(server.URL, "", expectedUser, expectedPass),
			},
		},
	})
}

func TestHTTPStateStore_WithEnvironmentConfiguration(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var currentLock *statestore.LockInfo
	var mu sync.Mutex

	expectedUser := "envuser"
	expectedPass := "envpass"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		user, pass, ok := r.BasicAuth()
		if !ok || user != expectedUser || pass != expectedPass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "PUT":
			if currentLock != nil && r.URL.Query().Get("ID") != currentLock.ID {
				w.WriteHeader(http.StatusConflict)
				return
			}
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		case "POST":
			if currentLock != nil {
				w.WriteHeader(http.StatusLocked)
				_ = json.NewEncoder(w).Encode(currentLock)
				return
			}

			var lockInfo statestore.LockInfo
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lockInfo)
			currentLock = &lockInfo
			w.WriteHeader(http.StatusOK)
		case "PATCH":
			lockID := lockIDFromRequest(r)
			if currentLock == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if currentLock.ID != lockID {
				w.WriteHeader(http.StatusConflict)
				return
			}
			currentLock = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	t.Setenv("TF_HTTP_ADDRESS", server.URL)
	t.Setenv("TF_HTTP_UPDATE_METHOD", "PUT")
	t.Setenv("TF_HTTP_LOCK_ADDRESS", server.URL)
	t.Setenv("TF_HTTP_UNLOCK_ADDRESS", server.URL)
	t.Setenv("TF_HTTP_LOCK_METHOD", "POST")
	t.Setenv("TF_HTTP_UNLOCK_METHOD", "PATCH")
	t.Setenv("TF_HTTP_USERNAME", expectedUser)
	t.Setenv("TF_HTTP_PASSWORD", expectedPass)
	t.Setenv("TF_HTTP_RETRY_MAX", "2")
	t.Setenv("TF_HTTP_RETRY_WAIT_MIN", "0")
	t.Setenv("TF_HTTP_RETRY_WAIT_MAX", "0")

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				VerifyStateStoreLock: true,
				Config: `
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
  }
}`,
			},
		},
	})
}

func TestHTTPStateStore_ConfigOverridesEnvironment(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")
	t.Setenv("TF_HTTP_UPDATE_METHOD", "PUT")

	var storedState []byte
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config:               testAccStateStoreConfigWithUpdateMethod(server.URL, "POST"),
			},
		},
	})
}

func TestHTTPStateStore_InvalidRetryEnvironment(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")
	t.Setenv("TF_HTTP_RETRY_MAX", "invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config:               testAccStateStoreConfig(server.URL, "", "", ""),
				ExpectError:          regexp.MustCompile(`invalid retry_max`),
			},
		},
	})
}

func TestHTTPStateStore_CustomUpdateMethod(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(storedState)
			if err != nil {
				return
			}
		case "PUT":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				// Configured to use PUT method instead of POST
				Config: testAccStateStoreConfigWithUpdateMethod(server.URL, "PUT"),
			},
		},
	})
}

func TestHTTPStateStore_WriteNoContent(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusNoContent)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config:               testAccStateStoreConfig(server.URL, "", "", ""),
			},
		},
	})
}

func TestHTTPStateStore_SkipCertVerification(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	certPath, keyPath := generateCert(t)
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("failed to load generated server cert/key: %v", err)
	}

	var storedState []byte
	var mu sync.Mutex

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config: fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    skip_cert_verification = true
  }
}`,
					server.URL,
				),
			},
		},
	})
}

func TestHTTPStateStore_NoSkipCertVerificationFails(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	certPath, keyPath := generateCert(t)
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("failed to load generated server cert/key: %v", err)
	}
	var storedState []byte
	var mu sync.Mutex

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config: fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
  }
}`,
					server.URL,
				),
				ExpectError: regexp.MustCompile(`(?i)x509:.*(unknown authority|certificate)`),
			},
		},
	})
}

func TestHTTPStateStore_ClientCertificateAuth(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	certPath, keyPath := generateCert(t)

	caData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read generated CA cert: %v", err)
	}

	clientCertData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read generated client cert: %v", err)
	}

	clientKeyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("failed to read generated client key: %v", err)
	}

	serverCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("failed to load generated server cert/key: %v", err)
	}

	clientCAPool := x509.NewCertPool()
	if ok := clientCAPool.AppendCertsFromPEM(caData); !ok {
		t.Fatal("failed to append CA cert to client CA pool")
	}

	var storedState []byte
	var mu sync.Mutex

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAPool,
	}
	server.StartTLS()
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config: fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    client_ca_certificate_pem = %q
    client_certificate_pem = %q
    client_private_key_pem = %q
  }
}`,
					server.URL,
					string(caData),
					string(clientCertData),
					string(clientKeyData),
				),
			},
		},
	})
}

func TestHTTPStateStore_NoClientCertificateFails(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	certPath, keyPath := generateCert(t)

	caData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read generated CA cert: %v", err)
	}

	serverCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("failed to load generated server cert/key: %v", err)
	}

	clientCAPool := x509.NewCertPool()
	if ok := clientCAPool.AppendCertsFromPEM(caData); !ok {
		t.Fatal("failed to append CA cert to client CA pool")
	}

	var requestCount int
	var mu sync.Mutex

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAPool,
	}
	server.StartTLS()
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config: fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    skip_cert_verification = true
  }
}`,
					server.URL,
				),
				ExpectError: regexp.MustCompile(`(?i)(tls: certificate required|certificate required|handshake failure)`),
			},
		},
	})

	if requestCount != 0 {
		t.Fatalf("expected TLS handshake to fail before any handler invocation, got %d requests", requestCount)
	}
}

func TestHTTPStateStore_NoLockSupport(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(storedState)
			if err != nil {
				return
			}
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				// Locking isn't configured, so this will cause a test assertion failure
				VerifyStateStoreLock: true,
				Config:               testAccStateStoreConfig(server.URL, "", "", ""),
				ExpectError:          regexp.MustCompile(`Failed client lock assertion`),
			},
		},
	})
}

func TestHTTPStateStore_InvalidUnlock(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var currentLock *statestore.LockInfo
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(storedState)
			if err != nil {
				return
			}

		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)

		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)

		case "LOCK":
			if currentLock != nil {
				w.WriteHeader(http.StatusLocked)
				err := json.NewEncoder(w).Encode(currentLock)
				if err != nil {
					return
				}
				return
			}

			var lockInfo statestore.LockInfo
			body, _ := io.ReadAll(r.Body)
			err := json.Unmarshal(body, &lockInfo)
			if err != nil {
				return
			}
			currentLock = &lockInfo
			w.WriteHeader(http.StatusOK)

		case "UNLOCK":
			lockID := lockIDFromRequest(r)
			if currentLock == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if currentLock.ID != lockID {
				w.WriteHeader(http.StatusConflict)
				return
			}
			// This simulates a broken unlock implementation, since it doesn't clear currentLock
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				VerifyStateStoreLock: true,
				Config:               testAccStateStoreConfig(server.URL, server.URL, "", ""),
				ExpectError:          regexp.MustCompile(`(?s)(Workspace is currently locked|Error creating test resource)`),
			},
		},
	})
}

func TestHTTPStateStore_CustomLockAndUnlockAddress(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var currentLock *statestore.LockInfo
	var mu sync.Mutex

	// State server
	stateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer stateServer.Close()

	// Lock server
	lockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.Method == "LOCK" {
			if currentLock != nil {
				w.WriteHeader(http.StatusLocked)
				_ = json.NewEncoder(w).Encode(currentLock)
				return
			}

			var lockInfo statestore.LockInfo
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lockInfo)
			currentLock = &lockInfo
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer lockServer.Close()

	// Unlock server
	unlockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.Method == "UNLOCK" {
			lockID := lockIDFromRequest(r)
			if currentLock == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if currentLock.ID != lockID {
				w.WriteHeader(http.StatusConflict)
				return
			}
			currentLock = nil
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer unlockServer.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				VerifyStateStoreLock: true,
				Config:               testAccStateStoreConfigWithLockAndUnlockAddress(stateServer.URL, lockServer.URL, unlockServer.URL),
			},
		},
	})
}

func TestHTTPStateStore_LockUnlockAddressWithExistingQueryParams(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var currentLock *statestore.LockInfo
	var mu sync.Mutex

	const token = "abc123"

	stateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer stateServer.Close()

	lockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.URL.Query().Get("token") != token {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.Method != "LOCK" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if currentLock != nil {
			w.WriteHeader(http.StatusLocked)
			_ = json.NewEncoder(w).Encode(currentLock)
			return
		}

		var lockInfo statestore.LockInfo
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &lockInfo)
		if lockInfo.ID == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.URL.Query().Get("ID") != "" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		currentLock = &lockInfo
		w.WriteHeader(http.StatusOK)
	}))
	defer lockServer.Close()

	unlockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.URL.Query().Get("token") != token {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.Method != "UNLOCK" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		lockID := lockIDFromRequest(r)
		if currentLock == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if currentLock.ID != lockID {
			w.WriteHeader(http.StatusConflict)
			return
		}
		currentLock = nil
		w.WriteHeader(http.StatusOK)
	}))
	defer unlockServer.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				VerifyStateStoreLock: true,
				Config: testAccStateStoreConfigWithLockAndUnlockAddress(
					stateServer.URL,
					lockServer.URL+"?token="+token,
					unlockServer.URL+"?token="+token,
				),
			},
		},
	})
}

func TestHTTPStateStore_CustomLockMethod(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var currentLock *statestore.LockInfo
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)

		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)

		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)

		case "PUT":
			// Custom lock method
			if currentLock != nil {
				w.WriteHeader(http.StatusLocked)
				_ = json.NewEncoder(w).Encode(currentLock)
				return
			}

			var lockInfo statestore.LockInfo
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lockInfo)
			currentLock = &lockInfo
			w.WriteHeader(http.StatusOK)

		case "UNLOCK":
			lockID := lockIDFromRequest(r)
			if currentLock == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if currentLock.ID != lockID {
				w.WriteHeader(http.StatusConflict)
				return
			}
			currentLock = nil
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				VerifyStateStoreLock: true,
				Config:               testAccStateStoreConfigWithCustomMethods(server.URL, server.URL, "PUT", "UNLOCK"),
			},
		},
	})
}

func TestHTTPStateStore_RetryConfiguration(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var mu sync.Mutex
	var requestCount int
	var postAttempts int
	var failedPostAttempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "POST":
			postAttempts++
			if postAttempts < 3 {
				failedPostAttempts++
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("temporary error"))
				return
			}
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config:               testAccStateStoreConfigWithRetry(server.URL, 2, 0, 0),
			},
		},
	})

	// Verify that retryable writes were attempted and eventually succeeded.
	if requestCount == 0 {
		t.Fatalf("expected at least 1 request, got %d", requestCount)
	}

	if failedPostAttempts != 2 {
		t.Fatalf("expected exactly 2 transient POST failures to trigger retries, got %d", failedPostAttempts)
	}

	if postAttempts < 3 {
		t.Fatalf("expected at least 3 POST attempts (initial + retries), got %d", postAttempts)
	}

	if storedState == nil {
		t.Fatal("expected state to be stored after retries succeeded")
	}
}

func TestHTTPStateStore_WebDAVPutCreatedThenNoContent(t *testing.T) {
	var storedState []byte
	var mu sync.Mutex
	var putAttempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "PUT":
			body, _ := io.ReadAll(r.Body)
			putAttempts++
			if string(storedState) == string(body) {
				storedState = body
				w.WriteHeader(http.StatusNoContent)
				return
			}

			storedState = body
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	store := &httpStateStore{
		client: &httpStateStoreClient{
			address:      server.URL,
			updateMethod: "PUT",
			client:       retryablehttp.NewClient(),
		},
	}

	ctx := context.Background()
	stateBytes := []byte(`{"version":4}`)

	firstResp := statestore.WriteResponse{}
	store.Write(ctx, statestore.WriteRequest{StateID: defaultWorkspaceName, StateBytes: stateBytes}, &firstResp)
	if firstResp.Diagnostics.HasError() {
		t.Fatalf("expected first PUT write to succeed with 201, got diagnostics: %v", firstResp.Diagnostics)
	}

	secondResp := statestore.WriteResponse{}
	store.Write(ctx, statestore.WriteRequest{StateID: defaultWorkspaceName, StateBytes: stateBytes}, &secondResp)
	if secondResp.Diagnostics.HasError() {
		t.Fatalf("expected second PUT write to succeed with 204, got diagnostics: %v", secondResp.Diagnostics)
	}

	if putAttempts != 2 {
		t.Fatalf("expected exactly 2 PUT attempts, got %d", putAttempts)
	}

	if string(storedState) != string(stateBytes) {
		t.Fatalf("expected stored state %q, got %q", string(stateBytes), string(storedState))
	}
}

func TestHTTPStateStore_SkipCertVerificationWithCACertificate(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	certPath, _ := generateCert(t)

	caData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read generated CA cert: %v", err)
	}

	var storedState []byte
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)
		case "POST":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)
		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	// Terraform Core backend allows skip_cert_verification alongside client_ca_certificate_pem.
	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config: fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    skip_cert_verification = true
    client_ca_certificate_pem = %q
  }
}`,
					server.URL,
					string(caData),
				),
			},
		},
	})
}

func TestHTTPStateStore_ClientCertificateWithoutKeyFails(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config: fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    client_certificate_pem = "dummy"
  }
}`,
					server.URL,
				),
				ExpectError: regexp.MustCompile(`client_certificate_pem is set but client_private_key_pem is not`),
			},
		},
	})
}

func TestHTTPStateStore_ClientKeyWithoutCertificateFails(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				Config: fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    client_private_key_pem = "dummy"
  }
}`,
					server.URL,
				),
				ExpectError: regexp.MustCompile(`client_private_key_pem is set but client_certificate_pem is not`),
			},
		},
	})
}

// Helper functions for test configs

func testAccStateStoreConfig(address, lockAddress, username, password string) string {
	additionalAttrs := ""

	if lockAddress != "" {
		additionalAttrs += fmt.Sprintf("\nlock_address = %q", lockAddress)
	}

	if username != "" && password != "" {
		additionalAttrs += fmt.Sprintf("\nusername = %q", username)
		additionalAttrs += fmt.Sprintf("\npassword = %q", password)
	}

	config := fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
	%s
  }
}`, address, additionalAttrs)

	return config
}

func testAccStateStoreConfigWithUpdateMethod(address, updateMethod string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    update_method = %q
  }
}`, address, updateMethod)
}

func testAccStateStoreConfigWithLockAndUnlockAddress(stateAddr, lockAddr, unlockAddr string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    lock_address = %q
    unlock_address = %q
  }
}`, stateAddr, lockAddr, unlockAddr)
}

func testAccStateStoreConfigWithCustomMethods(stateAddr, lockAddr, lockMethod, unlockMethod string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    lock_address = %q
    lock_method = %q
    unlock_method = %q
  }
}`, stateAddr, lockAddr, lockMethod, unlockMethod)
}

func testAccStateStoreConfigWithRetry(address string, maxRetry, waitMin, waitMax int64) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    retry_max = %d
    retry_wait_min = %d
    retry_wait_max = %d
  }
}`, address, maxRetry, waitMin, waitMax)
}

func TestHTTPStateStore_ComprehensiveConfiguration(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	var storedState []byte
	var currentLock *statestore.LockInfo
	var mu sync.Mutex

	expectedUser := "testuser"
	expectedPass := "testpass"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		// Check basic auth
		user, pass, ok := r.BasicAuth()
		if !ok || user != expectedUser || pass != expectedPass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(storedState)

		case "PUT":
			body, _ := io.ReadAll(r.Body)
			storedState = body
			w.WriteHeader(http.StatusOK)

		case "DELETE":
			storedState = nil
			w.WriteHeader(http.StatusOK)

		case "POST":
			// Custom lock method
			if currentLock != nil {
				w.WriteHeader(http.StatusLocked)
				_ = json.NewEncoder(w).Encode(currentLock)
				return
			}

			var lockInfo statestore.LockInfo
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lockInfo)
			currentLock = &lockInfo
			w.WriteHeader(http.StatusOK)

		case "PATCH":
			lockID := lockIDFromRequest(r)
			if currentLock == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if currentLock.ID != lockID {
				w.WriteHeader(http.StatusConflict)
				return
			}
			currentLock = nil
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_15_0),
			tfversion.SkipIfNotPrerelease(),
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				StateStore:           true,
				DefaultWorkspaceOnly: true,
				VerifyStateStoreLock: true,
				Config:               testAccStateStoreConfigComprehensive(server.URL, expectedUser, expectedPass),
			},
		},
	})
}

func testAccStateStoreConfigComprehensive(address, username, password string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    http = {
      source = "registry.terraform.io/hashicorp/http"
    }
  }
  state_store "http" {
    provider "http" {}
    address = %q
    update_method = "PUT"
    lock_address = %q
    lock_method = "POST"
    unlock_method = "PATCH"
	username = %q
	password = %q
    retry_max = 2
    retry_wait_min = 1
    retry_wait_max = 10
  }
}`, address, address, username, password)
}

func lockIDFromRequest(r *http.Request) string {
	if lockID := r.URL.Query().Get("ID"); lockID != "" {
		return lockID
	}

	body, _ := io.ReadAll(r.Body)
	if len(body) == 0 {
		return ""
	}

	var lockInfo statestore.LockInfo
	if err := json.Unmarshal(body, &lockInfo); err != nil {
		return ""
	}

	return lockInfo.ID
}
