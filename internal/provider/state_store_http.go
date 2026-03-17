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
				Description: "The address of the HTTP endpoint for state storage",
				Required:    true,
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

// configModel represents the configuration for the HTTP state store
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

// httpStateStoreClient represents the configured HTTP client for state operations
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
}

func (s *httpStateStore) Initialize(ctx context.Context, req statestore.InitializeRequest, resp *statestore.InitializeResponse) {
	var config configModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate mutual TLS configuration
	hasClientCert := !config.ClientCertPEM.IsNull() && !config.ClientCertPEM.IsUnknown()
	hasClientKey := !config.ClientPrivateKeyPEM.IsNull() && !config.ClientPrivateKeyPEM.IsUnknown()

	if hasClientCert != hasClientKey {
		resp.Diagnostics.AddAttributeError(
			path.Root("client_certificate_pem"),
			"Invalid mTLS Configuration",
			"Both client_certificate_pem and client_private_key_pem must be set together for mTLS authentication",
		)
		return
	}

	// Validate skip_cert_verification conflicts
	if !config.SkipCertVerification.IsNull() && config.SkipCertVerification.ValueBool() &&
		!config.ClientCACertPEM.IsNull() && !config.ClientCACertPEM.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("skip_cert_verification"),
			"Conflicting Configuration",
			"skip_cert_verification cannot be true when client_ca_certificate_pem is set",
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
	if !config.ClientCACertPEM.IsNull() && !config.ClientCACertPEM.IsUnknown() {
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM([]byte(config.ClientCACertPEM.ValueString())); !ok {
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
			[]byte(config.ClientCertPEM.ValueString()),
			[]byte(config.ClientPrivateKeyPEM.ValueString()),
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
	retryMax := int64(2) // default
	if !config.RetryMax.IsNull() && !config.RetryMax.IsUnknown() {
		retryMax = config.RetryMax.ValueInt64()
	}
	retryClient.RetryMax = int(retryMax)

	retryWaitMin := int64(1) // default (seconds)
	if !config.RetryWaitMin.IsNull() && !config.RetryWaitMin.IsUnknown() {
		retryWaitMin = config.RetryWaitMin.ValueInt64()
	}
	retryClient.RetryWaitMin = time.Duration(retryWaitMin) * time.Second

	retryWaitMax := int64(30) // default (seconds)
	if !config.RetryWaitMax.IsNull() && !config.RetryWaitMax.IsUnknown() {
		retryWaitMax = config.RetryWaitMax.ValueInt64()
	}
	retryClient.RetryWaitMax = time.Duration(retryWaitMax) * time.Second

	// Set defaults for optional fields
	updateMethod := "POST"
	if !config.UpdateMethod.IsNull() && !config.UpdateMethod.IsUnknown() {
		updateMethod = config.UpdateMethod.ValueString()
	}

	lockMethod := "LOCK"
	if !config.LockMethod.IsNull() && !config.LockMethod.IsUnknown() {
		lockMethod = config.LockMethod.ValueString()
	}

	unlockMethod := "UNLOCK"
	if !config.UnlockMethod.IsNull() && !config.UnlockMethod.IsUnknown() {
		unlockMethod = config.UnlockMethod.ValueString()
	}

	unlockAddress := config.LockAddress.ValueString()
	if !config.UnlockAddress.IsNull() && !config.UnlockAddress.IsUnknown() {
		unlockAddress = config.UnlockAddress.ValueString()
	}

	// Create and store client
	client := &httpStateStoreClient{
		address:       config.Address.ValueString(),
		updateMethod:  updateMethod,
		lockAddress:   config.LockAddress.ValueString(),
		unlockAddress: unlockAddress,
		lockMethod:    lockMethod,
		unlockMethod:  unlockMethod,
		username:      config.Username.ValueString(),
		password:      config.Password.ValueString(),
		client:        retryClient,
	}

	resp.StateStoreData = client
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
	// TODO: HTTP backend today doesn't support listing states from the server, so I think we'll likely always have to indicate the default workspace exists
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

	httpReq, err := retryablehttp.NewRequestWithContext(ctx, s.client.updateMethod, s.client.address, bytes.NewReader(req.StateBytes))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating HTTP request",
			fmt.Sprintf("Error creating %s request to %s: %s", s.client.updateMethod, s.client.address, err),
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

	// 200 OK or 201 Created are success
	if httpResp.StatusCode == http.StatusOK || httpResp.StatusCode == http.StatusCreated {
		tflog.Debug(ctx, "State written successfully", map[string]interface{}{"status": httpResp.StatusCode})
		return
	}

	// Any other status code is an error
	body, _ := io.ReadAll(httpResp.Body)
	resp.Diagnostics.AddError(
		"Unexpected HTTP status code",
		fmt.Sprintf("Expected status 200 or 201, got %d from %s: %s", httpResp.StatusCode, s.client.address, string(body)),
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

	// Build URL with lock ID as query parameter
	lockURL := fmt.Sprintf("%s?ID=%s", s.client.lockAddress, url.QueryEscape(lockInfo.ID))

	httpReq, err := retryablehttp.NewRequestWithContext(ctx, s.client.lockMethod, lockURL, bytes.NewReader(lockData))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating HTTP request",
			fmt.Sprintf("Error creating %s request to %s: %s", s.client.lockMethod, lockURL, err),
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
			fmt.Sprintf("Error making %s request to %s: %s", s.client.lockMethod, lockURL, err),
		)
		return
	}
	defer httpResp.Body.Close()

	// 200 OK means lock acquired
	if httpResp.StatusCode == http.StatusOK {
		tflog.Debug(ctx, "State locked successfully", map[string]interface{}{"lockID": lockInfo.ID})
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
		fmt.Sprintf("Expected status 200, 423, or 409, got %d from %s: %s", httpResp.StatusCode, lockURL, string(body)),
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

	// Build URL with lock ID as query parameter
	unlockURL := fmt.Sprintf("%s?ID=%s", s.client.unlockAddress, url.QueryEscape(req.LockID))

	httpReq, err := retryablehttp.NewRequestWithContext(ctx, s.client.unlockMethod, unlockURL, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating HTTP request",
			fmt.Sprintf("Error creating %s request to %s: %s", s.client.unlockMethod, unlockURL, err),
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
			"Error unlocking state",
			fmt.Sprintf("Error making %s request to %s: %s", s.client.unlockMethod, unlockURL, err),
		)
		return
	}
	defer httpResp.Body.Close()

	// 200 OK means unlock successful
	if httpResp.StatusCode == http.StatusOK {
		tflog.Debug(ctx, "State unlocked successfully")
		return
	}

	// Any other status code is an error
	body, _ := io.ReadAll(httpResp.Body)
	resp.Diagnostics.AddError(
		"Unexpected HTTP status code",
		fmt.Sprintf("Expected status 200, got %d from %s: %s", httpResp.StatusCode, unlockURL, string(body)),
	)
}

var multipleWorkspacesNotSupportedDiag = diag.NewErrorDiagnostic(
	"Multiple workspaces not supported",
	"The http state store does not support multiple workspaces, use the \"default\" workspace",
)
