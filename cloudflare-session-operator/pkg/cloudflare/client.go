package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
)

const (
	baseURL        = "https://api.cloudflare.com/client/v4"
	maxRetries     = 3
	retryBaseDelay = 500 * time.Millisecond
)

// Client defines the minimal surface used by the operator to interact with Cloudflare.
type Client interface {
	EnsureSession(ctx context.Context, sessionID string) (bool, error)
	EnsureRoute(ctx context.Context, sessionID, endpoint string) error
	DeleteRoute(ctx context.Context, sessionID string) error
}

// APIClient is a lightweight implementation of Client built on top of the Cloudflare REST API.
type APIClient struct {
	HTTPClient   *http.Client
	BaseURL      string
	AccountID    string
	APIToken     string
	KVNamespaceID string
	Log          logr.Logger
}

// cfAPIResponse represents the standard Cloudflare API response envelope.
type cfAPIResponse struct {
	Success  bool            `json:"success"`
	Errors   []cfAPIError    `json:"errors"`
	Messages []string        `json:"messages"`
	Result   json.RawMessage `json:"result"`
}

// cfAPIError represents a single error returned by the Cloudflare API.
type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// accessKeyResponse represents the response from the Access keys endpoint.
type accessKeyResponse struct {
	KeyRotationIntervalDays int    `json:"key_rotation_interval_days"`
	LastKeyRotationAt       string `json:"last_key_rotation_at"`
}

// NewClientFromEnv creates a Client using environment variables for configuration.
// Expected environment variables:
//   - CLOUDFLARE_ACCOUNT_ID
//   - CLOUDFLARE_API_TOKEN
//   - CLOUDFLARE_KV_NAMESPACE_ID
func NewClientFromEnv() *APIClient {
	return &APIClient{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		BaseURL:    baseURL,
		AccountID:  os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
		APIToken:   os.Getenv("CLOUDFLARE_API_TOKEN"),
		KVNamespaceID: os.Getenv("CLOUDFLARE_KV_NAMESPACE_ID"),
		Log:        logr.Discard(),
	}
}

// SetLogger configures a structured logger for API operations.
func (c *APIClient) SetLogger(l logr.Logger) {
	c.Log = l.WithName("cloudflare-client")
}

// HasCredentials returns true if the required Cloudflare credentials are configured.
func (c *APIClient) HasCredentials() bool {
	return c.AccountID != "" && c.APIToken != ""
}

