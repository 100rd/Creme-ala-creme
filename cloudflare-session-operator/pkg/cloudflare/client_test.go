package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// newTestClient creates an APIClient pointing at the given test server.
func newTestClient(serverURL string) *APIClient {
	return &APIClient{
		HTTPClient:    &http.Client{Timeout: 5 * time.Second},
		BaseURL:       serverURL,
		AccountID:     "test-account-id",
		APIToken:      "test-api-token",
		KVNamespaceID: "test-kv-namespace",
		Log:           logr.Discard(),
	}
}

// successEnvelope returns a standard Cloudflare success envelope.
func successEnvelope(result interface{}) []byte {
	r, _ := json.Marshal(result)
	resp := cfAPIResponse{
		Success: true,
		Errors:  []cfAPIError{},
		Result:  r,
	}
	b, _ := json.Marshal(resp)
	return b
}

// errorEnvelope returns a standard Cloudflare error envelope.
func errorEnvelope(code int, message string) []byte {
	resp := cfAPIResponse{
		Success: false,
		Errors:  []cfAPIError{{Code: code, Message: message}},
	}
	b, _ := json.Marshal(resp)
	return b
}

// ---------- EnsureSession tests ----------

func TestEnsureSession_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/accounts/test-account-id/access/keys") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-token" {
			t.Errorf("missing or incorrect Authorization header")
		}
		w.WriteHeader(http.StatusOK)
		w.Write(successEnvelope(map[string]interface{}{
			"key_rotation_interval_days": 30,
		}))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	exists, err := client.EnsureSession(context.Background(), "session-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("expected session to exist")
	}
}

func TestEnsureSession_EmptySessionID(t *testing.T) {
	client := newTestClient("http://localhost")
	_, err := client.EnsureSession(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty sessionID")
	}
	if !strings.Contains(err.Error(), "sessionID is empty") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestEnsureSession_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	exists, err := client.EnsureSession(context.Background(), "session-missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected session to not exist")
	}
}

func TestEnsureSession_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.EnsureSession(context.Background(), "session-123")
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestEnsureSession_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(errorEnvelope(1000, "invalid token"))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.EnsureSession(context.Background(), "session-123")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
	if !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestEnsureSession_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	client.HTTPClient.Timeout = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.EnsureSession(ctx, "session-123")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ---------- EnsureRoute tests ----------

func TestEnsureRoute_Success(t *testing.T) {
	var capturedBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		expectedPath := "/accounts/test-account-id/storage/kv/namespaces/test-kv-namespace/values/session-456"
		if !strings.Contains(r.URL.Path, expectedPath) {
			t.Errorf("unexpected path: %s, expected to contain: %s", r.URL.Path, expectedPath)
		}
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&capturedBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(successEnvelope(nil))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.EnsureRoute(context.Background(), "session-456", "10.0.0.5:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedBody["endpoint"] != "10.0.0.5:8080" {
		t.Errorf("expected endpoint 10.0.0.5:8080, got %s", capturedBody["endpoint"])
	}
	if capturedBody["sessionID"] != "session-456" {
		t.Errorf("expected sessionID session-456, got %s", capturedBody["sessionID"])
	}
}

func TestEnsureRoute_EmptySessionID(t *testing.T) {
	client := newTestClient("http://localhost")
	err := client.EnsureRoute(context.Background(), "", "10.0.0.1:80")
	if err == nil {
		t.Fatal("expected error for empty sessionID")
	}
}

func TestEnsureRoute_EmptyEndpoint(t *testing.T) {
	client := newTestClient("http://localhost")
	err := client.EnsureRoute(context.Background(), "session-1", "")
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
}

func TestEnsureRoute_MissingKVNamespace(t *testing.T) {
	client := newTestClient("http://localhost")
	client.KVNamespaceID = ""
	err := client.EnsureRoute(context.Background(), "session-1", "10.0.0.1:80")
	if err == nil {
		t.Fatal("expected error for missing KV namespace")
	}
	if !strings.Contains(err.Error(), "KV namespace ID") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestEnsureRoute_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.EnsureRoute(context.Background(), "session-1", "10.0.0.1:80")
	if err == nil {
		t.Fatal("expected error for forbidden")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// ---------- DeleteRoute tests ----------

func TestDeleteRoute_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(successEnvelope(nil))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.DeleteRoute(context.Background(), "session-789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteRoute_EmptySessionID(t *testing.T) {
	client := newTestClient("http://localhost")
	err := client.DeleteRoute(context.Background(), "")
	if err != nil {
		t.Fatalf("expected no error for empty sessionID, got: %v", err)
	}
}

func TestDeleteRoute_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.DeleteRoute(context.Background(), "session-gone")
	if err != nil {
		t.Fatalf("expected no error for 404 delete, got: %v", err)
	}
}

func TestDeleteRoute_MissingKVNamespace(t *testing.T) {
	client := newTestClient("http://localhost")
	client.KVNamespaceID = ""
	err := client.DeleteRoute(context.Background(), "session-1")
	if err == nil {
		t.Fatal("expected error for missing KV namespace")
	}
}

// ---------- Retry behavior tests ----------

func TestDoWithRetry_RetriesOn500(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(successEnvelope(nil))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	resp, err := client.doWithRetry(context.Background(), http.MethodGet, server.URL+"/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDoWithRetry_RetriesOn429(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(successEnvelope(nil))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	resp, err := client.doWithRetry(context.Background(), http.MethodGet, server.URL+"/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDoWithRetry_DoesNotRetryOn400(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	resp, err := client.doWithRetry(context.Background(), http.MethodGet, server.URL+"/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if attempts != 1 {
		t.Errorf("expected 1 attempt for 400, got %d", attempts)
	}
}

// ---------- HasCredentials tests ----------

func TestHasCredentials(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		apiToken  string
		want      bool
	}{
		{"both set", "acct", "token", true},
		{"missing account", "", "token", false},
		{"missing token", "acct", "", false},
		{"both empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &APIClient{AccountID: tt.accountID, APIToken: tt.apiToken}
			if got := c.HasCredentials(); got != tt.want {
				t.Errorf("HasCredentials() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------- truncateBody tests ----------

func TestTruncateBody(t *testing.T) {
	short := "hello"
	if got := truncateBody([]byte(short), 10); got != short {
		t.Errorf("expected %q, got %q", short, got)
	}

	long := strings.Repeat("a", 300)
	result := truncateBody([]byte(long), 256)
	if len(result) != 259 { // 256 + "..."
		t.Errorf("expected truncated length 259, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("expected truncated body to end with ...")
	}
}
