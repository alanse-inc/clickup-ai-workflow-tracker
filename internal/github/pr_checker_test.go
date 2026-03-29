package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// prCheckerTestServer はブランチ検索 (/repos/) と Search API (/search/) を
// 個別に制御できるテストサーバーを構築する。
type prCheckerTestServer struct {
	branchStatus int
	branchBody   string
	searchStatus int
	searchBody   string
	branchCalled bool
	searchCalled bool
	capturedReq  *http.Request // ブランチ検索のリクエスト
	searchReq    *http.Request // Search API のリクエスト
}

func (s *prCheckerTestServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			s.branchCalled = true
			s.capturedReq = r
			w.WriteHeader(s.branchStatus)
			if s.branchBody != "" {
				_, _ = w.Write([]byte(s.branchBody))
			}
		case strings.HasPrefix(r.URL.Path, "/search/"):
			s.searchCalled = true
			s.searchReq = r
			w.WriteHeader(s.searchStatus)
			if s.searchBody != "" {
				_, _ = w.Write([]byte(s.searchBody))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

const (
	emptySearchResult  = `{"items":[]}`
	mergedSearchResult = `{"items":[{"pull_request":{"merged_at":"2024-01-01T00:00:00Z"}}]}`
)

func setupPRChecker(ts *prCheckerTestServer) (*GitHubPRChecker, *httptest.Server) {
	server := httptest.NewServer(ts.handler())
	c := NewPRChecker(NewPATAuthenticator("test-token"), "test-owner", "test-repo")
	c.httpClient = server.Client()
	c.httpClient.Transport = &rewriteTransport{
		base:    http.DefaultTransport,
		baseURL: server.URL,
	}
	return c, server
}

func TestIsFeaturePRMerged_BranchSearch(t *testing.T) {
	tests := []struct {
		name         string
		branchStatus int
		branchBody   string
		searchStatus int
		searchBody   string
		want         bool
		wantErr      bool
		wantSearch   bool // フォールバック呼ばれるか
	}{
		{
			name:         "ブランチでマージ済み発見、フォールバック不要",
			branchStatus: http.StatusOK,
			branchBody:   `[{"merged_at":"2024-01-01T00:00:00Z"}]`,
			searchStatus: http.StatusOK,
			searchBody:   emptySearchResult,
			want:         true,
			wantSearch:   false,
		},
		{
			name:         "未マージ PR、フォールバック空",
			branchStatus: http.StatusOK,
			branchBody:   `[{"merged_at":null}]`,
			searchStatus: http.StatusOK,
			searchBody:   emptySearchResult,
			want:         false,
			wantSearch:   true,
		},
		{
			name:         "PR 空配列、フォールバック空",
			branchStatus: http.StatusOK,
			branchBody:   `[]`,
			searchStatus: http.StatusOK,
			searchBody:   emptySearchResult,
			want:         false,
			wantSearch:   true,
		},
		{
			name:         "404、フォールバック空",
			branchStatus: http.StatusNotFound,
			branchBody:   `{"message":"Not Found"}`,
			searchStatus: http.StatusOK,
			searchBody:   emptySearchResult,
			want:         false,
			wantSearch:   true,
		},
		{
			name:         "ブランチ 500、フォールバックでマージ発見",
			branchStatus: http.StatusInternalServerError,
			branchBody:   `{"message":"Internal Server Error"}`,
			searchStatus: http.StatusOK,
			searchBody:   mergedSearchResult,
			want:         true,
			wantSearch:   true,
		},
		{
			name:         "ブランチ 500、フォールバック空",
			branchStatus: http.StatusInternalServerError,
			branchBody:   `{"message":"Internal Server Error"}`,
			searchStatus: http.StatusOK,
			searchBody:   emptySearchResult,
			want:         false,
			wantSearch:   true,
		},
		{
			name:         "ブランチ 500、フォールバックも 500",
			branchStatus: http.StatusInternalServerError,
			branchBody:   `{"message":"Internal Server Error"}`,
			searchStatus: http.StatusInternalServerError,
			searchBody:   `{"message":"Internal Server Error"}`,
			wantErr:      true,
			wantSearch:   true,
		},
		{
			name:         "ブランチ空、フォールバック 500",
			branchStatus: http.StatusOK,
			branchBody:   `[]`,
			searchStatus: http.StatusInternalServerError,
			searchBody:   `{"message":"Internal Server Error"}`,
			wantErr:      true,
			wantSearch:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &prCheckerTestServer{
				branchStatus: tt.branchStatus,
				branchBody:   tt.branchBody,
				searchStatus: tt.searchStatus,
				searchBody:   tt.searchBody,
			}
			c, server := setupPRChecker(ts)
			defer server.Close()

			got, err := c.IsFeaturePRMerged(context.Background(), "task123")

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
				t.Errorf("IsFeaturePRMerged() = %v, want %v", got, tt.want)
			}
			if ts.searchCalled != tt.wantSearch {
				t.Errorf("search called = %v, want %v", ts.searchCalled, tt.wantSearch)
			}
		})
	}
}

