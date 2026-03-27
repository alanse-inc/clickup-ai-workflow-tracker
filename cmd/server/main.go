package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/clickup"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/config"
	gh "github.com/rikeda71/clickup-ai-orchestrator/internal/github"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/health"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/logging"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/orchestrator"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/status"
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

	// 全プロジェクトのステータス検証を先に完了する
	clickupClients := make([]*clickup.Client, len(cfg.Projects))
	for i, proj := range cfg.Projects {
		clickupClients[i] = clickup.NewClient(cfg.ClickUpAPIToken, proj.ClickUpListID)
		if err := validateStatuses(clickupClients[i], cfg); err != nil {
			slog.Error("status_validation_failed", "error", err, "project", proj.GitHubOwner+"/"+proj.GitHubRepo)
			os.Exit(1)
		}
	}

	orchCfg := orchestrator.Config{
		PollInterval:  time.Duration(cfg.PollIntervalMS) * time.Millisecond,
		StatusMapping: cfg.StatusMapping,
	}

	// 全プロジェクトで共有するグローバル並行数リミッタ
	limiter := orchestrator.NewConcurrencyLimiter(cfg.MaxConcurrentTasks)

	dispatchers := make([]*gh.Dispatcher, len(cfg.Projects))
	for i, proj := range cfg.Projects {
		dispatchers[i] = gh.NewDispatcher(githubAuth, proj.GitHubOwner, proj.GitHubRepo, proj.GitHubWorkflowFile)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	providers := make([]status.StatusProvider, len(cfg.Projects))
	orchs := make([]*orchestrator.Orchestrator, len(cfg.Projects))
	for i, proj := range cfg.Projects {
		projectLabel := proj.GitHubOwner + "/" + proj.GitHubRepo
		projectLogger := logger.With("project", projectLabel)
		orch := orchestrator.New(clickupClients[i], dispatchers[i], orchCfg, projectLogger, limiter, projectLabel)
		orchs[i] = orch
		providers[i] = orch
	}

	mux := http.NewServeMux()
	mux.Handle("GET /health", health.NewHandler(clickupClients[0], dispatchers[0]))
	mux.Handle("GET /status", status.NewHandler(limiter, providers))

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	srvErrCh := make(chan error, 1)
	go func() {
		slog.Info("health_server_started", "port", port) //nolint:gosec // G706: port is from trusted env var
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srvErrCh <- err
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// シグナル受信時にヘルスチェックサーバーを即座に停止する
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("health_server_shutdown_error", "error", err)
		}
	}()

	var wg sync.WaitGroup
	for i, proj := range cfg.Projects {
		slog.InfoContext(ctx, "service_started",
			"poll_interval_ms", cfg.PollIntervalMS,
			"clickup_list_id", proj.ClickUpListID,
			"github_repo", proj.GitHubOwner+"/"+proj.GitHubRepo,
		)

		wg.Add(1)
		go func(o *orchestrator.Orchestrator) {
			defer wg.Done()
			o.Run(ctx)
		}(orchs[i])
	}

	select {
	case err := <-srvErrCh:
		slog.Error("health_server_failed", "error", err)
		os.Exit(1)
	case <-ctx.Done():
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
