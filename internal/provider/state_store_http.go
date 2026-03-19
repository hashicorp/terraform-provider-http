// Copyright IBM Corp. 2017, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/statestore"
	"github.com/hashicorp/terraform-plugin-framework/statestore/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/net/http/httpproxy"
)

var (
	_ statestore.StateStore              = (*httpStateStore)(nil)
	_ statestore.StateStoreWithConfigure = (*httpStateStore)(nil)
)

const defaultWorkspaceName = "default"

const (
	envHTTPAddress             = "TF_HTTP_ADDRESS"
	envHTTPUpdateMethod        = "TF_HTTP_UPDATE_METHOD"
	envHTTPLockAddress         = "TF_HTTP_LOCK_ADDRESS"
	envHTTPUnlockAddress       = "TF_HTTP_UNLOCK_ADDRESS"
	envHTTPLockMethod          = "TF_HTTP_LOCK_METHOD"
	envHTTPUnlockMethod        = "TF_HTTP_UNLOCK_METHOD"
	envHTTPUsername            = "TF_HTTP_USERNAME"
	envHTTPPassword            = "TF_HTTP_PASSWORD"
	envHTTPRetryMax            = "TF_HTTP_RETRY_MAX"
	envHTTPRetryWaitMin        = "TF_HTTP_RETRY_WAIT_MIN"
	envHTTPRetryWaitMax        = "TF_HTTP_RETRY_WAIT_MAX"
	envHTTPClientCACertificate = "TF_HTTP_CLIENT_CA_CERTIFICATE_PEM"
	envHTTPClientCertificate   = "TF_HTTP_CLIENT_CERTIFICATE_PEM"
	envHTTPClientPrivateKeyPEM = "TF_HTTP_CLIENT_PRIVATE_KEY_PEM"
)

type httpLockInfo struct {
	ID        string
	Operation string
	Who       string
	Created   time.Time
	Version   string
	Path      string
	Info      string
}

type httpStateStore struct {
	client           *httpStateStoreClient
	terraformVersion string
}

func NewHttpStateStore(terraformVersion string) statestore.StateStore {
	return &httpStateStore{
		terraformVersion: terraformVersion,
	}
}

func (s *httpStateStore) Metadata(ctx context.Context, req statestore.MetadataRequest, resp *statestore.MetadataResponse) {
	resp.TypeName = "http"
}