func TestIsFeaturePRMerged_BranchRequestParams(t *testing.T) {
	ts := &prCheckerTestServer{
		branchStatus: http.StatusOK,
		branchBody:   `[{"merged_at":"2024-01-01T00:00:00Z"}]`,
		searchStatus: http.StatusOK,
		searchBody:   emptySearchResult,
	}
	c, server := setupPRChecker(ts)
	defer server.Close()

	_, err := c.IsFeaturePRMerged(context.Background(), "task123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := ts.capturedReq
	if req == nil {
		t.Fatal("expected branch request to be captured")
	}
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
	if got := req.URL.Path; got != "/repos/test-owner/test-repo/pulls" {
		t.Errorf("path = %s, want /repos/test-owner/test-repo/pulls", got)
	}
	if got := req.URL.Query().Get("head"); got != "test-owner:feature/clickup-task123" {
		t.Errorf("head = %s, want test-owner:feature/clickup-task123", got)
	}
	if got := req.URL.Query().Get("state"); got != "all" {
		t.Errorf("state = %s, want all", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %s, want Bearer test-token", got)
	}
}

func TestIsFeaturePRMerged_FallbackSearchParams(t *testing.T) {
	ts := &prCheckerTestServer{
		branchStatus: http.StatusOK,
		branchBody:   `[]`,
		searchStatus: http.StatusOK,
		searchBody:   emptySearchResult,
	}
	c, server := setupPRChecker(ts)
	defer server.Close()

	_, err := c.IsFeaturePRMerged(context.Background(), "task123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := ts.searchReq
	if req == nil {
		t.Fatal("expected search request to be captured")
	}
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
	if req.URL.Path != "/search/issues" {
		t.Errorf("path = %s, want /search/issues", req.URL.Path)
	}
	q := req.URL.Query().Get("q")
	for _, want := range []string{`"Closes CU-task123"`, "in:body", "type:pr", "repo:test-owner/test-repo", "is:merged"} {
		if !strings.Contains(q, want) {
			t.Errorf("q param %q does not contain %q", q, want)
		}
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %s, want Bearer test-token", got)
	}
}

func TestIsFeaturePRMerged_MultiplePRs(t *testing.T) {
	ts := &prCheckerTestServer{
		branchStatus: http.StatusOK,
		branchBody:   `[{"merged_at":null},{"merged_at":"2024-01-01T00:00:00Z"}]`,
		searchStatus: http.StatusOK,
		searchBody:   emptySearchResult,
	}
	c, server := setupPRChecker(ts)
	defer server.Close()

	got, err := c.IsFeaturePRMerged(context.Background(), "task456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true when one of multiple PRs is merged")
	}
}

func TestIsFeaturePRMerged_InvalidJSON(t *testing.T) {
	ts := &prCheckerTestServer{
		branchStatus: http.StatusOK,
		branchBody:   `invalid json`,
		searchStatus: http.StatusOK,
		searchBody:   emptySearchResult,
	}
	c, server := setupPRChecker(ts)
	defer server.Close()

	// ブランチ検索が不正 JSON → エラー → フォールバックへ
	// フォールバックは空 → false
	got, err := c.IsFeaturePRMerged(context.Background(), "task789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false when branch returns invalid JSON and fallback is empty")
	}
}

func TestIsFeaturePRMerged_FallbackFindsAfterBranchUnmerged(t *testing.T) {
	ts := &prCheckerTestServer{
		branchStatus: http.StatusOK,
		branchBody:   `[{"merged_at":null}]`,
		searchStatus: http.StatusOK,
		searchBody:   mergedSearchResult,
	}
	c, server := setupPRChecker(ts)
	defer server.Close()

	got, err := c.IsFeaturePRMerged(context.Background(), "task123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true when fallback finds merged PR")
	}
}