// EnsureSession validates that a Cloudflare Access session exists and is active by
// calling the Access keys metadata endpoint. A successful response indicates the
// account's Access configuration is operational and sessions can be served.
func (c *APIClient) EnsureSession(ctx context.Context, sessionID string) (bool, error) {
	if sessionID == "" {
		return false, fmt.Errorf("sessionID is empty")
	}

	log := c.Log.WithValues("sessionID", sessionID)
	log.V(1).Info("validating Cloudflare Access session")

	url := fmt.Sprintf("%s/accounts/%s/access/keys", c.BaseURL, c.AccountID)
	resp, err := c.doWithRetry(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("cloudflare session validation request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read session validation response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		log.Info("session not found on Cloudflare")
		return false, nil
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, fmt.Errorf("cloudflare authentication failed (HTTP %d): check API token permissions", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("cloudflare API returned unexpected status %d: %s", resp.StatusCode, truncateBody(body, 256))
	}

	var apiResp cfAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return false, fmt.Errorf("failed to parse Cloudflare API response: %w", err)
	}

	if !apiResp.Success {
		errMsg := "unknown error"
		if len(apiResp.Errors) > 0 {
			errMsg = apiResp.Errors[0].Message
		}
		return false, fmt.Errorf("cloudflare API error: %s", errMsg)
	}

	log.V(1).Info("session validated successfully")
	return true, nil
}

// EnsureRoute programs a session-to-endpoint mapping in Cloudflare Workers KV.
// The key is the sessionID and the value is a JSON payload containing the backend
// endpoint address that Cloudflare Workers can use to route traffic.
func (c *APIClient) EnsureRoute(ctx context.Context, sessionID, endpoint string) error {
	if sessionID == "" {
		return fmt.Errorf("sessionID is empty")
	}
	if endpoint == "" {
		return fmt.Errorf("endpoint is empty")
	}
	if c.KVNamespaceID == "" {
		return fmt.Errorf("KV namespace ID is not configured (set CLOUDFLARE_KV_NAMESPACE_ID)")
	}

	log := c.Log.WithValues("sessionID", sessionID, "endpoint", endpoint, "kvNamespace", c.KVNamespaceID)
	log.V(1).Info("programming route in Workers KV")

	routeValue := map[string]string{
		"endpoint":  endpoint,
		"sessionID": sessionID,
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	}
	payload, err := json.Marshal(routeValue)
	if err != nil {
		return fmt.Errorf("failed to marshal route value: %w", err)
	}

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s",
		c.BaseURL, c.AccountID, c.KVNamespaceID, sessionID)

	resp, err := c.doWithRetry(ctx, http.MethodPut, url, payload)
	if err != nil {
		return fmt.Errorf("cloudflare KV write request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read KV write response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("cloudflare authentication failed (HTTP %d): check API token permissions for Workers KV", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cloudflare KV write failed (HTTP %d): %s", resp.StatusCode, truncateBody(body, 256))
	}

	var apiResp cfAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to parse KV write response: %w", err)
	}

	if !apiResp.Success {
		errMsg := "unknown error"
		if len(apiResp.Errors) > 0 {
			errMsg = apiResp.Errors[0].Message
		}
		return fmt.Errorf("cloudflare KV write error: %s", errMsg)
	}

	log.Info("route programmed successfully in Workers KV")
	return nil
}

// DeleteRoute removes a session-to-endpoint mapping from Cloudflare Workers KV.
func (c *APIClient) DeleteRoute(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if c.KVNamespaceID == "" {
		return fmt.Errorf("KV namespace ID is not configured (set CLOUDFLARE_KV_NAMESPACE_ID)")
	}

	log := c.Log.WithValues("sessionID", sessionID, "kvNamespace", c.KVNamespaceID)
	log.V(1).Info("deleting route from Workers KV")

	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s",
		c.BaseURL, c.AccountID, c.KVNamespaceID, sessionID)

	resp, err := c.doWithRetry(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("cloudflare KV delete request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read KV delete response: %w", err)
	}

	// 404 is acceptable during deletion -- the key may already be gone.
	if resp.StatusCode == http.StatusNotFound {
		log.V(1).Info("route key not found in KV (already deleted)")
		return nil
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("cloudflare authentication failed (HTTP %d): check API token permissions for Workers KV", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cloudflare KV delete failed (HTTP %d): %s", resp.StatusCode, truncateBody(body, 256))
	}

	var apiResp cfAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to parse KV delete response: %w", err)
	}

	if !apiResp.Success {
		errMsg := "unknown error"
		if len(apiResp.Errors) > 0 {
			errMsg = apiResp.Errors[0].Message
		}
		return fmt.Errorf("cloudflare KV delete error: %s", errMsg)
	}

	log.Info("route deleted from Workers KV")
	return nil
}

// doWithRetry performs an HTTP request with exponential backoff retry for transient errors.
func (c *APIClient) doWithRetry(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			c.Log.V(1).Info("retrying request", "attempt", attempt, "delay", delay.String())
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.APIToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "cloudflare-session-operator/1.0")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			c.Log.V(1).Info("request failed, will retry", "error", err.Error(), "attempt", attempt)
			continue
		}

		// Retry on 429 (rate limited) and 5xx (server errors).
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d from Cloudflare API", resp.StatusCode)
			resp.Body.Close()
			c.Log.V(1).Info("received retryable status", "statusCode", resp.StatusCode, "attempt", attempt)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("all %d retries exhausted: %w", maxRetries, lastErr)
}

// truncateBody returns the first n bytes of body as a string for error messages.
func truncateBody(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + "..."
}
