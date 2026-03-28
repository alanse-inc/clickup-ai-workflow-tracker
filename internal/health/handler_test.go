package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockPinger struct{ err error }

func (m *mockPinger) Ping(_ context.Context) error { return m.err }

func TestHandler_ServeHTTP(t *testing.T) {
	pingErr := errors.New("connection refused")

	tests := []struct {
		name         string
		projects     []ProjectPingers
		wantStatus   int
		wantOverall  string
		wantProjects map[string]map[string]bool // project -> service -> isOK
	}{
		{
			name: "single project all healthy",
			projects: []ProjectPingers{
				{Name: "owner/repo-a", ClickUp: &mockPinger{nil}, GitHub: &mockPinger{nil}},
			},
			wantStatus:  http.StatusOK,
			wantOverall: "ok",
			wantProjects: map[string]map[string]bool{
				"owner/repo-a": {"clickup": true, "github": true},
			},
		},
		{
			name: "single project clickup down",
			projects: []ProjectPingers{
				{Name: "owner/repo-a", ClickUp: &mockPinger{pingErr}, GitHub: &mockPinger{nil}},
			},
			wantStatus:  http.StatusServiceUnavailable,
			wantOverall: "degraded",
			wantProjects: map[string]map[string]bool{
				"owner/repo-a": {"clickup": false, "github": true},
			},
		},
		{
			name: "single project github down",
			projects: []ProjectPingers{
				{Name: "owner/repo-a", ClickUp: &mockPinger{nil}, GitHub: &mockPinger{pingErr}},
			},
			wantStatus:  http.StatusServiceUnavailable,
			wantOverall: "degraded",
			wantProjects: map[string]map[string]bool{
				"owner/repo-a": {"clickup": true, "github": false},
			},
		},
		{
			name: "multi project all healthy",
			projects: []ProjectPingers{
				{Name: "owner/repo-a", ClickUp: &mockPinger{nil}, GitHub: &mockPinger{nil}},
				{Name: "owner/repo-b", ClickUp: &mockPinger{nil}, GitHub: &mockPinger{nil}},
			},
			wantStatus:  http.StatusOK,
			wantOverall: "ok",
			wantProjects: map[string]map[string]bool{
				"owner/repo-a": {"clickup": true, "github": true},
				"owner/repo-b": {"clickup": true, "github": true},
			},
		},
		{
			name: "multi project one project degraded",
			projects: []ProjectPingers{
				{Name: "owner/repo-a", ClickUp: &mockPinger{nil}, GitHub: &mockPinger{nil}},
				{Name: "owner/repo-b", ClickUp: &mockPinger{pingErr}, GitHub: &mockPinger{nil}},
			},
			wantStatus:  http.StatusServiceUnavailable,
			wantOverall: "degraded",
			wantProjects: map[string]map[string]bool{
				"owner/repo-a": {"clickup": true, "github": true},
				"owner/repo-b": {"clickup": false, "github": true},
			},
		},
		{
			name: "multi project all down",
			projects: []ProjectPingers{
				{Name: "owner/repo-a", ClickUp: &mockPinger{pingErr}, GitHub: &mockPinger{pingErr}},
				{Name: "owner/repo-b", ClickUp: &mockPinger{pingErr}, GitHub: &mockPinger{pingErr}},
			},
			wantStatus:  http.StatusServiceUnavailable,
			wantOverall: "degraded",
			wantProjects: map[string]map[string]bool{
				"owner/repo-a": {"clickup": false, "github": false},
				"owner/repo-b": {"clickup": false, "github": false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(tt.projects)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %s, want application/json", got)
			}

			var resp healthResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Status != tt.wantOverall {
				t.Errorf("overall status = %q, want %q", resp.Status, tt.wantOverall)
			}

			for projName, wantServices := range tt.wantProjects {
				ps, ok := resp.Projects[projName]
				if !ok {
					t.Errorf("project %q not found in response", projName)
					continue
				}
				for svcName, wantOK := range wantServices {
					svc, ok := ps.Services[svcName]
					if !ok {
						t.Errorf("project %q: service %q not found", projName, svcName)
						continue
					}
					if wantOK {
						if svc.Status != "ok" {
							t.Errorf("project %q: %s.status = %q, want ok", projName, svcName, svc.Status)
						}
					} else {
						if svc.Status != "error" {
							t.Errorf("project %q: %s.status = %q, want error", projName, svcName, svc.Status)
						}
						if svc.Message == "" {
							t.Errorf("project %q: %s.message should not be empty on error", projName, svcName)
						}
					}
				}
			}
		})
	}
}
