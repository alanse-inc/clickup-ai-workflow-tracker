package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/clickup"
)

// writeProjectsFile はテスト用の projects.yaml を作成し、PROJECTS_FILE 環境変数を設定する
func writeProjectsFile(t *testing.T, yaml string) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "projects.yaml")
	if err := os.WriteFile(tmpFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROJECTS_FILE", tmpFile)
}

const defaultProjectsYAML = `projects:
  - clickup_list_id: "list-123"
    github_owner: "test-owner"
    github_repo: "test-repo"
`

func setRequiredEnvs(t *testing.T) {
	t.Helper()
	t.Setenv("CLICKUP_API_TOKEN", "test-token")
	t.Setenv("GITHUB_PAT", "ghp_test")
	writeProjectsFile(t, defaultProjectsYAML)
}

func clearEnvs(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CLICKUP_API_TOKEN",
		"GITHUB_PAT", "POLL_INTERVAL_MS", "SHUTDOWN_TIMEOUT_MS", "MAX_CONCURRENT_TASKS",
		"GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY",
		"PROJECTS_FILE",
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
				if len(cfg.Projects) != 1 {
					t.Fatalf("Projects len = %d, want 1", len(cfg.Projects))
				}
				p := cfg.Projects[0]
				if p.ClickUpListID != "list-123" {
					t.Errorf("ClickUpListID = %q, want %q", p.ClickUpListID, "list-123")
				}
				if p.GitHubOwner != "test-owner" {
					t.Errorf("GitHubOwner = %q, want %q", p.GitHubOwner, "test-owner")
				}
				if p.GitHubRepo != "test-repo" {
					t.Errorf("GitHubRepo = %q, want %q", p.GitHubRepo, "test-repo")
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
				if cfg.Projects[0].GitHubWorkflowFile != "agent.yaml" {
					t.Errorf("GitHubWorkflowFile = %q, want %q", cfg.Projects[0].GitHubWorkflowFile, "agent.yaml")
				}
				if cfg.PollIntervalMS != 10000 {
					t.Errorf("PollIntervalMS = %d, want %d", cfg.PollIntervalMS, 10000)
				}
			},
		},
		{
			name: "custom workflow file in YAML",
			setup: func(t *testing.T) {
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("GITHUB_PAT", "ghp_test")
				writeProjectsFile(t, `projects:
  - clickup_list_id: "list-123"
    github_owner: "test-owner"
    github_repo: "test-repo"
    github_workflow_file: "custom.yaml"
`)
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Projects[0].GitHubWorkflowFile != "custom.yaml" {
					t.Errorf("GitHubWorkflowFile = %q, want %q", cfg.Projects[0].GitHubWorkflowFile, "custom.yaml")
				}
			},
		},
		{
			name: "custom poll interval",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("POLL_INTERVAL_MS", "5000")
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.PollIntervalMS != 5000 {
					t.Errorf("PollIntervalMS = %d, want %d", cfg.PollIntervalMS, 5000)
				}
			},
		},
		{
			name: "missing CLICKUP_API_TOKEN",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("GITHUB_PAT", "ghp_test")
				writeProjectsFile(t, defaultProjectsYAML)
			},
			wantErr:     true,
			errContains: "CLICKUP_API_TOKEN",
		},
		{
			name: "missing projects file",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("GITHUB_PAT", "ghp_test")
				t.Setenv("PROJECTS_FILE", filepath.Join(t.TempDir(), "nonexistent.yaml"))
			},
			wantErr:     true,
			errContains: "reading projects file",
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
			name: "default status mapping applied per project",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
			},
			check: func(t *testing.T, cfg *Config) {
				want := clickup.DefaultStatusMapping()
				if cfg.Projects[0].StatusMapping != want {
					t.Errorf("StatusMapping = %+v, want %+v", cfg.Projects[0].StatusMapping, want)
				}
			},
		},
		{
			name: "GitHub App auth mode",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("GITHUB_APP_ID", "12345")
				t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
				t.Setenv("GITHUB_APP_PRIVATE_KEY", base64.StdEncoding.EncodeToString([]byte("test-private-key")))
				writeProjectsFile(t, defaultProjectsYAML)
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
				t.Setenv("GITHUB_APP_PRIVATE_KEY", base64.StdEncoding.EncodeToString([]byte("test-key")))
			},
			wantErr:     true,
			errContains: "mutually exclusive",
		},
		{
			name: "neither PAT nor App set",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				writeProjectsFile(t, defaultProjectsYAML)
			},
			wantErr:     true,
			errContains: "either GITHUB_PAT or all GITHUB_APP_*",
		},
		{
			name: "partial App config",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("GITHUB_APP_ID", "12345")
				writeProjectsFile(t, defaultProjectsYAML)
			},
			wantErr:     true,
			errContains: "missing GitHub App environment variables",
		},
		{
			name: "invalid GITHUB_APP_ID",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("GITHUB_APP_ID", "not-a-number")
				t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
				t.Setenv("GITHUB_APP_PRIVATE_KEY", base64.StdEncoding.EncodeToString([]byte("test-key")))
				writeProjectsFile(t, defaultProjectsYAML)
			},
			wantErr:     true,
			errContains: "invalid GITHUB_APP_ID",
		},
		{
			name: "GitHub App with base64 encoded private key",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("GITHUB_APP_ID", "12345")
				t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
				t.Setenv("GITHUB_APP_PRIVATE_KEY", base64.StdEncoding.EncodeToString([]byte("line1\nline2\nline3\n")))
				writeProjectsFile(t, defaultProjectsYAML)
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.GitHubAppPrivateKey != "line1\nline2\nline3\n" {
					t.Errorf("GitHubAppPrivateKey = %q, want %q", cfg.GitHubAppPrivateKey, "line1\nline2\nline3\n")
				}
			},
		},
		{
			name: "GitHub App with base64 key containing embedded newlines and spaces",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("GITHUB_APP_ID", "12345")
				t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
				encoded := base64.StdEncoding.EncodeToString([]byte("line1\nline2\nline3\n"))
				withWrapping := encoded[:10] + "\n" + encoded[10:20] + " " + encoded[20:]
				t.Setenv("GITHUB_APP_PRIVATE_KEY", withWrapping)
				writeProjectsFile(t, defaultProjectsYAML)
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.GitHubAppPrivateKey != "line1\nline2\nline3\n" {
					t.Errorf("GitHubAppPrivateKey = %q, want %q", cfg.GitHubAppPrivateKey, "line1\nline2\nline3\n")
				}
			},
		},
		{
			name: "GitHub App with invalid base64 private key",
			setup: func(t *testing.T) {
				clearEnvs(t)
				t.Setenv("CLICKUP_API_TOKEN", "test-token")
				t.Setenv("GITHUB_APP_ID", "12345")
				t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
				t.Setenv("GITHUB_APP_PRIVATE_KEY", "not-valid-base64!!!")
				writeProjectsFile(t, defaultProjectsYAML)
			},
			wantErr:     true,
			errContains: "invalid GITHUB_APP_PRIVATE_KEY",
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
		{
			name: "SHUTDOWN_TIMEOUT_MS default is 30000",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.ShutdownTimeoutMS != 30000 {
					t.Errorf("ShutdownTimeoutMS = %d, want 30000", cfg.ShutdownTimeoutMS)
				}
			},
		},
		{
			name: "SHUTDOWN_TIMEOUT_MS set to positive value",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("SHUTDOWN_TIMEOUT_MS", "5000")
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.ShutdownTimeoutMS != 5000 {
					t.Errorf("ShutdownTimeoutMS = %d, want 5000", cfg.ShutdownTimeoutMS)
				}
			},
		},
		{
			name: "SHUTDOWN_TIMEOUT_MS invalid value",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("SHUTDOWN_TIMEOUT_MS", "not-a-number")
			},
			wantErr:     true,
			errContains: "invalid SHUTDOWN_TIMEOUT_MS",
		},
		{
			name: "SHUTDOWN_TIMEOUT_MS zero value",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("SHUTDOWN_TIMEOUT_MS", "0")
			},
			wantErr:     true,
			errContains: "SHUTDOWN_TIMEOUT_MS must be positive",
		},
		{
			name: "SHUTDOWN_TIMEOUT_MS negative value",
			setup: func(t *testing.T) {
				setRequiredEnvs(t)
				t.Setenv("SHUTDOWN_TIMEOUT_MS", "-1")
			},
			wantErr:     true,
			errContains: "SHUTDOWN_TIMEOUT_MS must be positive",
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

func TestLoadProjects_FromYAML(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantErr     bool
		errContains string
		check       func(t *testing.T, projects []ProjectConfig)
	}{
		{
			name: "single project",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
`,
			check: func(t *testing.T, projects []ProjectConfig) {
				if len(projects) != 1 {
					t.Fatalf("len = %d, want 1", len(projects))
				}
				if projects[0].ClickUpListID != "list-1" {
					t.Errorf("ClickUpListID = %q, want %q", projects[0].ClickUpListID, "list-1")
				}
				if projects[0].GitHubWorkflowFile != "agent.yaml" {
					t.Errorf("GitHubWorkflowFile = %q, want %q", projects[0].GitHubWorkflowFile, "agent.yaml")
				}
				want := clickup.DefaultStatusMapping()
				if projects[0].StatusMapping != want {
					t.Errorf("StatusMapping = %+v, want %+v", projects[0].StatusMapping, want)
				}
			},
		},
		{
			name: "multiple projects",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
  - clickup_list_id: "list-2"
    github_owner: "org"
    github_repo: "repo-b"
    github_workflow_file: "custom.yaml"
`,
			check: func(t *testing.T, projects []ProjectConfig) {
				if len(projects) != 2 {
					t.Fatalf("len = %d, want 2", len(projects))
				}
				if projects[1].GitHubWorkflowFile != "custom.yaml" {
					t.Errorf("GitHubWorkflowFile = %q, want %q", projects[1].GitHubWorkflowFile, "custom.yaml")
				}
			},
		},
		{
			name: "project with custom status_mapping",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
    status_mapping:
      ready_for_spec: "todo"
      generating_spec: "in progress"
      spec_review: "spec review"
      ready_for_code: "ready for dev"
      implementing: "developing"
      pr_review: "code review"
      closed: "done"
`,
			check: func(t *testing.T, projects []ProjectConfig) {
				sm := projects[0].StatusMapping
				if sm.ReadyForSpec != "todo" {
					t.Errorf("ReadyForSpec = %q, want %q", sm.ReadyForSpec, "todo")
				}
				if sm.GeneratingSpec != "in progress" {
					t.Errorf("GeneratingSpec = %q, want %q", sm.GeneratingSpec, "in progress")
				}
				if sm.SpecReview != "spec review" {
					t.Errorf("SpecReview = %q, want %q", sm.SpecReview, "spec review")
				}
				if sm.ReadyForCode != "ready for dev" {
					t.Errorf("ReadyForCode = %q, want %q", sm.ReadyForCode, "ready for dev")
				}
				if sm.Implementing != "developing" {
					t.Errorf("Implementing = %q, want %q", sm.Implementing, "developing")
				}
				if sm.PRReview != "code review" {
					t.Errorf("PRReview = %q, want %q", sm.PRReview, "code review")
				}
				if sm.Closed != "done" {
					t.Errorf("Closed = %q, want %q", sm.Closed, "done")
				}
			},
		},
		{
			name: "project with partial status_mapping uses defaults for omitted fields",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
    status_mapping:
      ready_for_spec: "backlog"
      closed: "done"
`,
			check: func(t *testing.T, projects []ProjectConfig) {
				sm := projects[0].StatusMapping
				def := clickup.DefaultStatusMapping()
				if sm.ReadyForSpec != "backlog" {
					t.Errorf("ReadyForSpec = %q, want %q", sm.ReadyForSpec, "backlog")
				}
				if sm.Closed != "done" {
					t.Errorf("Closed = %q, want %q", sm.Closed, "done")
				}
				if sm.GeneratingSpec != def.GeneratingSpec {
					t.Errorf("GeneratingSpec = %q, want default %q", sm.GeneratingSpec, def.GeneratingSpec)
				}
				if sm.SpecReview != def.SpecReview {
					t.Errorf("SpecReview = %q, want default %q", sm.SpecReview, def.SpecReview)
				}
			},
		},
		{
			name: "status_mapping normalizes case and whitespace",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
    status_mapping:
      ready_for_spec: "  Ready For Spec  "
      closed: "DONE"
`,
			check: func(t *testing.T, projects []ProjectConfig) {
				sm := projects[0].StatusMapping
				if sm.ReadyForSpec != "ready for spec" {
					t.Errorf("ReadyForSpec = %q, want %q", sm.ReadyForSpec, "ready for spec")
				}
				if sm.Closed != "done" {
					t.Errorf("Closed = %q, want %q", sm.Closed, "done")
				}
			},
		},
		{
			name: "multiple projects with different status_mappings",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
    status_mapping:
      ready_for_spec: "backlog"
      closed: "done"
  - clickup_list_id: "list-2"
    github_owner: "org"
    github_repo: "repo-b"
`,
			check: func(t *testing.T, projects []ProjectConfig) {
				if len(projects) != 2 {
					t.Fatalf("len = %d, want 2", len(projects))
				}
				if projects[0].StatusMapping.ReadyForSpec != "backlog" {
					t.Errorf("project[0].ReadyForSpec = %q, want %q", projects[0].StatusMapping.ReadyForSpec, "backlog")
				}
				if projects[0].StatusMapping.Closed != "done" {
					t.Errorf("project[0].Closed = %q, want %q", projects[0].StatusMapping.Closed, "done")
				}
				def := clickup.DefaultStatusMapping()
				if projects[1].StatusMapping != def {
					t.Errorf("project[1].StatusMapping = %+v, want default %+v", projects[1].StatusMapping, def)
				}
			},
		},
		{
			name: "explicit empty status_mapping uses defaults",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
    status_mapping: {}
`,
			check: func(t *testing.T, projects []ProjectConfig) {
				want := clickup.DefaultStatusMapping()
				if projects[0].StatusMapping != want {
					t.Errorf("StatusMapping = %+v, want default %+v", projects[0].StatusMapping, want)
				}
			},
		},
		{
			name: "null status_mapping uses defaults",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
    status_mapping: ~
`,
			check: func(t *testing.T, projects []ProjectConfig) {
				want := clickup.DefaultStatusMapping()
				if projects[0].StatusMapping != want {
					t.Errorf("StatusMapping = %+v, want default %+v", projects[0].StatusMapping, want)
				}
			},
		},
		{
			name: "duplicate status values in status_mapping on only project",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
    status_mapping:
      ready_for_spec: "implementing"
`,
			wantErr:     true,
			errContains: "no valid projects",
		},
		{
			name:        "empty projects",
			yaml:        `projects: []`,
			wantErr:     true,
			errContains: "at least one project",
		},
		{
			name: "missing required field on only project",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
`,
			wantErr:     true,
			errContains: "no valid projects",
		},
		{
			name:        "invalid yaml",
			yaml:        `{invalid`,
			wantErr:     true,
			errContains: "parsing projects file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "projects.yaml")
			if err := os.WriteFile(tmpFile, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}

			projects, _, err := loadProjects(tmpFile)
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
			if tt.check != nil {
				tt.check(t, projects)
			}
		})
	}
}