func (s *httpStateStore) Schema(ctx context.Context, req statestore.SchemaRequest, resp *statestore.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "HTTP state store for managing Terraform state via HTTP endpoints",
		Attributes: map[string]schema.Attribute{
			"address": schema.StringAttribute{
				Description: "The address of the HTTP endpoint for state storage. May also be set via TF_HTTP_ADDRESS.",
				Optional:    true,
			},
			"update_method": schema.StringAttribute{
				Description: "HTTP method to use when updating state. Defaults to POST",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("POST", "PUT"),
				},
			},
			"lock_address": schema.StringAttribute{
				Description: "The address of the HTTP endpoint for state locking. Optional.",
				Optional:    true,
			},
			"lock_method": schema.StringAttribute{
				Description: "HTTP method to use when locking state. Defaults to LOCK",
				Optional:    true,
			},
			"unlock_address": schema.StringAttribute{
				Description: "The address of the HTTP endpoint for unlocking state. Defaults to lock_address if not set.",
				Optional:    true,
			},
			"unlock_method": schema.StringAttribute{
				Description: "HTTP method to use when unlocking state. Defaults to UNLOCK",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "Username for HTTP basic authentication",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password for HTTP basic authentication",
				Optional:    true,
			},
			"skip_cert_verification": schema.BoolAttribute{
				Description: "Whether to skip TLS certificate verification",
				Optional:    true,
			},
			"retry_max": schema.Int64Attribute{
				Description: "Maximum number of retries. Defaults to 2",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"retry_wait_min": schema.Int64Attribute{
				Description: "Minimum time in seconds to wait between retries. Defaults to 1",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"retry_wait_max": schema.Int64Attribute{
				Description: "Maximum time in seconds to wait between retries. Defaults to 30",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"client_ca_certificate_pem": schema.StringAttribute{
				Description: "PEM-encoded CA certificate for TLS verification",
				Optional:    true,
			},
			"client_certificate_pem": schema.StringAttribute{
				Description: "PEM-encoded client certificate for mTLS authentication",
				Optional:    true,
			},
			"client_private_key_pem": schema.StringAttribute{
				Description: "PEM-encoded client private key for mTLS authentication",
				Optional:    true,
			},
		},
	}
}

// configModel represents the configuration for the HTTP state store.
type configModel struct {
	Address              types.String `tfsdk:"address"`
	UpdateMethod         types.String `tfsdk:"update_method"`
	LockAddress          types.String `tfsdk:"lock_address"`
	LockMethod           types.String `tfsdk:"lock_method"`
	UnlockAddress        types.String `tfsdk:"unlock_address"`
	UnlockMethod         types.String `tfsdk:"unlock_method"`
	Username             types.String `tfsdk:"username"`
	Password             types.String `tfsdk:"password"`
	SkipCertVerification types.Bool   `tfsdk:"skip_cert_verification"`
	RetryMax             types.Int64  `tfsdk:"retry_max"`
	RetryWaitMin         types.Int64  `tfsdk:"retry_wait_min"`
	RetryWaitMax         types.Int64  `tfsdk:"retry_wait_max"`
	ClientCACertPEM      types.String `tfsdk:"client_ca_certificate_pem"`
	ClientCertPEM        types.String `tfsdk:"client_certificate_pem"`
	ClientPrivateKeyPEM  types.String `tfsdk:"client_private_key_pem"`
}

// httpStateStoreClient represents the configured HTTP client for state operations.
type httpStateStoreClient struct {
	address       string
	updateMethod  string
	lockAddress   string
	lockMethod    string
	unlockAddress string
	unlockMethod  string
	username      string
	password      string
	client        *retryablehttp.Client
	lockID        string
	lockData      []byte
}

func (s *httpStateStore) Initialize(ctx context.Context, req statestore.InitializeRequest, resp *statestore.InitializeResponse) {
	var config configModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	address := stringValueOrEnv(config.Address, envHTTPAddress, "")
	if address == "" {
		resp.Diagnostics.AddError(
			"Missing required configuration",
			fmt.Sprintf("address argument is required or must be set via %s", envHTTPAddress),
		)
		return
	}

	if err := validateHTTPURL(address, "address"); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}

	updateMethod := stringValueOrEnv(config.UpdateMethod, envHTTPUpdateMethod, "POST")
	lockAddress := stringValueOrEnv(config.LockAddress, envHTTPLockAddress, "")
	if lockAddress != "" {
		if err := validateHTTPURL(lockAddress, "lock_address"); err != nil {
			resp.Diagnostics.AddError("Invalid configuration", err.Error())
			return
		}
	}

	lockMethod := stringValueOrEnv(config.LockMethod, envHTTPLockMethod, "LOCK")
	unlockAddress := stringValueOrEnv(config.UnlockAddress, envHTTPUnlockAddress, lockAddress)
	if unlockAddress != "" {
		if err := validateHTTPURL(unlockAddress, "unlock_address"); err != nil {
			resp.Diagnostics.AddError("Invalid configuration", err.Error())
			return
		}
	}

	unlockMethod := stringValueOrEnv(config.UnlockMethod, envHTTPUnlockMethod, "UNLOCK")
	username := stringValueOrEnv(config.Username, envHTTPUsername, "")
	password := stringValueOrEnv(config.Password, envHTTPPassword, "")
	clientCACertPEM := stringValueOrEnv(config.ClientCACertPEM, envHTTPClientCACertificate, "")
	clientCertPEM := stringValueOrEnv(config.ClientCertPEM, envHTTPClientCertificate, "")
	clientPrivateKeyPEM := stringValueOrEnv(config.ClientPrivateKeyPEM, envHTTPClientPrivateKeyPEM, "")

	retryMax, err := int64ValueOrEnv(config.RetryMax, envHTTPRetryMax, 2)
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", fmt.Sprintf("invalid retry_max: %s", err))
		return
	}

	retryWaitMin, err := int64ValueOrEnv(config.RetryWaitMin, envHTTPRetryWaitMin, 1)
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", fmt.Sprintf("invalid retry_wait_min: %s", err))
		return
	}

	retryWaitMax, err := int64ValueOrEnv(config.RetryWaitMax, envHTTPRetryWaitMax, 30)
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", fmt.Sprintf("invalid retry_wait_max: %s", err))
		return
	}

	// Validate mutual TLS configuration
	hasClientCert := clientCertPEM != ""
	hasClientKey := clientPrivateKeyPEM != ""

	if hasClientCert && !hasClientKey {
		resp.Diagnostics.AddError(
			"Invalid mTLS Configuration",
			"client_certificate_pem is set but client_private_key_pem is not",
		)
		return
	}

	if hasClientKey && !hasClientCert {
		resp.Diagnostics.AddError(
			"Invalid mTLS Configuration",
			"client_private_key_pem is set but client_certificate_pem is not",
		)
		return
	}

	// Build HTTP transport (duplicated from data_source_http.go)
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		resp.Diagnostics.AddError(
			"Error configuring http transport",
			"Error http: Can't configure http transport.",
		)
		return
	}

	// Clone transport to avoid shared state
	clonedTr := tr.Clone()

	// Configure proxy from environment
	clonedTr.Proxy = func(req *http.Request) (*url.URL, error) {
		return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
	}

	if clonedTr.TLSClientConfig == nil {
		clonedTr.TLSClientConfig = &tls.Config{}
	}

	// Configure TLS skip verification
	if !config.SkipCertVerification.IsNull() {
		clonedTr.TLSClientConfig.InsecureSkipVerify = config.SkipCertVerification.ValueBool()
	}

	// Configure CA certificate
	if clientCACertPEM != "" {
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM([]byte(clientCACertPEM)); !ok {
			resp.Diagnostics.AddAttributeError(
				path.Root("client_ca_certificate_pem"),
				"Error configuring TLS client",
				"Error tls: Can't add the CA certificate to certificate pool. Only PEM encoded certificates are supported.",
			)
			return
		}
		clonedTr.TLSClientConfig.RootCAs = caCertPool
	}

	// Configure client certificate for mTLS
	if hasClientCert && hasClientKey {
		cert, err := tls.X509KeyPair(
			[]byte(clientCertPEM),
			[]byte(clientPrivateKeyPEM),
		)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error creating x509 key pair",
				fmt.Sprintf("Error creating x509 key pair from provided pem blocks\n\nError: %s", err),
			)
			return
		}
		clonedTr.TLSClientConfig.Certificates = []tls.Certificate{cert}
	}

	// Create retryable HTTP client
	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient.Transport = clonedTr
	retryClient.Logger = levelledLogger{ctx}

	// Configure retry parameters
	retryClient.RetryMax = int(retryMax)

	retryClient.RetryWaitMin = time.Duration(retryWaitMin) * time.Second

	retryClient.RetryWaitMax = time.Duration(retryWaitMax) * time.Second

	// Create and store client
	client := &httpStateStoreClient{
		address:       address,
		updateMethod:  updateMethod,
		lockAddress:   lockAddress,
		unlockAddress: unlockAddress,
		lockMethod:    lockMethod,
		unlockMethod:  unlockMethod,
		username:      username,
		password:      password,
		client:        retryClient,
	}

	resp.StateStoreData = client
}

