package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadProjects_SpecOutput(t *testing.T) {
	tests := []struct {
		name           string
		yaml           string
		wantSpecOutput string
		wantErr        bool
		errContains    string
	}{
		{
			name: "spec_output omitted defaults to clickup",
			yaml: `projects:
  - clickup_list_id: "list-123"
    github_owner: "owner"
    github_repo: "repo"
`,
			wantSpecOutput: "clickup",
		},
		{
			name: "spec_output clickup explicit",
			yaml: `projects:
  - clickup_list_id: "list-123"
    github_owner: "owner"
    github_repo: "repo"
    spec_output: "clickup"
`,
			wantSpecOutput: "clickup",
		},
		{
			name: "spec_output repo",
			yaml: `projects:
  - clickup_list_id: "list-123"
    github_owner: "owner"
    github_repo: "repo"
    spec_output: "repo"
`,
			wantSpecOutput: "repo",
		},
		{
			name: "spec_output invalid value on only project",
			yaml: `projects:
  - clickup_list_id: "list-123"
    github_owner: "owner"
    github_repo: "repo"
    spec_output: "invalid"
`,
			wantErr:     true,
			errContains: "no valid projects",
		},
		{
			name: "spec_output uppercase Repo is normalized",
			yaml: `projects:
  - clickup_list_id: "list-123"
    github_owner: "owner"
    github_repo: "repo"
    spec_output: "Repo"
`,
			wantSpecOutput: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := t.TempDir() + "/projects.yaml"
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

			if len(projects) != 1 {
				t.Fatalf("expected 1 project, got %d", len(projects))
			}
			if projects[0].SpecOutput != tt.wantSpecOutput {
				t.Errorf("SpecOutput = %q, want %q", projects[0].SpecOutput, tt.wantSpecOutput)
			}
		})
	}
}

func TestLoadProjects_PartialSkip(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantErr     bool
		errContains string
		wantSkipped int
		wantListIDs []string
	}{
		{
			name: "invalid second project is skipped, first project is returned",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
  - clickup_list_id: "list-2"
    github_owner: "org"
    github_repo: ""
`,
			wantSkipped: 1,
			wantListIDs: []string{"list-1"},
		},
		{
			name: "invalid first project is skipped, second project is returned",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: ""
  - clickup_list_id: "list-2"
    github_owner: "org"
    github_repo: "repo-b"
`,
			wantSkipped: 1,
			wantListIDs: []string{"list-2"},
		},
		{
			name: "invalid spec_output project is skipped, valid project is returned",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
    spec_output: "invalid"
  - clickup_list_id: "list-2"
    github_owner: "org"
    github_repo: "repo-b"
`,
			wantSkipped: 1,
			wantListIDs: []string{"list-2"},
		},
		{
			name: "project with duplicate status_mapping is skipped, valid project remains",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
    status_mapping:
      ready_for_spec: "implementing"
  - clickup_list_id: "list-2"
    github_owner: "org"
    github_repo: "repo-b"
`,
			wantSkipped: 1,
			wantListIDs: []string{"list-2"},
		},
		{
			name: "all projects invalid returns error",
			yaml: `projects:
  - github_owner: "org"
    github_repo: "repo-a"
  - github_owner: "org"
    github_repo: "repo-b"
`,
			wantErr:     true,
			errContains: "no valid projects",
		},
		{
			name: "middle project invalid is skipped, first and third are returned",
			yaml: `projects:
  - clickup_list_id: "list-1"
    github_owner: "org"
    github_repo: "repo-a"
  - clickup_list_id: "list-2"
    github_owner: "org"
    github_repo: ""
  - clickup_list_id: "list-3"
    github_owner: "org"
    github_repo: "repo-c"
`,
			wantSkipped: 1,
			wantListIDs: []string{"list-1", "list-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := t.TempDir() + "/projects.yaml"
			if err := os.WriteFile(tmpFile, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}

			projects, skipped, err := loadProjects(tmpFile)

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

			if len(projects) != len(tt.wantListIDs) {
				t.Fatalf("len(projects) = %d, want %d", len(projects), len(tt.wantListIDs))
			}

			if len(skipped) != tt.wantSkipped {
				t.Errorf("len(skipped) = %d, want %d", len(skipped), tt.wantSkipped)
			}

			for i, wantID := range tt.wantListIDs {
				if projects[i].ClickUpListID != wantID {
					t.Errorf("projects[%d].ClickUpListID = %q, want %q", i, projects[i].ClickUpListID, wantID)
				}
			}
		})
	}
}
