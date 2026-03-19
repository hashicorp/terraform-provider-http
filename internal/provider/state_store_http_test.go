// Copyright IBM Corp. 2017, 2026
// SPDX-License-Identifier: MPL-2.0

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
				Config:               testAccStateStoreConfigWithRetry(server.URL, 3, 1, 5),
			},
		},
	})

	// Verify that requests were made
	if requestCount == 0 {
		t.Fatalf("expected at least 1 request, got %d", requestCount)
	}
}

func TestHTTPStateStore_ValidationMutuallyExclusive(t *testing.T) {
	t.Setenv("TF_ENABLE_PLUGGABLE_STATE_STORAGE", "1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test skip_cert_verification with client_ca_certificate_pem (mutually exclusive)
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
				Config:               testAccStateStoreConfigWithMutuallyExclusive(server.URL),
				ExpectError:          regexp.MustCompile(`skip_cert_verification cannot be true when client_ca_certificate_pem is set`),
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

func testAccStateStoreConfigWithMutuallyExclusive(address string) string {
	// Note: Using a dummy cert for testing validation
	dummyCert := "-----BEGIN CERTIFICATE-----\nAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n-----END CERTIFICATE-----"
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
    skip_cert_verification = true
    client_ca_certificate_pem = %q
  }
}`, address, dummyCert)
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
