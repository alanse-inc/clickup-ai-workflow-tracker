package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTriggerWorkflow(t *testing.T) {
	tests := []struct {
		name            string
		statusCode      int
		responseBody    string
		taskID          string
		phase           string
		statusOnSuccess string
		statusOnError   string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "success 204",
			statusCode:      http.StatusNoContent,
			responseBody:    "",
			taskID:          "task123",
			phase:           "SPEC",
			statusOnSuccess: "spec review",
			statusOnError:   "open",
			wantErr:         false,
		},
		{
			name:            "error 401 unauthorized",
			statusCode:      http.StatusUnauthorized,
			responseBody:    `{"message":"Bad credentials"}`,
			taskID:          "task456",
			phase:           "CODE",
			statusOnSuccess: "review",
			statusOnError:   "spec done",
			wantErr:         true,
			errContains:     "401",
		},
		{
			name:            "error 404 not found",
			statusCode:      http.StatusNotFound,
			responseBody:    `{"message":"Not Found"}`,
			taskID:          "task789",
			phase:           "SPEC",
			statusOnSuccess: "spec review",
			statusOnError:   "open",
			wantErr:         true,
			errContains:     "Not Found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedReq *http.Request
			var capturedBody []byte

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedReq = r
				capturedBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(tt.statusCode)
				if tt.responseBody != "" {
					_, _ = w.Write([]byte(tt.responseBody))
				}
			}))
			defer server.Close()

			d := NewDispatcher(NewPATAuthenticator("test-pat"), "test-owner", "test-repo", "agent.yml")
			// httpClient のベースURLをモックサーバーに差し替え
			d.httpClient = server.Client()
			// URL をモックサーバーに向けるためにカスタムトランスポートを設定
			d.httpClient.Transport = &rewriteTransport{
				base:    http.DefaultTransport,
				baseURL: server.URL,
			}

			err := d.TriggerWorkflow(context.Background(), tt.taskID, tt.phase, tt.statusOnSuccess, tt.statusOnError)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" {
					if got := err.Error(); !strings.Contains(got, tt.errContains) {
						t.Errorf("error %q does not contain %q", got, tt.errContains)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// HTTP メソッドの検証
			if capturedReq.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", capturedReq.Method)
			}

			// URL パスの検証
			wantPath := "/repos/test-owner/test-repo/actions/workflows/agent.yml/dispatches"
			if capturedReq.URL.Path != wantPath {
				t.Errorf("path = %s, want %s", capturedReq.URL.Path, wantPath)
			}

			// ヘッダーの検証
			if got := capturedReq.Header.Get("Authorization"); got != "Bearer test-pat" {
				t.Errorf("Authorization = %s, want Bearer test-pat", got)
			}
			if got := capturedReq.Header.Get("Accept"); got != "application/vnd.github.v3+json" {
				t.Errorf("Accept = %s, want application/vnd.github.v3+json", got)
			}
			if got := capturedReq.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %s, want application/json", got)
			}

			// リクエストボディの検証
			var body dispatchRequest
			if err := json.Unmarshal(capturedBody, &body); err != nil {
				t.Fatalf("failed to unmarshal request body: %v", err)
			}
			if body.Ref != "main" {
				t.Errorf("ref = %s, want main", body.Ref)
			}
			if body.Inputs["task_id"] != tt.taskID {
				t.Errorf("task_id = %s, want %s", body.Inputs["task_id"], tt.taskID)
			}
			if body.Inputs["phase"] != tt.phase {
				t.Errorf("phase = %s, want %s", body.Inputs["phase"], tt.phase)
			}
			if body.Inputs["status_on_success"] != tt.statusOnSuccess {
				t.Errorf("status_on_success = %s, want %s", body.Inputs["status_on_success"], tt.statusOnSuccess)
			}
			if body.Inputs["status_on_error"] != tt.statusOnError {
				t.Errorf("status_on_error = %s, want %s", body.Inputs["status_on_error"], tt.statusOnError)
			}
		})
	}
}

// rewriteTransport は全てのリクエストをモックサーバーに転送するカスタムトランスポート
type rewriteTransport struct {
	base    http.RoundTripper
	baseURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 元のパスを保持しつつ、ホストをモックサーバーに変更
	req.URL.Scheme = "http"
	req.URL.Host = t.baseURL[len("http://"):]
	return t.base.RoundTrip(req)
}