func TestIsSpecPRMerged(t *testing.T) {
	tests := []struct {
		name         string
		branchStatus int
		branchBody   string
		searchStatus int
		searchBody   string
		want         bool
		wantErr      bool
		wantSearch   bool
	}{
		{
			name:         "ブランチでマージ済み発見",
			branchStatus: http.StatusOK,
			branchBody:   `[{"merged_at":"2024-01-01T00:00:00Z"}]`,
			searchStatus: http.StatusOK,
			searchBody:   emptySearchResult,
			want:         true,
			wantSearch:   false,
		},
		{
			name:         "未マージ、フォールバック空",
			branchStatus: http.StatusOK,
			branchBody:   `[{"merged_at":null}]`,
			searchStatus: http.StatusOK,
			searchBody:   emptySearchResult,
			want:         false,
			wantSearch:   true,
		},
		{
			name:         "空、フォールバックでマージ発見",
			branchStatus: http.StatusOK,
			branchBody:   `[]`,
			searchStatus: http.StatusOK,
			searchBody:   mergedSearchResult,
			want:         true,
			wantSearch:   true,
		},
		{
			name:         "ブランチ 500、フォールバック成功",
			branchStatus: http.StatusInternalServerError,
			branchBody:   `{"message":"Internal Server Error"}`,
			searchStatus: http.StatusOK,
			searchBody:   mergedSearchResult,
			want:         true,
			wantSearch:   true,
		},
		{
			name:         "ブランチ 500、フォールバックも 500",
			branchStatus: http.StatusInternalServerError,
			branchBody:   `{"message":"Internal Server Error"}`,
			searchStatus: http.StatusInternalServerError,
			searchBody:   `{"message":"Internal Server Error"}`,
			wantErr:      true,
			wantSearch:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &prCheckerTestServer{
				branchStatus: tt.branchStatus,
				branchBody:   tt.branchBody,
				searchStatus: tt.searchStatus,
				searchBody:   tt.searchBody,
			}
			c, server := setupPRChecker(ts)
			defer server.Close()

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
			if ts.searchCalled != tt.wantSearch {
				t.Errorf("search called = %v, want %v", ts.searchCalled, tt.wantSearch)
			}
		})
	}
}

func TestIsSpecPRMerged_BranchRequestParams(t *testing.T) {
	ts := &prCheckerTestServer{
		branchStatus: http.StatusOK,
		branchBody:   `[{"merged_at":"2024-01-01T00:00:00Z"}]`,
		searchStatus: http.StatusOK,
		searchBody:   emptySearchResult,
	}
	c, server := setupPRChecker(ts)
	defer server.Close()

	_, err := c.IsSpecPRMerged(context.Background(), "task123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := ts.capturedReq
	if req == nil {
		t.Fatal("expected request to be captured")
	}
	if got := req.URL.Query().Get("head"); got != "test-owner:spec/clickup-task123" {
		t.Errorf("head = %s, want test-owner:spec/clickup-task123", got)
	}
}

func TestIsSpecPRMerged_FallbackSearchParams(t *testing.T) {
	ts := &prCheckerTestServer{
		branchStatus: http.StatusOK,
		branchBody:   `[]`,
		searchStatus: http.StatusOK,
		searchBody:   emptySearchResult,
	}
	c, server := setupPRChecker(ts)
	defer server.Close()

	_, err := c.IsSpecPRMerged(context.Background(), "task123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := ts.searchReq
	if req == nil {
		t.Fatal("expected search request to be captured")
	}
	q := req.URL.Query().Get("q")
	// SPEC フェーズは "Refs CU-" マーカーを使う
	if !strings.Contains(q, `"Refs CU-task123"`) {
		t.Errorf("q param %q does not contain '\"Refs CU-task123\"'", q)
	}
	for _, want := range []string{"in:body", "type:pr", "repo:test-owner/test-repo", "is:merged"} {
		if !strings.Contains(q, want) {
			t.Errorf("q param %q does not contain %q", q, want)
		}
	}
}

func TestIsFeaturePRMerged_NetworkError(t *testing.T) {
	ts := &prCheckerTestServer{
		branchStatus: http.StatusOK,
		branchBody:   `[]`,
		searchStatus: http.StatusOK,
		searchBody:   emptySearchResult,
	}
	c, server := setupPRChecker(ts)
	server.Close() // 両方のエンドポイントがネットワークエラーになる

	_, err := c.IsFeaturePRMerged(context.Background(), "task123")
	if err == nil {
		t.Fatal("expected error for network failure, got nil")
	}
}
