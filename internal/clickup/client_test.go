package clickup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestGetTasksErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(server, "list123")
	_, err := client.GetTasks(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestGetTaskErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server, "list123")
	_, err := client.GetTask(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestUpdateTaskStatusErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(server, "list123")
	err := client.UpdateTaskStatus(context.Background(), "task1", "closed")
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}
