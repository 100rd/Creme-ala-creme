package cloudflare

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	// cloudflareAPIBase is the base URL for the Cloudflare API.
	cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

	// sessionIDPattern validates session IDs to prevent injection.
	sessionIDPattern = `^[a-zA-Z0-9_-]{1,128}$`

	// httpTimeout is the default timeout for HTTP requests.
	httpTimeout = 10 * time.Second
)

var sessionIDRegex = regexp.MustCompile(sessionIDPattern)

// Client defines the minimal surface used by the operator to interact with Cloudflare.
type Client interface {
	EnsureSession(ctx context.Context, sessionID string) (bool, error)
	EnsureRoute(ctx context.Context, sessionID, endpoint string) error
	DeleteRoute(ctx context.Context, sessionID string) error
}

// APIClient is a lightweight implementation of Client built on top of the Cloudflare REST API.
type APIClient struct {
	HTTPClient   *http.Client
	AccountID    string
	APIToken     string
	KVNamespace  string
	DryRun       bool
}

// NewClientFromEnv creates a Client using environment variables for configuration.
// Expected environment variables:
//   - CLOUDFLARE_ACCOUNT_ID
//   - CLOUDFLARE_API_TOKEN
//   - CLOUDFLARE_KV_NAMESPACE_ID
//   - CLOUDFLARE_DRY_RUN (optional, "true" to enable dry-run mode)
func NewClientFromEnv() Client {
	return &APIClient{
		HTTPClient:  &http.Client{Timeout: httpTimeout},
		AccountID:   os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
		APIToken:    os.Getenv("CLOUDFLARE_API_TOKEN"),
		KVNamespace: os.Getenv("CLOUDFLARE_KV_NAMESPACE_ID"),
		DryRun:      strings.EqualFold(os.Getenv("CLOUDFLARE_DRY_RUN"), "true"),
	}
}

// ValidateSessionID checks that a session ID matches the expected pattern.
func ValidateSessionID(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("sessionID is empty")
	}
	if !sessionIDRegex.MatchString(sessionID) {
		return fmt.Errorf("sessionID %q does not match pattern %s", sessionID, sessionIDPattern)
	}
	return nil
}

// EnsureSession verifies a Cloudflare session exists via the Access API.
// Returns (true, nil) if the session is active, (false, nil) if not found,
// and (false, error) on transient failures.
func (c *APIClient) EnsureSession(ctx context.Context, sessionID string) (bool, error) {
	if err := ValidateSessionID(sessionID); err != nil {
		return false, fmt.Errorf("invalid session ID: %w", err)
	}
	if c.DryRun {
		return true, nil
	}

	url := fmt.Sprintf("%s/accounts/%s/access/sessions/%s", cloudflareAPIBase, c.AccountID, sessionID)
	return c.doSessionCheck(ctx, url)
}

func (c *APIClient) doSessionCheck(ctx context.Context, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("creating session check request: %w", err)
	}
	c.setAuthHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("executing session check request: %w", err)
	}
	defer drainAndClose(resp.Body)

	switch {
	case resp.StatusCode == http.StatusOK:
		return true, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	case resp.StatusCode >= 500:
		return false, fmt.Errorf("cloudflare server error: status %d", resp.StatusCode)
	default:
		return false, fmt.Errorf("unexpected status from cloudflare: %d", resp.StatusCode)
	}
}

// EnsureRoute writes a session-to-endpoint mapping in Cloudflare Workers KV.
func (c *APIClient) EnsureRoute(ctx context.Context, sessionID, endpoint string) error {
	if err := ValidateSessionID(sessionID); err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	if endpoint == "" {
		return fmt.Errorf("endpoint is empty")
	}
	if c.DryRun {
		return nil
	}

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s",
		cloudflareAPIBase, c.AccountID, c.KVNamespace, sessionID)
	return c.doKVWrite(ctx, url, endpoint)
}

func (c *APIClient) doKVWrite(ctx context.Context, url, value string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(value))
	if err != nil {
		return fmt.Errorf("creating KV write request: %w", err)
	}
	c.setAuthHeaders(req)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing KV write request: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare KV write failed: status %d", resp.StatusCode)
	}
	return nil
}

// DeleteRoute removes a session-to-endpoint mapping from Cloudflare Workers KV.
func (c *APIClient) DeleteRoute(ctx context.Context, sessionID string) error {
	if err := ValidateSessionID(sessionID); err != nil {
		return fmt.Errorf("invalid session ID for route deletion: %w", err)
	}
	if c.DryRun {
		return nil
	}

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s",
		cloudflareAPIBase, c.AccountID, c.KVNamespace, sessionID)
	return c.doKVDelete(ctx, url)
}

func (c *APIClient) doKVDelete(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("creating KV delete request: %w", err)
	}
	c.setAuthHeaders(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing KV delete request: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return nil // already deleted
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare KV delete failed: status %d", resp.StatusCode)
	}
	return nil
}

func (c *APIClient) setAuthHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.APIToken)
}

// drainAndClose reads remaining bytes and closes the body to allow connection reuse.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}
