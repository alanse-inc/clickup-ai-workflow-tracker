package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/clickup"
	"gopkg.in/yaml.v3"
)

const defaultWorkflowFile = "agent.yaml"

// ProjectConfig は1つの ClickUp リスト - GitHub リポジトリペアの設定
type ProjectConfig struct {
	ClickUpListID      string
	GitHubOwner        string
	GitHubRepo         string
	GitHubWorkflowFile string
	StatusMapping      clickup.StatusMapping
}

// rawStatusMappingConfig は YAML パース用の内部構造体。
// clickup.StatusMapping に yaml タグを直接付与すると clickup パッケージが
// yaml 依存を持つことになるため、変換用の中間構造体として分離している。
type rawStatusMappingConfig struct {
	ReadyForSpec   string `yaml:"ready_for_spec"`
	GeneratingSpec string `yaml:"generating_spec"`
	SpecReview     string `yaml:"spec_review"`
	ReadyForCode   string `yaml:"ready_for_code"`
	Implementing   string `yaml:"implementing"`
	PRReview       string `yaml:"pr_review"`
	Closed         string `yaml:"closed"`
}

// rawProjectConfig は YAML パース用の内部構造体
type rawProjectConfig struct {
	ClickUpListID      string                  `yaml:"clickup_list_id"`
	GitHubOwner        string                  `yaml:"github_owner"`
	GitHubRepo         string                  `yaml:"github_repo"`
	GitHubWorkflowFile string                  `yaml:"github_workflow_file"`
	StatusMapping      *rawStatusMappingConfig `yaml:"status_mapping"`
}

type projectsFile struct {
	Projects []rawProjectConfig `yaml:"projects"`
}

func loadProjects(path string) ([]ProjectConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // パスは環境変数 PROJECTS_FILE またはデフォルト値で制御される
	if err != nil {
		return nil, fmt.Errorf("reading projects file: %w", err)
	}

	var pf projectsFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parsing projects file: %w", err)
	}

	if len(pf.Projects) == 0 {
		return nil, fmt.Errorf("projects file must contain at least one project")
	}

	projects := make([]ProjectConfig, len(pf.Projects))
	for i, p := range pf.Projects {
		var missing []string
		if p.ClickUpListID == "" {
			missing = append(missing, "clickup_list_id")
		}
		if p.GitHubOwner == "" {
			missing = append(missing, "github_owner")
		}
		if p.GitHubRepo == "" {
			missing = append(missing, "github_repo")
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("project[%d]: missing required fields: %s", i, strings.Join(missing, ", "))
		}

		workflowFile := p.GitHubWorkflowFile
		if workflowFile == "" {
			workflowFile = defaultWorkflowFile
		}

		sm, err := resolveStatusMapping(p.StatusMapping)
		if err != nil {
			return nil, fmt.Errorf("project[%d] (%s/%s): invalid status_mapping: %w", i, p.GitHubOwner, p.GitHubRepo, err)
		}

		projects[i] = ProjectConfig{
			ClickUpListID:      p.ClickUpListID,
			GitHubOwner:        p.GitHubOwner,
			GitHubRepo:         p.GitHubRepo,
			GitHubWorkflowFile: workflowFile,
			StatusMapping:      sm,
		}
	}

	return projects, nil
}

// resolveStatusMapping は rawStatusMappingConfig からデフォルト値を補完した StatusMapping を返す
func resolveStatusMapping(raw *rawStatusMappingConfig) (clickup.StatusMapping, error) {
	sm := clickup.DefaultStatusMapping()

	if raw != nil {
		if raw.ReadyForSpec != "" {
			sm.ReadyForSpec = strings.ToLower(strings.TrimSpace(raw.ReadyForSpec))
		}
		if raw.GeneratingSpec != "" {
			sm.GeneratingSpec = strings.ToLower(strings.TrimSpace(raw.GeneratingSpec))
		}
		if raw.SpecReview != "" {
			sm.SpecReview = strings.ToLower(strings.TrimSpace(raw.SpecReview))
		}
		if raw.ReadyForCode != "" {
			sm.ReadyForCode = strings.ToLower(strings.TrimSpace(raw.ReadyForCode))
		}
		if raw.Implementing != "" {
			sm.Implementing = strings.ToLower(strings.TrimSpace(raw.Implementing))
		}
		if raw.PRReview != "" {
			sm.PRReview = strings.ToLower(strings.TrimSpace(raw.PRReview))
		}
		if raw.Closed != "" {
			sm.Closed = strings.ToLower(strings.TrimSpace(raw.Closed))
		}
	}

	if err := validateStatusMapping(sm); err != nil {
		return clickup.StatusMapping{}, err
	}

	return sm, nil
}

// validateStatusMapping はステータスマッピングの空文字チェックと重複チェックを行う。
// fields スライスは AllStatuses() および rawStatusMappingConfig と同期を保つこと。
func validateStatusMapping(sm clickup.StatusMapping) error {
	type field struct {
		name  string
		value string
	}
	fields := []field{
		{"ReadyForSpec", sm.ReadyForSpec},
		{"GeneratingSpec", sm.GeneratingSpec},
		{"SpecReview", sm.SpecReview},
		{"ReadyForCode", sm.ReadyForCode},
		{"Implementing", sm.Implementing},
		{"PRReview", sm.PRReview},
		{"Closed", sm.Closed},
	}

	for _, f := range fields {
		if f.value == "" {
			return fmt.Errorf("status mapping %s must not be empty", f.name)
		}
	}

	seen := make(map[string]string, len(fields))
	for _, f := range fields {
		if prev, ok := seen[f.value]; ok {
			return fmt.Errorf("duplicate status %q in mapping fields %s and %s", f.value, prev, f.name)
		}
		seen[f.value] = f.name
	}

	return nil
}
