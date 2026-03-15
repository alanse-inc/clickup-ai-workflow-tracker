package config

import (
	"strings"
	"testing"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/clickup"
)

func setRequiredEnvs(t *testing.T) {
	t.Helper()
	t.Setenv("CLICKUP_API_TOKEN", "test-token")
	t.Setenv("CLICKUP_LIST_ID", "list-123")
	t.Setenv("GITHUB_PAT", "ghp_test")
	t.Setenv("GITHUB_OWNER", "test-owner")
	t.Setenv("GITHUB_REPO", "test-repo")
}

func clearEnvs(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CLICKUP_API_TOKEN", "CLICKUP_LIST_ID",
		"GITHUB_PAT", "GITHUB_OWNER", "GITHUB_REPO",
		"GITHUB_WORKFLOW_FILE", "POLL_INTERVAL_MS",
		"CLICKUP_STATUS_READY_FOR_SPEC", "CLICKUP_STATUS_GENERATING_SPEC",
		"CLICKUP_STATUS_SPEC_REVIEW", "CLICKUP_STATUS_READY_FOR_CODE",
		"CLICKUP_STATUS_IMPLEMENTING", "CLICKUP_STATUS_PR_REVIEW",
		"CLICKUP_STATUS_CLOSED",
		"GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY",
	} {
		t.Setenv(key, "")
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T)
		wantErr     bool
		errContains string
		check       func(t *testing.T, cfg *Config)
	}{
		{
			name: "all required fields set with PAT",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.ClickUpAPIToken != "test-token" {
					t.Errorf("ClickUpAPIToken = %q, want %q", cfg.ClickUpAPIToken, "test-token")
				}
				if cfg.ClickUpListID != "list-123" {
					t.Errorf("ClickUpListID = %q, want %q", cfg.ClickUpListID, "list-123")
				}
				if cfg.GitHubPAT != "ghp_test" {
					t.Errorf("GitHubPAT = %q, want %q", cfg.GitHubPAT, "ghp_test")
				}
				if cfg.GitHubOwner != "test-owner" {
					t.Errorf("GitHubOwner = %q, want %q", cfg.GitHubOwner, "test-owner")
				}
				if cfg.GitHubRepo != "test-repo" {
					t.Errorf("GitHubRepo = %q, want %q", cfg.GitHubRepo, "test-repo")
				}
				if cfg.AuthMode != "pat" {
					t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, "pat")
				}
			},
		},
		{
			name: "default values",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.GitHubWorkflowFile != "agent.yml" {
					t.Errorf("GitHubWorkflowFile = %q, want %q", cfg.GitHubWorkflowFile, "agent.yml")
				}
				if cfg.PollIntervalMS != 10000 {
					t.Errorf("PollIntervalMS = %d, want %d", cfg.PollIntervalMS, 10000)
				}
			},
		},
		{
			name: "custom optional values",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("GITHUB_WORKFLOW_FILE", "custom.yml")
				t.Setenv("POLL_INTERVAL_MS", "5000")
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.GitHubWorkflowFile != "custom.yml" {
					t.Errorf("GitHubWorkflowFile = %q, want %q", cfg.GitHubWorkflowFile, "custom.yml")
				}
				if cfg.PollIntervalMS != 5000 {
					t.Errorf("PollIntervalMS = %d, want %d", cfg.PollIntervalMS, 5000)
				}
			},
		},
		{
			name: "missing CLICKUP_API_TOKEN",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_LIST_ID", "list-123")
				t.Setenv("GITHUB_PAT", "ghp_test")
				t.Setenv("GITHUB_OWNER", "test-owner")
				t.Setenv("GITHUB_REPO", "test-repo")
			},
			wantErr:     true,
			errContains: "CLICKUP_API_TOKEN",
		},
		{
			name: "missing multiple required fields without PAT",
			setup: func(t *testing.T) {
				clearEnvs(t)
			},
			wantErr:     true,
			errContains: "missing required environment variables",
		},
		{
			name: "missing multiple required fields",
			setup: func(t *testing.T) {
				clearEnvs(t)
			},
			wantErr:     true,
			errContains: "missing required environment variables",
		},
		{
			name: "invalid POLL_INTERVAL_MS",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("POLL_INTERVAL_MS", "not-a-number")
			},
			wantErr:     true,
			errContains: "invalid POLL_INTERVAL_MS",
		},
		{
			name: "zero POLL_INTERVAL_MS",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("POLL_INTERVAL_MS", "0")
			},
			wantErr:     true,
			errContains: "POLL_INTERVAL_MS must be positive",
		},
		{
			name: "negative POLL_INTERVAL_MS",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("POLL_INTERVAL_MS", "-100")
			},
			wantErr:     true,
			errContains: "POLL_INTERVAL_MS must be positive",
		},
		{
			name: "default status mapping",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
			},
			check: func(t *testing.T, cfg *Config) {
				want := clickup.DefaultStatusMapping()
				if cfg.StatusMapping != want {
					t.Errorf("StatusMapping = %+v, want %+v", cfg.StatusMapping, want)
				}
			},
		},
		{
			name: "custom status mapping",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("CLICKUP_STATUS_READY_FOR_SPEC", "custom ready")
				t.Setenv("CLICKUP_STATUS_CLOSED", "done")
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.StatusMapping.ReadyForSpec != "custom ready" {
					t.Errorf("ReadyForSpec = %q, want %q", cfg.StatusMapping.ReadyForSpec, "custom ready")
				}
				if cfg.StatusMapping.Closed != "done" {
					t.Errorf("Closed = %q, want %q", cfg.StatusMapping.Closed, "done")
				}
				// unchanged fields should keep defaults
				if cfg.StatusMapping.GeneratingSpec != "generating spec" {
					t.Errorf("GeneratingSpec = %q, want %q", cfg.StatusMapping.GeneratingSpec, "generating spec")
				}
			},
		},
		{
			name: "status mapping normalizes case and whitespace",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("CLICKUP_STATUS_READY_FOR_SPEC", "  Ready For Spec  ")
				t.Setenv("CLICKUP_STATUS_CLOSED", "DONE")
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.StatusMapping.ReadyForSpec != "ready for spec" {
					t.Errorf("ReadyForSpec = %q, want %q", cfg.StatusMapping.ReadyForSpec, "ready for spec")
				}
				if cfg.StatusMapping.Closed != "done" {
					t.Errorf("Closed = %q, want %q", cfg.StatusMapping.Closed, "done")
				}
			},
		},
		{
			name: "duplicate status mapping values",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("CLICKUP_STATUS_READY_FOR_SPEC", "implementing")
			},
			wantErr:     true,
			errContains: "duplicate status",
		},
		{
			name: "GitHub App auth mode",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("CLICKUP_LIST_ID", "list-123")
				t.Setenv("GITHUB_OWNER", "test-owner")
				t.Setenv("GITHUB_REPO", "test-repo")
				t.Setenv("GITHUB_APP_ID", "12345")
				t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
				t.Setenv("GITHUB_APP_PRIVATE_KEY", "test-private-key")
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.AuthMode != "app" {
					t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, "app")
				}
				if cfg.GitHubAppID != 12345 {
					t.Errorf("GitHubAppID = %d, want %d", cfg.GitHubAppID, 12345)
				}
				if cfg.GitHubAppInstallationID != 67890 {
					t.Errorf("GitHubAppInstallationID = %d, want %d", cfg.GitHubAppInstallationID, 67890)
				}
				if cfg.GitHubAppPrivateKey != "test-private-key" {
					t.Errorf("GitHubAppPrivateKey = %q, want %q", cfg.GitHubAppPrivateKey, "test-private-key")
				}
			},
		},
		{
			name: "PAT and App mutually exclusive",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("GITHUB_APP_ID", "12345")
				t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
				t.Setenv("GITHUB_APP_PRIVATE_KEY", "test-key")
			},
			wantErr:     true,
			errContains: "mutually exclusive",
		},
		{
			name: "neither PAT nor App set",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("CLICKUP_LIST_ID", "list-123")
				t.Setenv("GITHUB_OWNER", "test-owner")
				t.Setenv("GITHUB_REPO", "test-repo")
			},
			wantErr:     true,
			errContains: "either GITHUB_PAT or all GITHUB_APP_*",
		},
		{
			name: "partial App config",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("CLICKUP_LIST_ID", "list-123")
				t.Setenv("GITHUB_OWNER", "test-owner")
				t.Setenv("GITHUB_REPO", "test-repo")
				t.Setenv("GITHUB_APP_ID", "12345")
			},
			wantErr:     true,
			errContains: "missing GitHub App environment variables",
		},
		{
			name: "invalid GITHUB_APP_ID",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("CLICKUP_LIST_ID", "list-123")
				t.Setenv("GITHUB_OWNER", "test-owner")
				t.Setenv("GITHUB_REPO", "test-repo")
				t.Setenv("GITHUB_APP_ID", "not-a-number")
				t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
				t.Setenv("GITHUB_APP_PRIVATE_KEY", "test-key")
			},
			wantErr:     true,
			errContains: "invalid GITHUB_APP_ID",
		},
		{
			name: "MAX_CONCURRENT_TASKS default is 0",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.MaxConcurrentTasks != 0 {
					t.Errorf("MaxConcurrentTasks = %d, want 0", cfg.MaxConcurrentTasks)
				}
			},
		},
		{
			name: "MAX_CONCURRENT_TASKS set to positive value",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("MAX_CONCURRENT_TASKS", "5")
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.MaxConcurrentTasks != 5 {
					t.Errorf("MaxConcurrentTasks = %d, want 5", cfg.MaxConcurrentTasks)
				}
			},
		},
		{
			name: "MAX_CONCURRENT_TASKS invalid value",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("MAX_CONCURRENT_TASKS", "not-a-number")
			},
			wantErr:     true,
			errContains: "invalid MAX_CONCURRENT_TASKS",
		},
		{
			name: "MAX_CONCURRENT_TASKS negative value",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("MAX_CONCURRENT_TASKS", "-1")
			},
			wantErr:     true,
			errContains: "MAX_CONCURRENT_TASKS must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			cfg, err := Load()

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
			if cfg == nil {
				t.Fatal("expected non-nil config")
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}
