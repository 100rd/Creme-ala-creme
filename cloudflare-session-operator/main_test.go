package main

import (
	"os"
	"testing"
)

func TestValidateCredentials(t *testing.T) {
	tests := []struct {
		name      string
		envVars   map[string]string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "all credentials present",
			envVars: map[string]string{
				"CLOUDFLARE_ACCOUNT_ID": "test-account",
				"CLOUDFLARE_API_TOKEN":  "test-token",
				"CLOUDFLARE_DRY_RUN":    "",
			},
			wantErr: false,
		},
		{
			name: "missing account ID",
			envVars: map[string]string{
				"CLOUDFLARE_ACCOUNT_ID": "",
				"CLOUDFLARE_API_TOKEN":  "test-token",
				"CLOUDFLARE_DRY_RUN":    "",
			},
			wantErr:   true,
			errSubstr: "CLOUDFLARE_ACCOUNT_ID",
		},
		{
			name: "missing API token",
			envVars: map[string]string{
				"CLOUDFLARE_ACCOUNT_ID": "test-account",
				"CLOUDFLARE_API_TOKEN":  "",
				"CLOUDFLARE_DRY_RUN":    "",
			},
			wantErr:   true,
			errSubstr: "CLOUDFLARE_API_TOKEN",
		},
		{
			name: "both missing",
			envVars: map[string]string{
				"CLOUDFLARE_ACCOUNT_ID": "",
				"CLOUDFLARE_API_TOKEN":  "",
				"CLOUDFLARE_DRY_RUN":    "",
			},
			wantErr: true,
		},
		{
			name: "dry run mode skips check",
			envVars: map[string]string{
				"CLOUDFLARE_ACCOUNT_ID": "",
				"CLOUDFLARE_API_TOKEN":  "",
				"CLOUDFLARE_DRY_RUN":    "true",
			},
			wantErr: false,
		},
		{
			name: "dry run mode case insensitive",
			envVars: map[string]string{
				"CLOUDFLARE_ACCOUNT_ID": "",
				"CLOUDFLARE_API_TOKEN":  "",
				"CLOUDFLARE_DRY_RUN":    "TRUE",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env vars
			saved := map[string]string{}
			for k := range tt.envVars {
				saved[k] = os.Getenv(k)
			}
			defer func() {
				for k, v := range saved {
					os.Setenv(k, v)
				}
			}()

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			err := validateCredentials()
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCredentials() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errSubstr != "" {
				if got := err.Error(); !contains(got, tt.errSubstr) {
					t.Errorf("error %q should contain %q", got, tt.errSubstr)
				}
			}
		})
	}
}

func TestResolveWatchNamespace(t *testing.T) {
	tests := []struct {
		name           string
		watchNamespace string
		podNamespace   string
		want           string
	}{
		{
			name:           "WATCH_NAMESPACE set",
			watchNamespace: "target-ns",
			podNamespace:   "pod-ns",
			want:           "target-ns",
		},
		{
			name:           "fallback to POD_NAMESPACE",
			watchNamespace: "",
			podNamespace:   "pod-ns",
			want:           "pod-ns",
		},
		{
			name:           "neither set",
			watchNamespace: "",
			podNamespace:   "",
			want:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore
			savedWatch := os.Getenv("WATCH_NAMESPACE")
			savedPod := os.Getenv("POD_NAMESPACE")
			defer func() {
				os.Setenv("WATCH_NAMESPACE", savedWatch)
				os.Setenv("POD_NAMESPACE", savedPod)
			}()

			os.Setenv("WATCH_NAMESPACE", tt.watchNamespace)
			os.Setenv("POD_NAMESPACE", tt.podNamespace)

			got := resolveWatchNamespace()
			if got != tt.want {
				t.Errorf("resolveWatchNamespace() = %q, want %q", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
