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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	return req
}

func TestPATAuthenticator_SetAuth(t *testing.T) {
	auth := NewPATAuthenticator("ghp_test123")
	req := newTestRequest(t)
	err := auth.SetAuth(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

func marshalPKCS8PEM(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal PKCS#8 key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})
}

func TestNewGitHubAppAuthenticator(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)
	pkcs8PEM := marshalPKCS8PEM(t, key)

	tests := []struct {
		name        string
		appID       int64
		installID   int64
		privateKey  []byte
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid PKCS#1 key",
			appID:      12345,
			installID:  67890,
			privateKey: pemBytes,
		},
		{
			name:       "valid PKCS#8 key",
			appID:      12345,
			installID:  67890,
			privateKey: pkcs8PEM,
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
			errContains: "failed to parse",
		},
		{
			name:        "unsupported PEM type",
			appID:       12345,
			installID:   67890,
			privateKey:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("data")}),
			wantErr:     true,
			errContains: "unsupported PEM block type",
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
		resp := installationTokenResponse{ //nolint:gosec // test value
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

	req := newTestRequest(t)
	err = auth.SetAuth(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := req.Header.Get("Authorization")
	want := "Bearer ghs_test_installation_token"
	if got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestGitHubAppAuthenticator_SetAuth_Error(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	// トークン取得が常に失敗するモックサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal error"}`))
	}))
	defer server.Close()

	auth, err := NewGitHubAppAuthenticator(12345, 67890, pemBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth.baseURL = server.URL

	// キャッシュトークンなし + リフレッシュ失敗 → エラーが返る
	req := newTestRequest(t)
	err = auth.SetAuth(req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to set auth") {
		t.Errorf("error %q does not contain expected message", err.Error())
	}
	// Authorization ヘッダーが設定されていないことを確認
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be empty, got %q", got)
	}
}

func TestGitHubAppAuthenticator_SetAuth_FallbackOnRefreshError(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		if count == 1 {
			// 初回は成功
			resp := installationTokenResponse{ //nolint:gosec // test value
				Token:     "ghs_valid_token",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			// 2回目以降は失敗
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	auth, err := NewGitHubAppAuthenticator(12345, 67890, pemBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth.baseURL = server.URL

	// 初回: 成功
	req1 := newTestRequest(t)
	if err := auth.SetAuth(req1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// トークンを期限切れ間近にする（リフレッシュが試行されるが、まだ有効）
	auth.mu.Lock()
	auth.expiresAt = time.Now().Add(2 * time.Minute) // tokenRefreshMargin(5min) 以内だがまだ未来
	auth.mu.Unlock()

	// 2回目: リフレッシュ失敗 → キャッシュトークンにフォールバック → エラーなし
	req2 := newTestRequest(t)
	err = auth.SetAuth(req2)
	if err != nil {
		t.Fatalf("expected fallback to cached token, got error: %v", err)
	}
	if got := req2.Header.Get("Authorization"); got != "Bearer ghs_valid_token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer ghs_valid_token")
	}
}

func TestGitHubAppAuthenticator_TokenCache(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	tokenValue := "ghs_cached_token" //nolint:gosec // test value
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
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
	req1 := newTestRequest(t)
	if err := auth.SetAuth(req1); err != nil {
		t.Fatalf("unexpected error on req1: %v", err)
	}
	req2 := newTestRequest(t)
	if err := auth.SetAuth(req2); err != nil {
		t.Fatalf("unexpected error on req2: %v", err)
	}

	if got := callCount.Load(); got != 1 {
		t.Errorf("API call count = %d, want 1 (token should be cached)", got)
	}
}

func TestGitHubAppAuthenticator_TokenRefresh(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		resp := installationTokenResponse{ //nolint:gosec // test value
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
	req1 := newTestRequest(t)
	if err := auth.SetAuth(req1); err != nil {
		t.Fatalf("unexpected error on req1: %v", err)
	}

	// トークンを期限切れにする
	auth.mu.Lock()
	auth.expiresAt = time.Now().Add(-1 * time.Minute)
	auth.mu.Unlock()

	// 2回目はリフレッシュされるべき
	req2 := newTestRequest(t)
	if err := auth.SetAuth(req2); err != nil {
		t.Fatalf("unexpected error on req2: %v", err)
	}

	if got := callCount.Load(); got != 2 {
		t.Errorf("API call count = %d, want 2 (token should be refreshed)", got)
	}
}

func TestGitHubAppAuthenticator_TokenRefreshBoundary(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	tests := []struct {
		name          string
		expiresOffset time.Duration // relative to now when set after first call
		wantCallCount int32
	}{
		{
			name:          "valid_10_minutes_cached",
			expiresOffset: 10 * time.Minute,
			wantCallCount: 1, // within margin → cached
		},
		{
			name:          "refresh_3_minutes",
			expiresOffset: 3 * time.Minute,
			wantCallCount: 2, // within tokenRefreshMargin (5min) → refresh
		},
		{
			name:          "expired",
			expiresOffset: -1 * time.Second,
			wantCallCount: 2, // past expiry → refresh
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var callCount atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount.Add(1)
				resp := installationTokenResponse{ //nolint:gosec // test value
					Token:     "ghs_boundary_token",
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

			// First call: always fetches a fresh token
			req1 := newTestRequest(t)
			if err := auth.SetAuth(req1); err != nil {
				t.Fatalf("unexpected error on first call: %v", err)
			}

			// Override expiresAt to the test boundary
			auth.mu.Lock()
			auth.expiresAt = time.Now().Add(tt.expiresOffset)
			auth.mu.Unlock()

			// Second call: may or may not refresh depending on boundary
			req2 := newTestRequest(t)
			if err := auth.SetAuth(req2); err != nil {
				t.Fatalf("unexpected error on second call: %v", err)
			}

			if got := callCount.Load(); got != tt.wantCallCount {
				t.Errorf("API call count = %d, want %d", got, tt.wantCallCount)
			}
		})
	}
}

func TestGitHubAppAuthenticator_ConcurrentSetAuth(t *testing.T) {
	key := generateTestPrivateKey(t)
	pemBytes := marshalPrivateKeyPEM(key)

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		resp := installationTokenResponse{ //nolint:gosec // test value
			Token:     "ghs_concurrent_token",
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

	const goroutines = 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			req := newTestRequest(t)
			errs[idx] = auth.SetAuth(req)
		}(i)
	}

	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
	}

	if got := callCount.Load(); got != 1 {
		t.Errorf("API call count = %d, want 1 (mutex serializes concurrent callers)", got)
	}
}
