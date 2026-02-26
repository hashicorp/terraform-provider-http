package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

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
			w.Write(storedState)
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

		lockID := r.URL.Query().Get("ID")

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write(storedState)

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
				json.NewEncoder(w).Encode(currentLock)
				return
			}

			var lockInfo statestore.LockInfo
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &lockInfo)
			currentLock = &lockInfo
			w.WriteHeader(http.StatusOK)

		case "UNLOCK":
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
			w.Write(storedState)
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
			w.Write(storedState)
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
			w.Write(storedState)
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

		lockID := r.URL.Query().Get("ID")

		switch r.Method {
		case "GET":
			if storedState == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write(storedState)

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
				json.NewEncoder(w).Encode(currentLock)
				return
			}

			var lockInfo statestore.LockInfo
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &lockInfo)
			currentLock = &lockInfo
			w.WriteHeader(http.StatusOK)

		case "UNLOCK":
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

// TODO: add more tests for other configurations
// - lock address + unlock address
// - lock method + unlock method
// - skip cert verification
// - retry max, wait min, wait max
// - client cert for TLS verification
// - client cert + private key for mTLS auth
// - validation tests? at least the mutually exclusive ones
// - maybe a test that uses almost all of the provider configuration, but with environment variables
//   - I need to update initialize to actually do this
