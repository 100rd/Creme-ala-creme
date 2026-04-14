package cloudflare

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateSessionID(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
	}{
		{"valid alphanumeric", "abc123", false},
		{"valid with hyphens", "session-abc-123", false},
		{"valid with underscores", "session_abc_123", false},
		{"valid max length", string(make([]byte, 128)), true}, // 128 null bytes won't match pattern
		{"empty", "", true},
		{"too long", "a" + string(make([]byte, 128)), true},
		{"invalid chars spaces", "session abc", true},
		{"invalid chars special", "session@abc", true},
		{"invalid chars slash", "session/abc", true},
		{"valid 128 chars", repeatChar('a', 128), false},
		{"invalid 129 chars", repeatChar('a', 129), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSessionID(tt.sessionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSessionID(%q) error = %v, wantErr %v", tt.sessionID, err, tt.wantErr)
			}
		})
	}
}

func repeatChar(c byte, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

func TestEnsureSession(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		sessionID  string
		dryRun     bool
		wantExists bool
		wantErr    bool
	}{
		{
			name:       "session exists (200)",
			statusCode: http.StatusOK,
			sessionID:  "valid-session-1",
			wantExists: true,
			wantErr:    false,
		},
		{
			name:       "session not found (404)",
			statusCode: http.StatusNotFound,
			sessionID:  "missing-session",
			wantExists: false,
			wantErr:    false,
		},
		{
			name:       "server error (500)",
			statusCode: http.StatusInternalServerError,
			sessionID:  "error-session",
			wantExists: false,
			wantErr:    true,
		},
		{
			name:       "unauthorized (401)",
			statusCode: http.StatusUnauthorized,
			sessionID:  "unauth-session",
			wantExists: false,
			wantErr:    true,
		},
		{
			name:      "empty session ID",
			sessionID: "",
			wantErr:   true,
		},
		{
			name:      "invalid session ID",
			sessionID: "invalid/session",
			wantErr:   true,
		},
		{
			name:       "dry run mode returns true",
			sessionID:  "dry-run-session",
			dryRun:     true,
			wantExists: true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify auth header is set
				if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
					t.Errorf("expected auth header 'Bearer test-token', got %q", auth)
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			client := &APIClient{
				HTTPClient:  srv.Client(),
				AccountID:   "test-account",
				APIToken:    "test-token",
				KVNamespace: "test-ns",
				DryRun:      tt.dryRun,
			}

			// Override the base URL by using a custom transport
			if !tt.dryRun && tt.sessionID != "" && ValidateSessionID(tt.sessionID) == nil {
				// Use the test server URL directly
				origClient := client.HTTPClient
				client.HTTPClient = &http.Client{
					Transport: &rewriteTransport{
						base:    origClient.Transport,
						baseURL: srv.URL,
					},
				}
			}

			exists, err := client.EnsureSession(context.Background(), tt.sessionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if exists != tt.wantExists {
				t.Errorf("EnsureSession() = %v, want %v", exists, tt.wantExists)
			}
		})
	}
}

func TestEnsureRoute(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		sessionID  string
		endpoint   string
		dryRun     bool
		wantErr    bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			sessionID:  "valid-session",
			endpoint:   "10.0.0.1:8080",
			wantErr:    false,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			sessionID:  "valid-session",
			endpoint:   "10.0.0.1:8080",
			wantErr:    true,
		},
		{
			name:      "empty session ID",
			sessionID: "",
			endpoint:  "10.0.0.1:8080",
			wantErr:   true,
		},
		{
			name:      "empty endpoint",
			sessionID: "valid-session",
			endpoint:  "",
			wantErr:   true,
		},
		{
			name:      "dry run skips API",
			sessionID: "valid-session",
			endpoint:  "10.0.0.1:8080",
			dryRun:    true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut {
					t.Errorf("expected PUT, got %s", r.Method)
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			client := &APIClient{
				HTTPClient:  srv.Client(),
				AccountID:   "test-account",
				APIToken:    "test-token",
				KVNamespace: "test-ns",
				DryRun:      tt.dryRun,
			}

			if !tt.dryRun && tt.sessionID != "" && tt.endpoint != "" && ValidateSessionID(tt.sessionID) == nil {
				origClient := client.HTTPClient
				client.HTTPClient = &http.Client{
					Transport: &rewriteTransport{
						base:    origClient.Transport,
						baseURL: srv.URL,
					},
				}
			}

			err := client.EnsureRoute(context.Background(), tt.sessionID, tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureRoute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteRoute(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		sessionID  string
		dryRun     bool
		wantErr    bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			sessionID:  "valid-session",
			wantErr:    false,
		},
		{
			name:       "already deleted (404)",
			statusCode: http.StatusNotFound,
			sessionID:  "missing-session",
			wantErr:    false,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			sessionID:  "valid-session",
			wantErr:    true,
		},
		{
			name:      "empty session ID",
			sessionID: "",
			wantErr:   true,
		},
		{
			name:      "dry run skips API",
			sessionID: "valid-session",
			dryRun:    true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Errorf("expected DELETE, got %s", r.Method)
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			client := &APIClient{
				HTTPClient:  srv.Client(),
				AccountID:   "test-account",
				APIToken:    "test-token",
				KVNamespace: "test-ns",
				DryRun:      tt.dryRun,
			}

			if !tt.dryRun && tt.sessionID != "" && ValidateSessionID(tt.sessionID) == nil {
				origClient := client.HTTPClient
				client.HTTPClient = &http.Client{
					Transport: &rewriteTransport{
						base:    origClient.Transport,
						baseURL: srv.URL,
					},
				}
			}

			err := client.DeleteRoute(context.Background(), tt.sessionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteRoute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// rewriteTransport rewrites request URLs to point to the test server.
type rewriteTransport struct {
	base    http.RoundTripper
	baseURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.baseURL[len("http://"):]
	if t.base != nil {
		return t.base.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
