package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	orchCfg := orchestrator.Config{
		PollInterval:       time.Duration(cfg.PollIntervalMS) * time.Millisecond,
		StatusMapping:      cfg.StatusMapping,
		MaxConcurrentTasks: cfg.MaxConcurrentTasks,
	}

	var wg sync.WaitGroup
	for _, proj := range cfg.Projects {
		clickupClient := clickup.NewClient(cfg.ClickUpAPIToken, proj.ClickUpListID)

		if err := validateStatuses(clickupClient, cfg); err != nil {
			slog.Error("status_validation_failed", "error", err, "project", proj.GitHubOwner+"/"+proj.GitHubRepo)
			os.Exit(1)
		}

		githubDispatcher := gh.NewDispatcher(githubAuth, proj.GitHubOwner, proj.GitHubRepo, proj.GitHubWorkflowFile)
		projectLogger := logger.With("project", proj.GitHubOwner+"/"+proj.GitHubRepo)
		orch := orchestrator.New(clickupClient, githubDispatcher, orchCfg, projectLogger)

		slog.InfoContext(ctx, "service_started",
			"poll_interval_ms", cfg.PollIntervalMS,
			"clickup_list_id", proj.ClickUpListID,
			"github_repo", proj.GitHubOwner+"/"+proj.GitHubRepo,
		)

		wg.Add(1)
		go func() {
			defer wg.Done()
			orch.Run(ctx)
		}()
	}

	wg.Wait()
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
