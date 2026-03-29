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
		name             string
		limiters         []LimiterStatus
		providers        []StatusProvider
		wantProjectCount int
		checkProjects    func(t *testing.T, projects []projectStatusJSON)
	}{
		{
			name:             "no projects",
			limiters:         []LimiterStatus{},
			providers:        []StatusProvider{},
			wantProjectCount: 0,
		},
		{
			name:     "running and retry tasks",
			limiters: []LimiterStatus{&mockLimiter{active: 2, max: 5}},
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
			wantProjectCount: 1,
			checkProjects: func(t *testing.T, projects []projectStatusJSON) {
				t.Helper()
				p := projects[0]
				if p.Project != "owner/repo" {
					t.Errorf("project = %q, want owner/repo", p.Project)
				}
				if p.ActiveTasks != 2 {
					t.Errorf("active_tasks = %d, want 2", p.ActiveTasks)
				}
				if p.MaxConcurrentTasks != 5 {
					t.Errorf("max_concurrent_tasks = %d, want 5", p.MaxConcurrentTasks)
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
			name:     "max_concurrent_tasks 0 (unlimited)",
			limiters: []LimiterStatus{&mockLimiter{active: 0, max: 0}},
			providers: []StatusProvider{
				&mockStatusProvider{
					status: orchestrator.OrchestratorStatus{
						Project: "owner/repo",
					},
				},
			},
			wantProjectCount: 1,
			checkProjects: func(t *testing.T, projects []projectStatusJSON) {
				t.Helper()
				if projects[0].ActiveTasks != 0 {
					t.Errorf("active_tasks = %d, want 0", projects[0].ActiveTasks)
				}
				if projects[0].MaxConcurrentTasks != 0 {
					t.Errorf("max_concurrent_tasks = %d, want 0", projects[0].MaxConcurrentTasks)
				}
			},
		},
		{
			name: "multiple projects with different limits",
			limiters: []LimiterStatus{
				&mockLimiter{active: 1, max: 3},
				&mockLimiter{active: 0, max: 5},
			},
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
			wantProjectCount: 2,
			checkProjects: func(t *testing.T, projects []projectStatusJSON) {
				t.Helper()
				for _, p := range projects {
					switch p.Project {
					case "owner/repo-a":
						if p.ActiveTasks != 1 {
							t.Errorf("repo-a active_tasks = %d, want 1", p.ActiveTasks)
						}
						if p.MaxConcurrentTasks != 3 {
							t.Errorf("repo-a max_concurrent_tasks = %d, want 3", p.MaxConcurrentTasks)
						}
					case "owner/repo-b":
						if p.ActiveTasks != 0 {
							t.Errorf("repo-b active_tasks = %d, want 0", p.ActiveTasks)
						}
						if p.MaxConcurrentTasks != 5 {
							t.Errorf("repo-b max_concurrent_tasks = %d, want 5", p.MaxConcurrentTasks)
						}
					default:
						t.Errorf("unexpected project %q", p.Project)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(tt.limiters, tt.providers)

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

			if len(resp.Projects) != tt.wantProjectCount {
				t.Errorf("projects len = %d, want %d", len(resp.Projects), tt.wantProjectCount)
			}

			if tt.checkProjects != nil {
				tt.checkProjects(t, resp.Projects)
			}
		})
	}
}
