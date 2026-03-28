package clickup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func newTestClient(server *httptest.Server, listID string) *Client {
	c := NewClient("test-token", listID)
	c.baseURL = server.URL + "/api/v2"
	c.httpClient = server.Client()
	return c
}

func TestGetTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/list/list123/task" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "test-token" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}

		resp := map[string]any{
			"tasks": []map[string]any{
				{
					"id":          "task1",
					"name":        "Test Task",
					"description": "desc",
					"status":      map[string]any{"status": "Ready For Spec"},
					"custom_fields": []map[string]any{
						{"name": "GitHub PR URL", "value": "https://github.com/pr/1"},
					},
					"date_created": "1234567890",
					"date_updated": "1234567891",
				},
			},
			"last_page": true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(server, "list123")
	tasks, err := client.GetTasks(context.Background())
	if err != nil {
		t.Fatalf("GetTasks() error = %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.ID != "task1" {
		t.Errorf("expected ID task1, got %s", task.ID)
	}
	if task.Name != "Test Task" {
		t.Errorf("expected name 'Test Task', got %s", task.Name)
	}
	if task.Status != "ready for spec" {
		t.Errorf("expected status 'ready for spec', got %s", task.Status)
	}
	if task.CustomFields["github_pr_url"] != "https://github.com/pr/1" {
		t.Errorf("expected custom field github_pr_url, got %v", task.CustomFields)
	}
}

func TestGetTasksPagination(t *testing.T) {
	makeTask := func(id string) map[string]any {
		return map[string]any{
			"id":            id,
			"name":          "Task " + id,
			"description":   "",
			"status":        map[string]any{"status": "open"},
			"custom_fields": []map[string]any{},
			"date_created":  "0",
			"date_updated":  "0",
		}
	}

	make100Tasks := func() []map[string]any {
		tasks := make([]map[string]any, 100)
		for i := range tasks {
			tasks[i] = makeTask(fmt.Sprintf("t%d", i))
		}
		return tasks
	}

	tests := []struct {
		name          string
		pages         []map[string]any // each element is a page response
		wantTaskCount int
		wantCallCount int
		wantErr       bool
	}{
		{
			name: "single_page",
			pages: []map[string]any{
				{"tasks": []map[string]any{makeTask("t1")}, "last_page": true},
			},
			wantTaskCount: 1,
			wantCallCount: 1,
		},
		{
			name: "two_pages",
			pages: []map[string]any{
				{"tasks": []map[string]any{makeTask("t1")}, "last_page": false},
				{"tasks": []map[string]any{makeTask("t2")}, "last_page": true},
			},
			wantTaskCount: 2,
			wantCallCount: 2,
		},
		{
			name: "empty_list",
			pages: []map[string]any{
				{"tasks": []map[string]any{}, "last_page": true},
			},
			wantTaskCount: 0,
			wantCallCount: 1,
		},
		{
			name: "exactly_100_then_empty",
			pages: []map[string]any{
				{"tasks": make100Tasks(), "last_page": false},
				{"tasks": []map[string]any{}, "last_page": true},
			},
			wantTaskCount: 100,
			wantCallCount: 2,
		},
		{
			name: "error_on_second_page",
			pages: []map[string]any{
				{"tasks": []map[string]any{makeTask("t1")}, "last_page": false},
			},
			wantErr:       true,
			wantCallCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var callCount atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := 0
				if p := r.URL.Query().Get("page"); p != "" {
					_, _ = fmt.Sscanf(p, "%d", &page)
				}
				callCount.Add(1)

				if tt.wantErr && page >= len(tt.pages) {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if page >= len(tt.pages) {
					t.Errorf("unexpected page request: %d", page)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tt.pages[page])
			}))
			defer server.Close()

			client := newTestClient(server, "list123")
			tasks, err := client.GetTasks(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("GetTasks() error = %v", err)
				}
				if len(tasks) != tt.wantTaskCount {
					t.Errorf("expected %d tasks, got %d", tt.wantTaskCount, len(tasks))
				}
			}

			if got := int(callCount.Load()); got != tt.wantCallCount {
				t.Errorf("expected %d API calls, got %d", tt.wantCallCount, got)
			}
		})
	}
}

func TestGetTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/task/task1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]any{
			"id":            "task1",
			"name":          "Single Task",
			"description":   "single desc",
			"status":        map[string]any{"status": "Implementing"},
			"custom_fields": []map[string]any{},
			"date_created":  "1234567890",
			"date_updated":  "1234567891",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(server, "list123")
	task, err := client.GetTask(context.Background(), "task1")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}

	if task.ID != "task1" {
		t.Errorf("expected ID task1, got %s", task.ID)
	}
	if task.Status != "implementing" {
		t.Errorf("expected status 'implementing', got %s", task.Status)
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/task/task1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		if payload["status"] != "implementing" {
			t.Errorf("expected status 'implementing', got %s", payload["status"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(server, "list123")
	err := client.UpdateTaskStatus(context.Background(), "task1", "implementing")
	if err != nil {
		t.Fatalf("UpdateTaskStatus() error = %v", err)
	}
}

func TestGetStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/list/list123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]any{
			"statuses": []map[string]any{
				{"status": "Ready For Spec"},
				{"status": "Generating Spec"},
				{"status": "Closed"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(server, "list123")
	statuses, err := client.GetStatuses(context.Background())
	if err != nil {
		t.Fatalf("GetStatuses() error = %v", err)
	}

	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}
	// statuses should be lowercased
	expected := []string{"ready for spec", "generating spec", "closed"}
	for i, s := range expected {
		if statuses[i] != s {
			t.Errorf("statuses[%d] = %q, want %q", i, statuses[i], s)
		}
	}
}

func TestGetStatusesErrorResponse(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		errContains string
	}{
		{
			name:        "500 no body",
			statusCode:  http.StatusInternalServerError,
			body:        "",
			errContains: "unexpected status code: 500",
		},
		{
			name:        "429 with rate limit body",
			statusCode:  http.StatusTooManyRequests,
			body:        `{"err": "Rate limit exceeded"}`,
			errContains: `unexpected status code: 429: {"err": "Rate limit exceeded"}`,
		},
		{
			name:        "401 with auth error body",
			statusCode:  http.StatusUnauthorized,
			body:        `{"err": "Token invalid"}`,
			errContains: `unexpected status code: 401: {"err": "Token invalid"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := newTestClient(server, "list123")
			_, err := client.GetStatuses(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func TestGetTasksErrorResponse(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		errContains string
	}{
		{
			name:        "500 no body",
			statusCode:  http.StatusInternalServerError,
			body:        "",
			errContains: "unexpected status code: 500",
		},
		{
			name:        "429 with rate limit body",
			statusCode:  http.StatusTooManyRequests,
			body:        `{"err": "Rate limit exceeded"}`,
			errContains: `unexpected status code: 429: {"err": "Rate limit exceeded"}`,
		},
		{
			name:        "401 with auth error body",
			statusCode:  http.StatusUnauthorized,
			body:        `{"err": "Token invalid"}`,
			errContains: `unexpected status code: 401: {"err": "Token invalid"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := newTestClient(server, "list123")
			_, err := client.GetTasks(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func TestGetTaskErrorResponse(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		errContains string
	}{
		{
			name:        "404 no body",
			statusCode:  http.StatusNotFound,
			body:        "",
			errContains: "unexpected status code: 404",
		},
		{
			name:        "429 with rate limit body",
			statusCode:  http.StatusTooManyRequests,
			body:        `{"err": "Rate limit exceeded"}`,
			errContains: `unexpected status code: 429: {"err": "Rate limit exceeded"}`,
		},
		{
			name:        "401 with auth error body",
			statusCode:  http.StatusUnauthorized,
			body:        `{"err": "Token invalid"}`,
			errContains: `unexpected status code: 401: {"err": "Token invalid"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := newTestClient(server, "list123")
			_, err := client.GetTask(context.Background(), "nonexistent")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func TestUpdateTaskStatusErrorResponse(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		errContains string
	}{
		{
			name:        "403 no body",
			statusCode:  http.StatusForbidden,
			body:        "",
			errContains: "unexpected status code: 403",
		},
		{
			name:        "429 with rate limit body",
			statusCode:  http.StatusTooManyRequests,
			body:        `{"err": "Rate limit exceeded"}`,
			errContains: `unexpected status code: 429: {"err": "Rate limit exceeded"}`,
		},
		{
			name:        "401 with auth error body",
			statusCode:  http.StatusUnauthorized,
			body:        `{"err": "Token invalid"}`,
			errContains: `unexpected status code: 401: {"err": "Token invalid"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := newTestClient(server, "list123")
			err := client.UpdateTaskStatus(context.Background(), "task1", "closed")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func Test_readErrorBody(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"short json", `{"err":"foo"}`, `{"err":"foo"}`},
		{"trims whitespace", "body\n", "body"},
		{"truncates at 512 bytes", strings.Repeat("a", 600), strings.Repeat("a", 512)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readErrorBody(strings.NewReader(tt.input))
			if got != tt.want {
				t.Errorf("readErrorBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetTasks_CustomFieldNonStringValue(t *testing.T) {
	tests := []struct {
		name            string
		customFields    []map[string]any
		wantKeys        []string
		wantMissingKeys []string
	}{
		{
			name: "numeric_value_skipped",
			customFields: []map[string]any{
				{"name": "Score", "value": 123},
			},
			wantMissingKeys: []string{"score"},
		},
		{
			name: "bool_value_skipped",
			customFields: []map[string]any{
				{"name": "Active", "value": true},
			},
			wantMissingKeys: []string{"active"},
		},
		{
			name: "null_value_skipped",
			customFields: []map[string]any{
				{"name": "Empty Field", "value": nil},
			},
			wantMissingKeys: []string{"empty_field"},
		},
		{
			name: "mixed_string_and_non_string",
			customFields: []map[string]any{
				{"name": "GitHub PR URL", "value": "https://github.com/pr/1"},
				{"name": "Score", "value": 42},
				{"name": "Active", "value": false},
			},
			wantKeys:        []string{"github_pr_url"},
			wantMissingKeys: []string{"score", "active"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				resp := map[string]any{
					"tasks": []map[string]any{
						{
							"id":            "task1",
							"name":          "Test Task",
							"description":   "",
							"status":        map[string]any{"status": "open"},
							"custom_fields": tt.customFields,
							"date_created":  "0",
							"date_updated":  "0",
						},
					},
					"last_page": true,
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			client := newTestClient(server, "list123")
			tasks, err := client.GetTasks(context.Background())
			if err != nil {
				t.Fatalf("GetTasks() error = %v", err)
			}
			if len(tasks) != 1 {
				t.Fatalf("expected 1 task, got %d", len(tasks))
			}
			cf := tasks[0].CustomFields

			for _, key := range tt.wantKeys {
				if _, ok := cf[key]; !ok {
					t.Errorf("expected key %q in CustomFields, not found: %v", key, cf)
				}
			}
			for _, key := range tt.wantMissingKeys {
				if _, ok := cf[key]; ok {
					t.Errorf("expected key %q absent from CustomFields, but found: %v", key, cf)
				}
			}
		})
	}
}

func TestClient_Ping(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		closeServer bool
		wantErr     bool
		errContains string
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:        "auth error 401",
			statusCode:  http.StatusUnauthorized,
			wantErr:     true,
			errContains: "unexpected status code: 401",
		},
		{
			name:        "server error 500",
			statusCode:  http.StatusInternalServerError,
			wantErr:     true,
			errContains: "unexpected status code: 500",
		},
		{
			name:        "429 with rate limit body",
			statusCode:  http.StatusTooManyRequests,
			body:        `{"err": "Rate limit exceeded"}`,
			wantErr:     true,
			errContains: `unexpected status code: 429: {"err": "Rate limit exceeded"}`,
		},
		{
			name:        "401 with auth error body",
			statusCode:  http.StatusUnauthorized,
			body:        `{"err": "Token invalid"}`,
			wantErr:     true,
			errContains: `unexpected status code: 401: {"err": "Token invalid"}`,
		},
		{
			name:        "connection refused",
			closeServer: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/user" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))

			client := newTestClient(server, "list123")

			if tt.closeServer {
				server.Close()
			} else {
				defer server.Close()
			}

			err := client.Ping(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" {
					if !strings.Contains(err.Error(), tt.errContains) {
						t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
