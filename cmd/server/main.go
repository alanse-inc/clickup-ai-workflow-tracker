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
	for _, skippedErr := range cfg.SkippedProjectErrors {
		slog.Error("project_skipped", "error", skippedErr)
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

	pi, err := initProjects(cfg, githubAuth, logger)
	if err != nil {
		slog.Error("project_init_failed", "error", err)
		os.Exit(1)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	statusProviders := make([]status.StatusProvider, len(pi.orchs))
	limiterStatuses := make([]status.LimiterStatus, len(pi.orchs))
	for i, o := range pi.orchs {
		statusProviders[i] = o
		limiterStatuses[i] = pi.limiters[i]
	}
	mux.Handle("GET /health", health.NewHandler(pi.pingers))
	mux.Handle("GET /status", status.NewHandler(limiterStatuses, statusProviders))
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
			"poll_interval_ms", proj.PollIntervalMS,
			"clickup_list_id", proj.ClickUpListID,
			"github_repo", proj.GitHubOwner+"/"+proj.GitHubRepo,
		)

		wg.Add(1)
		go func(o *orchestrator.Orchestrator) {
			defer wg.Done()
			o.Run(ctx)
		}(pi.orchs[i])
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

type projectInstances struct {
	orchs    []*orchestrator.Orchestrator
	limiters []*orchestrator.ConcurrencyLimiter
	pingers  []health.ProjectPingers
}

// initProjects はプロジェクトのステータス検証を行い、正常なプロジェクトのみでインスタンスを構築する。
// 検証に失敗したプロジェクトはスキップし、正常なプロジェクトのみで稼働を継続する。
func initProjects(cfg *config.Config, githubAuth gh.Authenticator, logger *slog.Logger) (*projectInstances, error) {
	var orchs []*orchestrator.Orchestrator
	var limiters []*orchestrator.ConcurrencyLimiter
	var pingers []health.ProjectPingers

	for _, proj := range cfg.Projects {
		client := clickup.NewClient(cfg.ClickUpAPIToken, proj.ClickUpListID)
		if err := validateStatuses(client, proj.StatusMapping); err != nil {
			slog.Error("project_skipped", "error", err, "project", proj.GitHubOwner+"/"+proj.GitHubRepo)
			continue
		}

		dispatcher := gh.NewDispatcher(githubAuth, proj.GitHubOwner, proj.GitHubRepo, proj.GitHubWorkflowFile)
		prChecker := gh.NewPRChecker(githubAuth, proj.GitHubOwner, proj.GitHubRepo)
		limiter := orchestrator.NewConcurrencyLimiter(proj.MaxConcurrentTasks)

		projectLabel := proj.GitHubOwner + "/" + proj.GitHubRepo
		orchCfg := orchestrator.Config{
			PollInterval:    time.Duration(proj.PollIntervalMS) * time.Millisecond,
			StatusMapping:   proj.StatusMapping,
			ShutdownTimeout: time.Duration(proj.ShutdownTimeoutMS) * time.Millisecond,
			SpecOutput:      proj.SpecOutput,
		}
		orchs = append(orchs, orchestrator.New(client, dispatcher, orchCfg, logger.With("project", projectLabel), limiter, projectLabel, prChecker))
		limiters = append(limiters, limiter)
		pingers = append(pingers, health.ProjectPingers{
			Name:    projectLabel,
			ClickUp: client,
			GitHub:  dispatcher,
		})
	}

	if len(orchs) == 0 {
		return nil, fmt.Errorf("no valid projects after status validation")
	}

	return &projectInstances{orchs: orchs, limiters: limiters, pingers: pingers}, nil
}

func validateStatuses(client *clickup.Client, sm clickup.StatusMapping) error {
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
	for _, s := range sm.AllStatuses() {
		if _, ok := statusSet[s]; !ok {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("statuses not found on ClickUp board: %v", missing)
	}
	return nil
}
