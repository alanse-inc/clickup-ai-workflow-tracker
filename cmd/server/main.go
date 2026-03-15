package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/clickup"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/config"
	gh "github.com/rikeda71/clickup-ai-orchestrator/internal/github"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/logging"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/orchestrator"
)

func main() {
	logger := logging.NewLogger(slog.LevelInfo)
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config_validation_failed", "error", err)
		os.Exit(1)
	}

	clickupClient := clickup.NewClient(cfg.ClickUpAPIToken, cfg.ClickUpListID)

	if err := validateStatuses(clickupClient, cfg); err != nil {
		slog.Error("status_validation_failed", "error", err)
		os.Exit(1)
	}

	var githubAuth gh.Authenticator
	switch cfg.AuthMode {
	case "app":
		var err error
		githubAuth, err = gh.NewGitHubAppAuthenticator(cfg.GitHubAppID, cfg.GitHubAppInstallationID, []byte(cfg.GitHubAppPrivateKey))
		if err != nil {
			slog.Error("github_app_auth_failed", "error", err)
			os.Exit(1)
		}
	default:
		githubAuth = gh.NewPATAuthenticator(cfg.GitHubPAT)
	}
	githubDispatcher := gh.NewDispatcher(githubAuth, cfg.GitHubOwner, cfg.GitHubRepo, cfg.GitHubWorkflowFile)
	pollInterval := time.Duration(cfg.PollIntervalMS) * time.Millisecond

	orch := orchestrator.New(clickupClient, githubDispatcher, pollInterval, cfg.StatusMapping, logger, cfg.MaxConcurrentTasks)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	slog.InfoContext(ctx, "service_started",
		"poll_interval_ms", cfg.PollIntervalMS,
		"clickup_list_id", cfg.ClickUpListID,
		"github_repo", cfg.GitHubOwner+"/"+cfg.GitHubRepo,
	)

	orch.Run(ctx)
	slog.InfoContext(ctx, "service_stopped")
}

func validateStatuses(client *clickup.Client, cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	boardStatuses, err := client.GetStatuses(ctx)
	if err != nil {
		return fmt.Errorf("fetching ClickUp statuses: %w", err)
	}

	statusSet := make(map[string]struct{}, len(boardStatuses))
	for _, s := range boardStatuses {
		statusSet[s] = struct{}{}
	}

	var missing []string
	for _, s := range cfg.StatusMapping.AllStatuses() {
		if _, ok := statusSet[s]; !ok {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("statuses not found on ClickUp board: %v", missing)
	}
	return nil
}
