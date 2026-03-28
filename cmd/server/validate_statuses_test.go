package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/clickup"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/config"
)

func TestValidateStatuses(t *testing.T) {
	sm := clickup.DefaultStatusMapping()
	allStatuses := sm.AllStatuses()

	tests := []struct {
		name        string
		statuses    []string
		apiErr      bool
		wantErr     bool
		errContains string
	}{
		{
			name:     "all_statuses_exist",
			statuses: allStatuses,
			wantErr:  false,
		},
		{
			name:        "some_statuses_missing",
			statuses:    allStatuses[:4],
			wantErr:     true,
			errContains: "statuses not found on ClickUp board",
		},
		{
			name:        "api_error",
			apiErr:      true,
			wantErr:     true,
			errContains: "fetching ClickUp statuses",
		},
		{
			name:        "empty_status_list",
			statuses:    []string{},
			wantErr:     true,
			errContains: "statuses not found on ClickUp board",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.apiErr {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				statuses := make([]map[string]string, len(tt.statuses))
				for i, s := range tt.statuses {
					statuses[i] = map[string]string{"status": s}
				}
				resp := map[string]any{"statuses": statuses}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			client := clickup.NewClientWithBaseURL("test-token", "list123", server.URL+"/api/v2")
			cfg := &config.Config{StatusMapping: sm}

			err := validateStatuses(client, cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
