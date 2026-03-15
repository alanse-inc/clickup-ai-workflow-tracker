package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rikeda71/clickup-ai-workflow-tracker/internal/clickup"
	"github.com/rikeda71/clickup-ai-workflow-tracker/internal/config"
	gh "github.com/rikeda71/clickup-ai-workflow-tracker/internal/github"
	"github.com/rikeda71/clickup-ai-workflow-tracker/internal/orchestrator"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config_validation_failed", "error", err)
		os.Exit(1)
	}

	clickupClient := clickup.NewClient(cfg.ClickUpAPIToken, cfg.ClickUpListID)
	githubAuth := gh.NewPATAuthenticator(cfg.GitHubPAT)
	githubDispatcher := gh.NewDispatcher(githubAuth, cfg.GitHubOwner, cfg.GitHubRepo, cfg.GitHubWorkflowFile)
	pollInterval := time.Duration(cfg.PollIntervalMS) * time.Millisecond

	orch := orchestrator.New(clickupClient, githubDispatcher, pollInterval)

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