func stringValueOrEnv(value types.String, envName, defaultValue string) string {
	if !value.IsNull() && !value.IsUnknown() {
		return value.ValueString()
	}

	if envValue, ok := os.LookupEnv(envName); ok {
		return envValue
	}

	return defaultValue
}

func int64ValueOrEnv(value types.Int64, envName string, defaultValue int64) (int64, error) {
	if !value.IsNull() && !value.IsUnknown() {
		return value.ValueInt64(), nil
	}

	if envValue, ok := os.LookupEnv(envName); ok {
		parsed, err := strconv.ParseInt(envValue, 10, 64)
		if err != nil {
			return 0, err
		}

		return parsed, nil
	}

	return defaultValue, nil
}

func validateHTTPURL(rawURL, fieldName string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("failed to parse %s URL: %s", fieldName, err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("%s must be HTTP or HTTPS", fieldName)
	}

	return nil
}

func (s *httpStateStore) Configure(ctx context.Context, req statestore.ConfigureRequest, resp *statestore.ConfigureResponse) {
	if req.StateStoreData == nil {
		return
	}

	client, ok := req.StateStoreData.(*httpStateStoreClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected StateStore Configure Type",
			fmt.Sprintf("Expected *httpStateStoreClient, got: %T. Please report this issue to the provider developers.", req.StateStoreData),
		)
		return
	}

	s.client = client
}

