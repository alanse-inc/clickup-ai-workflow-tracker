package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsPRMerged(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		closeServer bool
		want        bool
		wantErr     bool
	}{
		{
			name:       "マージ済み PR が存在する",
			statusCode: http.StatusOK,
			body:       `[{"merged_at":"2024-01-01T00:00:00Z"}]`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "未マージ PR が存在する",
			statusCode: http.StatusOK,
			body:       `[{"merged_at":null}]`,
			want:       false,
			wantErr:    false,
		},
		{
			name:       "PR が存在しない（空配列）",
			statusCode: http.StatusOK,
			body:       `[]`,
			want:       false,
			wantErr:    false,
		},
		{
			name:       "PR が存在しない（404）",
			statusCode: http.StatusNotFound,
			body:       `{"message":"Not Found"}`,
			want:       false,
			wantErr:    false,
		},
		{
			name:       "GitHub API エラー（500）",
			statusCode: http.StatusInternalServerError,
			body:       `{"message":"Internal Server Error"}`,
			want:       false,
			wantErr:    true,
		},
		{
			name:       "認証エラー（401）",
			statusCode: http.StatusUnauthorized,
			body:       `{"message":"Bad credentials"}`,
			want:       false,
			wantErr:    true,
		},
		{
			name:        "ネットワークエラー",
			closeServer: true,
			want:        false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedReq *http.Request

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedReq = r
				w.WriteHeader(tt.statusCode)
				if tt.body != "" {
					_, _ = w.Write([]byte(tt.body))
				}
			}))

			c := NewPRChecker(NewPATAuthenticator("test-token"), "test-owner", "test-repo")
			c.httpClient = server.Client()
			c.httpClient.Transport = &rewriteTransport{
				base:    http.DefaultTransport,
				baseURL: server.URL,
			}

			if tt.closeServer {
				server.Close()
			} else {
				defer server.Close()
			}

			got, err := c.IsPRMerged(context.Background(), "task123")

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("IsPRMerged() = %v, want %v", got, tt.want)
			}

			if tt.closeServer {
				return
			}

			// 正常ケースでリクエスト検証
			if capturedReq == nil {
				t.Fatal("expected request to be captured")
			}
			if capturedReq.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", capturedReq.Method)
			}
			wantPath := "/repos/test-owner/test-repo/pulls"
			if capturedReq.URL.Path != wantPath {
				t.Errorf("path = %s, want %s", capturedReq.URL.Path, wantPath)
			}
			if got := capturedReq.URL.Query().Get("head"); got != "test-owner:feature/clickup-task123" {
				t.Errorf("head param = %s, want test-owner:feature/clickup-task123", got)
			}
			if got := capturedReq.URL.Query().Get("state"); got != "all" {
				t.Errorf("state param = %s, want all", got)
			}
			if got := capturedReq.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("Authorization = %s, want Bearer test-token", got)
			}
		})
	}
}

func TestIsPRMerged_MultiplePRs(t *testing.T) {
	// 複数 PR のうち 1 件でもマージ済みなら true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"merged_at":null},{"merged_at":"2024-01-01T00:00:00Z"}]`))
	}))
	defer server.Close()

	c := NewPRChecker(NewPATAuthenticator("test-token"), "test-owner", "test-repo")
	c.httpClient = server.Client()
	c.httpClient.Transport = &rewriteTransport{
		base:    http.DefaultTransport,
		baseURL: server.URL,
	}

	got, err := c.IsPRMerged(context.Background(), "task456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true when one of multiple PRs is merged")
	}
}

func TestIsPRMerged_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	c := NewPRChecker(NewPATAuthenticator("test-token"), "test-owner", "test-repo")
	c.httpClient = server.Client()
	c.httpClient.Transport = &rewriteTransport{
		base:    http.DefaultTransport,
		baseURL: server.URL,
	}

	_, err := c.IsPRMerged(context.Background(), "task789")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decoding response") {
		t.Errorf("error %q does not contain 'decoding response'", err.Error())
	}
}

func TestIsSpecPRMerged(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		closeServer bool
		want        bool
		wantErr     bool
		wantHead    string
	}{
		{
			name:       "マージ済み SPEC PR が存在する",
			statusCode: http.StatusOK,
			body:       `[{"merged_at":"2024-01-01T00:00:00Z"}]`,
			want:       true,
			wantHead:   "test-owner:spec/clickup-task123",
		},
		{
			name:       "未マージ SPEC PR が存在する",
			statusCode: http.StatusOK,
			body:       `[{"merged_at":null}]`,
			want:       false,
			wantHead:   "test-owner:spec/clickup-task123",
		},
		{
			name:       "SPEC PR が存在しない",
			statusCode: http.StatusOK,
			body:       `[]`,
			want:       false,
			wantHead:   "test-owner:spec/clickup-task123",
		},
		{
			name:       "GitHub API エラー（500）",
			statusCode: http.StatusInternalServerError,
			body:       `{"message":"Internal Server Error"}`,
			want:       false,
			wantErr:    true,
		},
		{
			name:        "ネットワークエラー",
			closeServer: true,
			want:        false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedReq *http.Request

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedReq = r
				w.WriteHeader(tt.statusCode)
				if tt.body != "" {
					_, _ = w.Write([]byte(tt.body))
				}
			}))

			c := NewPRChecker(NewPATAuthenticator("test-token"), "test-owner", "test-repo")
			c.httpClient = server.Client()
			c.httpClient.Transport = &rewriteTransport{
				base:    http.DefaultTransport,
				baseURL: server.URL,
			}

			if tt.closeServer {
				server.Close()
			} else {
				defer server.Close()
			}

			got, err := c.IsSpecPRMerged(context.Background(), "task123")

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("IsSpecPRMerged() = %v, want %v", got, tt.want)
			}

			if tt.closeServer {
				return
			}

			// 正常ケースでリクエスト検証
			if capturedReq == nil {
				t.Fatal("expected request to be captured")
			}
			if capturedReq.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", capturedReq.Method)
			}
			if got := capturedReq.URL.Query().Get("head"); got != tt.wantHead {
				t.Errorf("head param = %s, want %s", got, tt.wantHead)
			}
			if got := capturedReq.URL.Query().Get("state"); got != "all" {
				t.Errorf("state param = %s, want all", got)
			}
			if got := capturedReq.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("Authorization = %s, want Bearer test-token", got)
			}
		})
	}
}
