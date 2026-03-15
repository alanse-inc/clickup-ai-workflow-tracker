package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/clickup"
)

type Config struct {
	ClickUpAPIToken         string
	ClickUpListID           string
	GitHubPAT               string
	AuthMode                string // "pat" or "app"
	GitHubAppID             int64
	GitHubAppInstallationID int64
	GitHubAppPrivateKey     string
	GitHubOwner             string
	GitHubRepo              string
	GitHubWorkflowFile      string // default: "agent.yml"
	PollIntervalMS          int    // default: 10000
	StatusMapping           clickup.StatusMapping
}

func Load() (*Config, error) {
	cfg := &Config{
		GitHubWorkflowFile: "agent.yml",
		PollIntervalMS:     10000,
	}

	required := map[string]*string{
		"CLICKUP_API_TOKEN": &cfg.ClickUpAPIToken,
		"CLICKUP_LIST_ID":   &cfg.ClickUpListID,
		"GITHUB_OWNER":      &cfg.GitHubOwner,
		"GITHUB_REPO":       &cfg.GitHubRepo,
	}

	var missing []string
	for envKey, field := range required {
		v := os.Getenv(envKey)
		if v == "" {
			missing = append(missing, envKey)
		} else {
			*field = v
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	// GitHub 認証: PAT と App は排他
	if err := loadGitHubAuth(cfg); err != nil {
		return nil, err
	}

	if v := os.Getenv("GITHUB_WORKFLOW_FILE"); v != "" {
		cfg.GitHubWorkflowFile = v
	}

	if v := os.Getenv("POLL_INTERVAL_MS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid POLL_INTERVAL_MS value %q: %w", v, err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("POLL_INTERVAL_MS must be positive, got %d", parsed)
		}
		cfg.PollIntervalMS = parsed
	}

	sm := clickup.DefaultStatusMapping()
	statusEnvs := map[string]*string{
		"CLICKUP_STATUS_READY_FOR_SPEC":  &sm.ReadyForSpec,
		"CLICKUP_STATUS_GENERATING_SPEC": &sm.GeneratingSpec,
		"CLICKUP_STATUS_SPEC_REVIEW":     &sm.SpecReview,
		"CLICKUP_STATUS_READY_FOR_CODE":  &sm.ReadyForCode,
		"CLICKUP_STATUS_IMPLEMENTING":    &sm.Implementing,
		"CLICKUP_STATUS_PR_REVIEW":       &sm.PRReview,
		"CLICKUP_STATUS_CLOSED":          &sm.Closed,
	}
	for envKey, field := range statusEnvs {
		if v := os.Getenv(envKey); v != "" {
			*field = strings.ToLower(strings.TrimSpace(v))
		}
	}
	cfg.StatusMapping = sm

	if err := validateStatusMapping(sm); err != nil {
		return nil, err
	}

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

	cfg.AuthMode = "app"
	cfg.GitHubAppID = parsedAppID
	cfg.GitHubAppInstallationID = parsedInstallID
	cfg.GitHubAppPrivateKey = privateKey
	return nil
}

func validateStatusMapping(sm clickup.StatusMapping) error {
	fields := map[string]string{
		"ReadyForSpec":   sm.ReadyForSpec,
		"GeneratingSpec": sm.GeneratingSpec,
		"SpecReview":     sm.SpecReview,
		"ReadyForCode":   sm.ReadyForCode,
		"Implementing":   sm.Implementing,
		"PRReview":       sm.PRReview,
		"Closed":         sm.Closed,
	}

	for name, val := range fields {
		if val == "" {
			return fmt.Errorf("status mapping %s must not be empty", name)
		}
	}

	seen := make(map[string]string, len(fields))
	for name, val := range fields {
		if prev, ok := seen[val]; ok {
			return fmt.Errorf("duplicate status %q in mapping fields %s and %s", val, prev, name)
		}
		seen[val] = name
	}

	return nil
}