func (s *httpStateStore) GetStates(ctx context.Context, req statestore.GetStatesRequest, resp *statestore.GetStatesResponse) {
	// Terraform Core's HTTP backend does not support workspace enumeration and only operates on the default workspace.
	resp.StateIDs = []string{defaultWorkspaceName}
}

func (s *httpStateStore) Read(ctx context.Context, req statestore.ReadRequest, resp *statestore.ReadResponse) {
	if req.StateID != defaultWorkspaceName {
		resp.Diagnostics.Append(multipleWorkspacesNotSupportedDiag)
		return
	}

	tflog.Debug(ctx, "Reading state", map[string]interface{}{"address": s.client.address})

	httpReq, err := retryablehttp.NewRequestWithContext(ctx, "GET", s.client.address, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating HTTP request",
			fmt.Sprintf("Error creating GET request to %s: %s", s.client.address, err),
		)
		return
	}

	// Set basic auth if configured
	if s.client.username != "" && s.client.password != "" {
		httpReq.SetBasicAuth(s.client.username, s.client.password)
	}

	httpResp, err := s.client.client.Do(httpReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading state",
			fmt.Sprintf("Error making GET request to %s: %s", s.client.address, err),
		)
		return
	}
	defer httpResp.Body.Close()

	// 404 means no state exists yet - this is OK
	if httpResp.StatusCode == http.StatusNotFound {
		tflog.Debug(ctx, "No state exists yet (404)")
		resp.StateBytes = nil
		return
	}

	// 200 means state exists
	if httpResp.StatusCode == http.StatusOK {
		stateData, err := io.ReadAll(httpResp.Body)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error reading response body",
				fmt.Sprintf("Error reading response body from %s: %s", s.client.address, err),
			)
			return
		}
		resp.StateBytes = stateData
		return
	}

	// 204 means no content exists yet - this is OK
	if httpResp.StatusCode == http.StatusNoContent {
		tflog.Debug(ctx, "No content exists (204)")
		resp.StateBytes = nil
		return
	}

	// Any other status code is an error
	resp.Diagnostics.AddError(
		"Unexpected HTTP status code",
		fmt.Sprintf("Expected status 200 or 404, got %d from %s", httpResp.StatusCode, s.client.address),
	)
}

func (s *httpStateStore) Write(ctx context.Context, req statestore.WriteRequest, resp *statestore.WriteResponse) {
	if req.StateID != defaultWorkspaceName {
		resp.Diagnostics.Append(multipleWorkspacesNotSupportedDiag)
		return
	}

	tflog.Debug(ctx, "Writing state", map[string]interface{}{
		"address": s.client.address,
		"method":  s.client.updateMethod,
		"stateID": req.StateID,
	})

	writeURL := s.client.address
	var err error
	if s.client.lockID != "" {
		writeURL, err = withQueryParam(s.client.address, "ID", s.client.lockID)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error creating write URL",
				fmt.Sprintf("Error adding lock ID query parameter to %s: %s", s.client.address, err),
			)
			return
		}
	}

	httpReq, err := retryablehttp.NewRequestWithContext(ctx, s.client.updateMethod, writeURL, bytes.NewReader(req.StateBytes))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating HTTP request",
			fmt.Sprintf("Error creating %s request to %s: %s", s.client.updateMethod, writeURL, err),
		)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Set basic auth if configured
	if s.client.username != "" && s.client.password != "" {
		httpReq.SetBasicAuth(s.client.username, s.client.password)
	}

	httpResp, err := s.client.client.Do(httpReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error writing state",
			fmt.Sprintf("Error making %s request to %s: %s", s.client.updateMethod, s.client.address, err),
		)
		return
	}
	defer httpResp.Body.Close()

	// 200 OK, 201 Created, or 204 No Content are success.
	if httpResp.StatusCode == http.StatusOK || httpResp.StatusCode == http.StatusCreated || httpResp.StatusCode == http.StatusNoContent {
		tflog.Debug(ctx, "State written successfully", map[string]interface{}{"status": httpResp.StatusCode})
		return
	}

	// Any other status code is an error
	body, _ := io.ReadAll(httpResp.Body)
	resp.Diagnostics.AddError(
		"Unexpected HTTP status code",
		fmt.Sprintf("Expected status 200, 201, or 204, got %d from %s: %s", httpResp.StatusCode, writeURL, string(body)),
	)
}

