package status

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/orchestrator"
)

type mockLimiter struct {
	active int
	max    int
}

func (m *mockLimiter) ActiveCount() int   { return m.active }
func (m *mockLimiter) MaxConcurrent() int { return m.max }

type mockProvider struct {
	status orchestrator.OrchestratorStatus
}

func (m *mockProvider) Status() orchestrator.OrchestratorStatus { return m.status }

func TestHandler_ServeHTTP(t *testing.T) {
	now := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	retryAfter := time.Date(2026, 3, 27, 10, 5, 0, 0, time.UTC)

	tests := []struct {
		name              string
		limiter           *mockLimiter
		providers         []StatusProvider
		wantActiveTasks   int
		wantMaxConcurrent int
		wantProjectCount  int
	}{
		{
			name:              "no active tasks",
			limiter:           &mockLimiter{active: 0, max: 5},
			providers:         []StatusProvider{},
			wantActiveTasks:   0,
			wantMaxConcurrent: 5,
			wantProjectCount:  0,
		},
		{
			name:    "running and retry present",
			limiter: &mockLimiter{active: 2, max: 5},
			providers: []StatusProvider{
				&mockProvider{status: orchestrator.OrchestratorStatus{
					Project: "owner/repo",
					RunningTasks: []orchestrator.RunningTaskInfo{
						{TaskID: "abc123", StartedAt: now},
					},
					RetryPending: []orchestrator.RetryInfo{
						{TaskID: "def456", Phase: "CODE", Attempt: 2, RetryAfter: retryAfter},
					},
				}},
			},
			wantActiveTasks:   2,
			wantMaxConcurrent: 5,
			wantProjectCount:  1,
		},
		{
			name:              "max_concurrent_tasks zero unlimited",
			limiter:           &mockLimiter{active: 0, max: 0},
			providers:         []StatusProvider{},
			wantActiveTasks:   0,
			wantMaxConcurrent: 0,
			wantProjectCount:  0,
		},
		{
			name:    "multiple projects",
			limiter: &mockLimiter{active: 1, max: 10},
			providers: []StatusProvider{
				&mockProvider{status: orchestrator.OrchestratorStatus{
					Project:      "owner/repo1",
					RunningTasks: []orchestrator.RunningTaskInfo{},
					RetryPending: []orchestrator.RetryInfo{},
				}},
				&mockProvider{status: orchestrator.OrchestratorStatus{
					Project:      "owner/repo2",
					RunningTasks: []orchestrator.RunningTaskInfo{},
					RetryPending: []orchestrator.RetryInfo{},
				}},
			},
			wantActiveTasks:   1,
			wantMaxConcurrent: 10,
			wantProjectCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(tt.limiter, tt.providers)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/status", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %s, want application/json", got)
			}

			var resp statusResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.ActiveTasks != tt.wantActiveTasks {
				t.Errorf("active_tasks = %d, want %d", resp.ActiveTasks, tt.wantActiveTasks)
			}
			if resp.MaxConcurrentTasks != tt.wantMaxConcurrent {
				t.Errorf("max_concurrent_tasks = %d, want %d", resp.MaxConcurrentTasks, tt.wantMaxConcurrent)
			}
			if len(resp.Projects) != tt.wantProjectCount {
				t.Errorf("len(projects) = %d, want %d", len(resp.Projects), tt.wantProjectCount)
			}
		})
	}
}

func TestHandler_RunningAndRetryFields(t *testing.T) {
	now := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	retryAfter := time.Date(2026, 3, 27, 10, 5, 0, 0, time.UTC)

	provider := &mockProvider{status: orchestrator.OrchestratorStatus{
		Project: "owner/repo",
		RunningTasks: []orchestrator.RunningTaskInfo{
			{TaskID: "abc123", StartedAt: now},
		},
		RetryPending: []orchestrator.RetryInfo{
			{TaskID: "def456", Phase: "CODE", Attempt: 2, RetryAfter: retryAfter},
		},
	}}

	h := NewHandler(&mockLimiter{active: 1, max: 5}, []StatusProvider{provider})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp statusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(resp.Projects))
	}

	proj := resp.Projects[0]
	if proj.Project != "owner/repo" {
		t.Errorf("project = %q, want owner/repo", proj.Project)
	}

	if len(proj.RunningTasks) != 1 {
		t.Fatalf("expected 1 running task, got %d", len(proj.RunningTasks))
	}
	rt := proj.RunningTasks[0]
	if rt.TaskID != "abc123" {
		t.Errorf("running task_id = %q, want abc123", rt.TaskID)
	}
	if !rt.StartedAt.Equal(now) {
		t.Errorf("started_at = %v, want %v", rt.StartedAt, now)
	}

	if len(proj.RetryPending) != 1 {
		t.Fatalf("expected 1 retry pending, got %d", len(proj.RetryPending))
	}
	rp := proj.RetryPending[0]
	if rp.TaskID != "def456" {
		t.Errorf("retry task_id = %q, want def456", rp.TaskID)
	}
	if rp.Phase != "CODE" {
		t.Errorf("retry phase = %q, want CODE", rp.Phase)
	}
	if rp.Attempt != 2 {
		t.Errorf("retry attempt = %d, want 2", rp.Attempt)
	}
	if !rp.RetryAfter.Equal(retryAfter) {
		t.Errorf("retry_after = %v, want %v", rp.RetryAfter, retryAfter)
	}
}
