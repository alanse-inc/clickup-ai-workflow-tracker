package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultPollIntervalMS     = 10000
	DefaultMaxConcurrentTasks = 0 // 0 = unlimited
	DefaultShutdownTimeoutMS  = 30000
)

type Config struct {
	ClickUpAPIToken         string
	GitHubPAT               string
	AuthMode                string // "pat" or "app"
	GitHubAppID             int64
	GitHubAppInstallationID int64
	GitHubAppPrivateKey     string
	Projects                []ProjectConfig
	SkippedProjectErrors    []error
}

func Load() (*Config, error) {
	cfg := &Config{}

	cfg.ClickUpAPIToken = os.Getenv("CLICKUP_API_TOKEN")
	if cfg.ClickUpAPIToken == "" {
		return nil, fmt.Errorf("missing required environment variable: CLICKUP_API_TOKEN")
	}

	// GitHub 認証: PAT と App は排他
	if err := loadGitHubAuth(cfg); err != nil {
		return nil, err
	}

	// プロジェクト設定の読み込み
	projectsFilePath := os.Getenv("PROJECTS_FILE")
	if projectsFilePath == "" {
		projectsFilePath = "projects.yaml"
	}
	projects, skipped, err := loadProjects(projectsFilePath)
	if err != nil {
		return nil, err
	}
	cfg.Projects = projects
	cfg.SkippedProjectErrors = skipped

	return cfg, nil
}

func loadGitHubAuth(cfg *Config) error {
	pat := os.Getenv("GITHUB_PAT")
	appID := os.Getenv("GITHUB_APP_ID")
	installID := os.Getenv("GITHUB_APP_INSTALLATION_ID")
	privateKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")

	hasPAT := pat != ""
	appFields := []string{appID, installID, privateKey}
	appFieldCount := 0
	for _, f := range appFields {
		if f != "" {
			appFieldCount++
		}
	}
	hasApp := appFieldCount > 0

	if hasPAT && hasApp {
		return fmt.Errorf("GITHUB_PAT and GITHUB_APP_* variables are mutually exclusive")
	}

	if !hasPAT && !hasApp {
		return fmt.Errorf("either GITHUB_PAT or all GITHUB_APP_* variables (GITHUB_APP_ID, GITHUB_APP_INSTALLATION_ID, GITHUB_APP_PRIVATE_KEY) must be set")
	}

	if hasPAT {
		cfg.AuthMode = "pat"
		cfg.GitHubPAT = pat
		return nil
	}

	// App モード: 全フィールド必須
	if appFieldCount < 3 {
		var missing []string
		if appID == "" {
			missing = append(missing, "GITHUB_APP_ID")
		}
		if installID == "" {
			missing = append(missing, "GITHUB_APP_INSTALLATION_ID")
		}
		if privateKey == "" {
			missing = append(missing, "GITHUB_APP_PRIVATE_KEY")
		}
		return fmt.Errorf("missing GitHub App environment variables: %s", strings.Join(missing, ", "))
	}

	parsedAppID, err := strconv.ParseInt(appID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid GITHUB_APP_ID value %q: %w", appID, err)
	}
	parsedInstallID, err := strconv.ParseInt(installID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid GITHUB_APP_INSTALLATION_ID value %q: %w", installID, err)
	}

	// 改行・空白を除去してからデコード（macOS base64 のデフォルト折り返しやコピペ時の余分な空白に対応）
	normalized := strings.NewReplacer("\n", "", "\r", "", " ", "", "\t", "").Replace(privateKey)
	decodedKey, err := base64.StdEncoding.DecodeString(normalized)
	if err != nil {
		return fmt.Errorf("invalid GITHUB_APP_PRIVATE_KEY: not valid base64: %w", err)
	}

	cfg.AuthMode = "app"
	cfg.GitHubAppID = parsedAppID
	cfg.GitHubAppInstallationID = parsedInstallID
	cfg.GitHubAppPrivateKey = string(decodedKey)
	return nil
}