func (s *httpStateStore) DeleteState(ctx context.Context, req statestore.DeleteStateRequest, resp *statestore.DeleteStateResponse) {
	if req.StateID != defaultWorkspaceName {
		resp.Diagnostics.Append(multipleWorkspacesNotSupportedDiag)
		return
	}

	tflog.Debug(ctx, "Deleting state", map[string]interface{}{"address": s.client.address})

	httpReq, err := retryablehttp.NewRequestWithContext(ctx, "DELETE", s.client.address, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating HTTP request",
			fmt.Sprintf("Error creating DELETE request to %s: %s", s.client.address, err),
		)
		return
	}

	// Set basic auth if configured
	if s.client.username != "" && s.client.password != "" {
		httpReq.SetBasicAuth(s.client.username, s.client.password)
	}

	httpResp, err := s.client.client.Do(httpReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting state",
			fmt.Sprintf("Error making DELETE request to %s: %s", s.client.address, err),
		)
		return
	}
	defer httpResp.Body.Close()

	// 200 OK, 204 No Content, or 404 Not Found are all success
	// (404 means it was already deleted)
	if httpResp.StatusCode == http.StatusOK ||
		httpResp.StatusCode == http.StatusNoContent ||
		httpResp.StatusCode == http.StatusNotFound {
		tflog.Debug(ctx, "State deleted successfully", map[string]interface{}{"status": httpResp.StatusCode})

		return
	}

	// Any other status code is an error
	body, _ := io.ReadAll(httpResp.Body)
	resp.Diagnostics.AddError(
		"Unexpected HTTP status code",
		fmt.Sprintf("Expected status 200, 204, or 404, got %d from %s: %s", httpResp.StatusCode, s.client.address, string(body)),
	)
}

func (s *httpStateStore) Lock(ctx context.Context, req statestore.LockRequest, resp *statestore.LockResponse) {
	if req.StateID != defaultWorkspaceName {
		resp.Diagnostics.Append(multipleWorkspacesNotSupportedDiag)
		return
	}

	// If locking is not configured, return empty LockID to indicate no locking support
	if s.client.lockAddress == "" {
		tflog.Debug(ctx, "Locking not configured, skipping lock")
		resp.LockID = ""
		return
	}

	tflog.Debug(ctx, "Locking state", map[string]interface{}{
		"address":   s.client.lockAddress,
		"method":    s.client.lockMethod,
		"stateID":   req.StateID,
		"operation": req.Operation,
	})

	// Create lock info
	// lockInfo := statestore.NewLockInfo(req)
	newLockInfo := statestore.NewLockInfo(req)
	lockInfo := httpLockInfo{
		ID:        newLockInfo.ID,
		Operation: newLockInfo.Operation,
		Who:       newLockInfo.Who,
		Created:   newLockInfo.Created,
		Version:   s.terraformVersion,
		Path:      "", // leave blank as required but not available
		Info:      "", // leave blank as required but not available
	}

	// Marshal lock info to JSON
	lockData, err := json.Marshal(lockInfo)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error marshaling lock info",
			fmt.Sprintf("Error marshaling lock info to JSON: %s", err),
		)
		return
	}

	httpReq, err := retryablehttp.NewRequestWithContext(ctx, s.client.lockMethod, s.client.lockAddress, bytes.NewReader(lockData))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating HTTP request",
			fmt.Sprintf("Error creating %s request to %s: %s", s.client.lockMethod, s.client.lockAddress, err),
		)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Set basic auth if configured
	if s.client.username != "" && s.client.password != "" {
		httpReq.SetBasicAuth(s.client.username, s.client.password)
	}

	httpResp, err := s.client.client.Do(httpReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error locking state",
			fmt.Sprintf("Error making %s request to %s: %s", s.client.lockMethod, s.client.lockAddress, err),
		)
		return
	}
	defer httpResp.Body.Close()

	// 200 OK means lock acquired
	if httpResp.StatusCode == http.StatusOK {
		tflog.Debug(ctx, "State locked successfully", map[string]interface{}{"lockID": lockInfo.ID})
		s.client.lockID = lockInfo.ID
		s.client.lockData = lockData
		resp.LockID = lockInfo.ID
		return
	}

	// 423 Locked or 409 Conflict means already locked
	if httpResp.StatusCode == http.StatusLocked || httpResp.StatusCode == http.StatusConflict {
		body, _ := io.ReadAll(httpResp.Body)

		// Try to unmarshal existing lock info
		var existingLock statestore.LockInfo
		if err := json.Unmarshal(body, &existingLock); err == nil {
			// Use helper to create formatted diagnostic
			resp.Diagnostics.Append(statestore.WorkspaceAlreadyLockedDiagnostic(req, existingLock))
		} else {
			// Fallback to basic error
			resp.Diagnostics.AddError(
				"State Already Locked",
				fmt.Sprintf("State is already locked: %s", string(body)),
			)
		}
		return
	}

	// Any other status code is an error
	body, _ := io.ReadAll(httpResp.Body)
	resp.Diagnostics.AddError(
		"Unexpected HTTP status code",
		fmt.Sprintf("Expected status 200, 423, or 409, got %d from %s: %s", httpResp.StatusCode, s.client.lockAddress, string(body)),
	)
}

