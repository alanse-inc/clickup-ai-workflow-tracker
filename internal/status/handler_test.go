package status

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/orchestrator"
)

type mockStatusProvider struct {
	status orchestrator.OrchestratorStatus
}

func (m *mockStatusProvider) Status() orchestrator.OrchestratorStatus {
	return m.status
}

type mockLimiter struct {
	active int
	max    int
}

func (m *mockLimiter) ActiveCount() int   { return m.active }
func (m *mockLimiter) MaxConcurrent() int { return m.max }

func TestHandler_ServeHTTP(t *testing.T) {
	now := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	retryAt := time.Date(2026, 3, 27, 10, 5, 0, 0, time.UTC)

	tests := []struct {
		name              string
		limiter           LimiterStatus
		providers         []StatusProvider
		wantActiveTasks   int
		wantMaxConcurrent int
		wantProjectCount  int
		checkProjects     func(t *testing.T, projects []projectStatusJSON)
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
			name:    "running and retry tasks",
			limiter: &mockLimiter{active: 2, max: 5},
			providers: []StatusProvider{
				&mockStatusProvider{
					status: orchestrator.OrchestratorStatus{
						Project: "owner/repo",
						RunningTasks: []orchestrator.RunningTaskInfo{
							{TaskID: "abc123", StartedAt: now},
						},
						RetryPending: []orchestrator.RetryInfo{
							{TaskID: "def456", Phase: "CODE", Attempt: 2, RetryAfter: retryAt},
						},
					},
				},
			},
			wantActiveTasks:   2,
			wantMaxConcurrent: 5,
			wantProjectCount:  1,
			checkProjects: func(t *testing.T, projects []projectStatusJSON) {
				t.Helper()
				p := projects[0]
				if p.Project != "owner/repo" {
					t.Errorf("project = %q, want owner/repo", p.Project)
				}
				if len(p.RunningTasks) != 1 {
					t.Fatalf("running_tasks len = %d, want 1", len(p.RunningTasks))
				}
				if p.RunningTasks[0].TaskID != "abc123" {
					t.Errorf("running task_id = %q, want abc123", p.RunningTasks[0].TaskID)
				}
				if !p.RunningTasks[0].StartedAt.Equal(now) {
					t.Errorf("started_at = %v, want %v", p.RunningTasks[0].StartedAt, now)
				}
				if len(p.RetryPending) != 1 {
					t.Fatalf("retry_pending len = %d, want 1", len(p.RetryPending))
				}
				rp := p.RetryPending[0]
				if rp.TaskID != "def456" {
					t.Errorf("retry task_id = %q, want def456", rp.TaskID)
				}
				if rp.Phase != "CODE" {
					t.Errorf("retry phase = %q, want CODE", rp.Phase)
				}
				if rp.Attempt != 2 {
					t.Errorf("retry attempt = %d, want 2", rp.Attempt)
				}
				if !rp.RetryAfter.Equal(retryAt) {
					t.Errorf("retry_after = %v, want %v", rp.RetryAfter, retryAt)
				}
			},
		},
		{
			name:              "max_concurrent_tasks 0 (unlimited)",
			limiter:           &mockLimiter{active: 0, max: 0},
			providers:         []StatusProvider{},
			wantActiveTasks:   0,
			wantMaxConcurrent: 0,
			wantProjectCount:  0,
		},
		{
			name:    "multiple projects",
			limiter: &mockLimiter{active: 1, max: 3},
			providers: []StatusProvider{
				&mockStatusProvider{
					status: orchestrator.OrchestratorStatus{
						Project:      "owner/repo-a",
						RunningTasks: []orchestrator.RunningTaskInfo{},
						RetryPending: []orchestrator.RetryInfo{},
					},
				},
				&mockStatusProvider{
					status: orchestrator.OrchestratorStatus{
						Project:      "owner/repo-b",
						RunningTasks: []orchestrator.RunningTaskInfo{},
						RetryPending: []orchestrator.RetryInfo{},
					},
				},
			},
			wantActiveTasks:   1,
			wantMaxConcurrent: 3,
			wantProjectCount:  2,
			checkProjects: func(t *testing.T, projects []projectStatusJSON) {
				t.Helper()
				names := map[string]bool{}
				for _, p := range projects {
					names[p.Project] = true
				}
				if !names["owner/repo-a"] {
					t.Error("owner/repo-a not found in projects")
				}
				if !names["owner/repo-b"] {
					t.Error("owner/repo-b not found in projects")
				}
			},
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
				t.Errorf("projects len = %d, want %d", len(resp.Projects), tt.wantProjectCount)
			}

			if tt.checkProjects != nil {
				tt.checkProjects(t, resp.Projects)
			}
		})
	}
}
