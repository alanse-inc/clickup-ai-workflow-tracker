package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultWorkflowFile = "agent.yml"

// ProjectConfig は1つの ClickUp リスト - GitHub リポジトリペアの設定
type ProjectConfig struct {
	ClickUpListID      string `yaml:"clickup_list_id"`
	GitHubOwner        string `yaml:"github_owner"`
	GitHubRepo         string `yaml:"github_repo"`
	GitHubWorkflowFile string `yaml:"github_workflow_file"`
}

type projectsFile struct {
	Projects []ProjectConfig `yaml:"projects"`
}

// loadProjects はプロジェクト設定を読み込む。
// YAML ファイルが存在する場合はそこから、存在しない場合は環境変数からフォールバックする。
// 両方設定されている場合はエラー。
func loadProjects(projectsFilePath string) ([]ProjectConfig, error) {
	fileExists := false
	if _, err := os.Stat(projectsFilePath); err == nil { //nolint:gosec // パスは環境変数 PROJECTS_FILE またはデフォルト値で制御される
		fileExists = true
	}

	envListID := os.Getenv("CLICKUP_LIST_ID")
	envOwner := os.Getenv("GITHUB_OWNER")
	envRepo := os.Getenv("GITHUB_REPO")
	hasEnv := envListID != "" || envOwner != "" || envRepo != ""

	if fileExists && hasEnv {
		return nil, fmt.Errorf("both projects file (%s) and environment variables (CLICKUP_LIST_ID, GITHUB_OWNER, GITHUB_REPO) are set; use one or the other", projectsFilePath)
	}

	if fileExists {
		return loadProjectsFromFile(projectsFilePath)
	}

	return loadProjectsFromEnv(envListID, envOwner, envRepo)
}

func loadProjectsFromFile(path string) ([]ProjectConfig, error) {
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
			return nil, fmt.Errorf("project[%d]: missing required fields: %v", i, missing)
		}
		if p.GitHubWorkflowFile == "" {
			pf.Projects[i].GitHubWorkflowFile = defaultWorkflowFile
		}
	}

	return pf.Projects, nil
}

func loadProjectsFromEnv(listID, owner, repo string) ([]ProjectConfig, error) {
	var missing []string
	if listID == "" {
		missing = append(missing, "CLICKUP_LIST_ID")
	}
	if owner == "" {
		missing = append(missing, "GITHUB_OWNER")
	}
	if repo == "" {
		missing = append(missing, "GITHUB_REPO")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v (or provide a projects file)", missing)
	}

	workflowFile := os.Getenv("GITHUB_WORKFLOW_FILE")
	if workflowFile == "" {
		workflowFile = defaultWorkflowFile
	}

	return []ProjectConfig{
		{
			ClickUpListID:      listID,
			GitHubOwner:        owner,
			GitHubRepo:         repo,
			GitHubWorkflowFile: workflowFile,
		},
	}, nil
}