func (s *httpStateStore) Unlock(ctx context.Context, req statestore.UnlockRequest, resp *statestore.UnlockResponse) {
	if req.StateID != defaultWorkspaceName {
		resp.Diagnostics.Append(multipleWorkspacesNotSupportedDiag)
		return
	}

	// If locking is not configured, this shouldn't be called, but handle gracefully
	if s.client.lockAddress == "" {
		tflog.Debug(ctx, "Locking not configured, skipping unlock")
		return
	}

	tflog.Debug(ctx, "Unlocking state", map[string]interface{}{
		"address": s.client.unlockAddress,
		"method":  s.client.unlockMethod,
		"stateID": req.StateID,
		"lockID":  req.LockID,
	})

	unlockData := s.client.lockData
	if req.LockID != "" && (s.client.lockID == "" || s.client.lockID != req.LockID || len(unlockData) == 0) {
		fallbackUnlockInfo := httpLockInfo{
			ID: req.LockID,
		}
		marshaledUnlockData, err := json.Marshal(fallbackUnlockInfo)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error marshaling unlock info",
				fmt.Sprintf("Error marshaling unlock info to JSON: %s", err),
			)
			return
		}
		unlockData = marshaledUnlockData
	}

	httpReq, err := retryablehttp.NewRequestWithContext(ctx, s.client.unlockMethod, s.client.unlockAddress, bytes.NewReader(unlockData))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating HTTP request",
			fmt.Sprintf("Error creating %s request to %s: %s", s.client.unlockMethod, s.client.unlockAddress, err),
		)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Set basic auth if configured
	if s.client.username != "" && s.client.password != "" {
		httpReq.SetBasicAuth(s.client.username, s.client.password)
	}

	httpResp, err := s.client.client.Do(httpReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error unlocking state",
			fmt.Sprintf("Error making %s request to %s: %s", s.client.unlockMethod, s.client.unlockAddress, err),
		)
		return
	}
	defer httpResp.Body.Close()

	// 200 OK means unlock successful
	if httpResp.StatusCode == http.StatusOK {
		tflog.Debug(ctx, "State unlocked successfully")
		s.client.lockID = ""
		s.client.lockData = nil
		return
	}

	// Any other status code is an error
	body, _ := io.ReadAll(httpResp.Body)
	resp.Diagnostics.AddError(
		"Unexpected HTTP status code",
		fmt.Sprintf("Expected status 200, got %d from %s: %s", httpResp.StatusCode, s.client.unlockAddress, string(body)),
	)
}

var multipleWorkspacesNotSupportedDiag = diag.NewErrorDiagnostic(
	"Multiple workspaces not supported",
	"The http state store does not support multiple workspaces, use the \"default\" workspace",
)

func withQueryParam(baseAddress, key, value string) (string, error) {
	u, err := url.Parse(baseAddress)
	if err != nil {
		return "", err
	}

	query := u.Query()
	query.Set(key, value)
	u.RawQuery = query.Encode()

	return u.String(), nil
}
