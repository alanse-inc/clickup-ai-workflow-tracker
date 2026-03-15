package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPATAuthenticator_SetAuth(t *testing.T) {
	auth := NewPATAuthenticator("ghp_test123")
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com", nil)
	auth.SetAuth(req)

	got := req.Header.Get("Authorization")
	want := "Bearer ghp_test123"
	if got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func generateTestPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	return key
}

func marshalPrivateKeyPEM(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

func TestNewGitHubAppAuthenticator(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	tests := []struct {
		name        string
		appID       int64
		installID   int64
		privateKey  []byte
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid key",
			appID:      12345,
			installID:  67890,
			privateKey: pemBytes,
		},
		{
			name:        "invalid PEM",
			appID:       12345,
			installID:   67890,
			privateKey:  []byte("not-a-pem"),
			wantErr:     true,
			errContains: "failed to decode PEM",
		},
		{
			name:        "invalid key bytes",
			appID:       12345,
			installID:   67890,
			privateKey:  pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("bad")}),
			wantErr:     true,
			errContains: "failed to parse private key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewGitHubAppAuthenticator(tt.appID, tt.installID, tt.privateKey)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if auth == nil {
				t.Fatal("expected non-nil authenticator")
			}
		})
	}
}

func TestGitHubAppAuthenticator_SetAuth(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	// Installation token レスポンスを返すモックサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// JWT が Bearer ヘッダーに含まれていることを確認
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// installation token レスポンス
		resp := installationTokenResponse{
			Token:     "ghs_test_installation_token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	auth, err := NewGitHubAppAuthenticator(12345, 67890, pemBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth.baseURL = server.URL

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com", nil)
	auth.SetAuth(req)

	got := req.Header.Get("Authorization")
	want := "Bearer ghs_test_installation_token"
	if got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestGitHubAppAuthenticator_TokenCache(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	tokenValue := "ghs_cached_token" //nolint:gosec // test value
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := installationTokenResponse{
			Token:     tokenValue,
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	auth, err := NewGitHubAppAuthenticator(12345, 67890, pemBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth.baseURL = server.URL

	// 2回呼んでも API は1回しか叩かれない
	req1, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com", nil)
	auth.SetAuth(req1)
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com", nil)
	auth.SetAuth(req2)

	if callCount != 1 {
		t.Errorf("API call count = %d, want 1 (token should be cached)", callCount)
	}
}

func TestGitHubAppAuthenticator_TokenRefresh(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := installationTokenResponse{
			Token:     "ghs_refreshed_token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	auth, err := NewGitHubAppAuthenticator(12345, 67890, pemBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth.baseURL = server.URL

	// 初回取得
	req1, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com", nil)
	auth.SetAuth(req1)

	// トークンを期限切れにする
	auth.mu.Lock()
	auth.expiresAt = time.Now().Add(-1 * time.Minute)
	auth.mu.Unlock()

	// 2回目はリフレッシュされるべき
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com", nil)
	auth.SetAuth(req2)

	if callCount != 2 {
		t.Errorf("API call count = %d, want 2 (token should be refreshed)", callCount)
	}
}
