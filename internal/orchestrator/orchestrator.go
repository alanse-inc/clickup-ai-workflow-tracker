package orchestrator

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/clickup"
	"github.com/rikeda71/clickup-ai-orchestrator/internal/logging"
)

// TaskClient は ClickUp のタスク操作を行うインターフェース
type TaskClient interface {
	GetTasks(ctx context.Context) ([]clickup.Task, error)
	GetTask(ctx context.Context, taskID string) (*clickup.Task, error)
	UpdateTaskStatus(ctx context.Context, taskID string, status string) error
}

// WorkflowDispatcher は GitHub Actions をトリガーするインターフェース
type WorkflowDispatcher interface {
	TriggerWorkflow(ctx context.Context, taskID string, phase string, statusOnSuccess string, statusOnError string, specOutput string) error
}

// PRChecker は GitHub PR のマージ状態を確認するインターフェース
type PRChecker interface {
	IsPRMerged(ctx context.Context, taskID string) (bool, error)
	IsSpecPRMerged(ctx context.Context, taskID string) (bool, error)
}

// Config は Orchestrator の設定を保持する
type Config struct {
	PollInterval    time.Duration
	StatusMapping   clickup.StatusMapping
	ShutdownTimeout time.Duration // デフォルト: 30s
	SpecOutput      string        // "clickup" (default) or "repo"
}

// Orchestrator はポーリングループとディスパッチロジックを管理する
type Orchestrator struct {
	taskClient      TaskClient
	dispatcher      WorkflowDispatcher
	prChecker       PRChecker // nil の場合は PR マージ自動検知を無効化
	state           *AgentState
	limiter         *ConcurrencyLimiter
	pollInterval    time.Duration
	statusMapping   clickup.StatusMapping
	specOutput      string // "clickup" or "repo"
	logger          *slog.Logger
	retryTimers     map[string]*retryEntry
	retryMu         sync.Mutex
	ctx             context.Context
	done            bool // shutdown が完了したかどうか
	shutdownTimeout time.Duration
	dispatchWg      sync.WaitGroup
	projectLabel    string // "owner/repo" 形式のプロジェクト識別子
}

type retryEntry struct {
	taskID     string
	phase      string
	attempt    int
	timer      *time.Timer
	retryAfter time.Time
}

// OrchestratorStatus は単一 Orchestrator のスナップショット
type OrchestratorStatus struct {
	Project      string
	RunningTasks []RunningTaskInfo
	RetryPending []RetryInfo
}

// RunningTaskInfo は実行中タスクの情報
type RunningTaskInfo struct {
	TaskID    string
	StartedAt time.Time
}

// RetryInfo はリトライ待ちタスクの情報
type RetryInfo struct {
	TaskID     string
	Phase      string
	Attempt    int
	RetryAfter time.Time
}

// New は新しい Orchestrator を返す。
// limiter が nil の場合は並行数制限なし。
// 複数 Orchestrator 間で ConcurrencyLimiter を共有することで、グローバルな並行タスク数制限を実現できる。
// projectLabel は "owner/repo" 形式のプロジェクト識別子で、Status() のレスポンスに含まれる。
// prChecker が nil の場合は PR マージ自動検知を無効化し、後方互換動作を維持する。
func New(taskClient TaskClient, dispatcher WorkflowDispatcher, cfg Config, logger *slog.Logger, limiter *ConcurrencyLimiter, projectLabel string, prChecker PRChecker) *Orchestrator {
	if logger == nil {
		logger = slog.Default()
	}
	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
	}
	return &Orchestrator{
		taskClient:      taskClient,
		dispatcher:      dispatcher,
		prChecker:       prChecker,
		state:           NewAgentState(),
		limiter:         limiter,
		pollInterval:    cfg.PollInterval,
		statusMapping:   cfg.StatusMapping,
		logger:          logger,
		retryTimers:     make(map[string]*retryEntry),
		shutdownTimeout: shutdownTimeout,
		projectLabel:    projectLabel,
		specOutput:      cfg.SpecOutput,
	}
}

// Status は現在の状態スナップショットを返す（外部I/Oなし）
func (o *Orchestrator) Status() OrchestratorStatus {
	runningMap := o.state.RunningTasksSnapshot()
	runningTasks := make([]RunningTaskInfo, 0, len(runningMap))
	for taskID, startedAt := range runningMap {
		runningTasks = append(runningTasks, RunningTaskInfo{
			TaskID:    taskID,
			StartedAt: startedAt,
		})
	}

	o.retryMu.Lock()
	retryPending := make([]RetryInfo, 0, len(o.retryTimers))
	for _, entry := range o.retryTimers {
		retryPending = append(retryPending, RetryInfo{
			TaskID:     entry.taskID,
			Phase:      entry.phase,
			Attempt:    entry.attempt,
			RetryAfter: entry.retryAfter,
		})
	}
	o.retryMu.Unlock()

	return OrchestratorStatus{
		Project:      o.projectLabel,
		RunningTasks: runningTasks,
		RetryPending: retryPending,
	}
}

// Run はメインポーリングループ。即時ティック実行後、ctx がキャンセルされるまで pollInterval ごとにティック実行する
func (o *Orchestrator) Run(ctx context.Context) {
	o.logger.Info("orchestrator started", "poll_interval", o.pollInterval.String())

	o.ctx = ctx

	// 再起動時の processing タスク復旧（tick より前に実行）
	o.recoverProcessingTasks(ctx)

	// SPEC 8.1: 起動時に即時ティックを実行
	o.tick(ctx)

	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			o.shutdown()
			o.logger.Info("orchestrator stopped")
			return
		case <-ticker.C:
			o.tick(ctx)
		}
	}
}

// shutdown は全リトライタイマーを停止し、実行中の dispatch 完了を待機する
func (o *Orchestrator) shutdown() {
	o.retryMu.Lock()
	o.done = true
	for taskID, entry := range o.retryTimers {
		entry.timer.Stop()
		delete(o.retryTimers, taskID)
	}
	o.retryMu.Unlock()

	done := make(chan struct{})
	go func() {
		o.dispatchWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		o.logger.Info("graceful shutdown completed")
	case <-time.After(o.shutdownTimeout):
		o.logger.Warn("graceful shutdown timed out, forcing stop", "timeout", o.shutdownTimeout)
	}
}

// recoverProcessingTasks は起動時に processing ステータスのタスクをトリガーステータスに巻き戻す。
// 再起動前に generating spec / implementing に変更されたタスクが stuck するのを防ぐ。
func (o *Orchestrator) recoverProcessingTasks(ctx context.Context) {
	tasks, err := o.taskClient.GetTasks(ctx)
	if err != nil {
		o.logger.Error("failed to fetch tasks for recovery", "error", err)
		return
	}

	for _, task := range tasks {
		if !o.statusMapping.IsProcessingStatus(task.Status) {
			continue
		}

		phase, err := o.statusMapping.PhaseFromStatus(task.Status)
		if err != nil {
			o.logger.Error("failed to determine phase for recovery",
				"task_id", task.ID, "status", task.Status, "error", err)
			continue
		}

		triggerStatus := o.statusMapping.ErrorStatusFor(phase)
		if err := o.taskClient.UpdateTaskStatus(ctx, task.ID, triggerStatus); err != nil {
			o.logger.Error("failed to revert task status for recovery",
				"task_id", task.ID, "from_status", task.Status,
				"to_status", triggerStatus, "error", err)
			continue
		}

		o.logger.Info("recovered processing task",
			"task_id", task.ID, "from_status", task.Status, "to_status", triggerStatus)
	}
}

// hasRetryPending はタスクにリトライタイマーが設定されているかを返す
func (o *Orchestrator) hasRetryPending(taskID string) bool {
	o.retryMu.Lock()
	defer o.retryMu.Unlock()
	_, ok := o.retryTimers[taskID]
	return ok
}

// tick は1ティックの処理を実行する
func (o *Orchestrator) tick(ctx context.Context) {
	o.reconcile(ctx)

	tasks, err := o.taskClient.GetTasks(ctx)
	if err != nil {
		o.logger.Error("failed to fetch tasks", "error", err)
		return
	}

	for _, task := range tasks {
		if o.statusMapping.IsTriggerStatus(task.Status) && !o.hasRetryPending(task.ID) {
			if !o.dispatch(ctx, task, 1) {
				break
			}
		}
	}
}

// reconcile は実行中タスクのリコンシリエーションを行う
func (o *Orchestrator) reconcile(ctx context.Context) {
	runningIDs := o.state.RunningTaskIDs()
	for _, taskID := range runningIDs {
		task, err := o.taskClient.GetTask(ctx, taskID)
		if err != nil {
			o.logger.Warn("failed to get task for reconciliation, skipping", "task_id", taskID, "error", err)
			continue
		}

		// PR Review ステータスかつ PRChecker が有効な場合、マージ状態を確認する。
		// このブロックは下流の IsTerminalStatus / IsProcessingStatus 判定を完全にバイパスする。
		// prChecker == nil の場合は「処理中でも終端でもない → reconciliation_release」パスへ移行する。
		if o.prChecker != nil && task.Status == o.statusMapping.PRReview {
			merged, err := o.prChecker.IsPRMerged(ctx, taskID)
			if err != nil {
				o.logger.Warn("failed to check PR merge status, skipping",
					"task_id", taskID, "error", err)
				continue
			}
			if merged {
				if err := o.taskClient.UpdateTaskStatus(ctx, taskID, o.statusMapping.Closed); err != nil {
					o.logger.Error("failed to update task to closed",
						"task_id", taskID, "error", err)
					continue
				}
				o.logger.Info("pr merged, task closed", "task_id", taskID)
				o.release(taskID)
				continue
			}
			// 未マージ: 処理中として維持
			continue
		}

		// Spec Review ステータスかつ PRChecker が有効な場合、SPEC PR のマージ状態を確認する。
		// SPEC PR がマージされていれば "ready for code" へ遷移する。
		// prChecker == nil の場合は「処理中でも終端でもない → reconciliation_release」パスへ移行する。
		if o.prChecker != nil && task.Status == o.statusMapping.SpecReview {
			merged, err := o.prChecker.IsSpecPRMerged(ctx, taskID)
			if err != nil {
				o.logger.Warn("failed to check spec PR merge status, skipping",
					"task_id", taskID, "error", err)
				continue
			}
			if merged {
				if err := o.taskClient.UpdateTaskStatus(ctx, taskID, o.statusMapping.ReadyForCode); err != nil {
					o.logger.Error("failed to update task to ready for code",
						"task_id", taskID, "error", err)
					continue
				}
				o.logger.Info("spec pr merged, task ready for code", "task_id", taskID)
				o.release(taskID)
				continue
			}
			// 未マージ: 処理中として維持
			continue
		}

		if o.statusMapping.IsTerminalStatus(task.Status) {
			o.logger.Info("task reached terminal status, releasing", "task_id", taskID, "status", task.Status)
			o.release(taskID)
			continue
		}

		if o.statusMapping.IsProcessingStatus(task.Status) {
			continue
		}

		// 処理中でも終端でもない場合（トリガー状態に戻った場合や手動変更を含む）はリリース
		o.logger.Info("reconciliation_release", "task_id", taskID, "status", task.Status)
		o.release(taskID)
	}
}

// dispatch はタスクのディスパッチを行う。attempt はリトライ回数で、失敗時に scheduleRetry に引き継がれる。
// 並行数上限に達した場合は false を返し、呼び出し元で残りタスクの処理を打ち切れるようにする。
func (o *Orchestrator) dispatch(ctx context.Context, task clickup.Task, attempt int) bool {
	o.retryMu.Lock()
	if o.done {
		o.retryMu.Unlock()
		return true
	}
	o.dispatchWg.Add(1)
	o.retryMu.Unlock()
	defer o.dispatchWg.Done()

	if !o.state.Claim(task.ID) {
		o.logger.Warn("task_already_claimed", "task_id", task.ID, "status", task.Status)
		return true
	}

	if !o.limiter.TryAcquire() {
		o.logger.Info("max concurrent tasks reached", "task_id", task.ID)
		o.state.Release(task.ID)
		return false
	}

	phase, err := o.statusMapping.PhaseFromStatus(task.Status)
	if err != nil {
		o.logger.Error("failed to determine phase", "task_id", task.ID, "status", task.Status, "error", err)
		o.release(task.ID)
		return true
	}

	phaseStr := string(phase)
	tl := logging.TaskLogger(o.logger, task.ID, phaseStr)

	processingStatus := o.statusMapping.ProcessingStatusFor(phase)
	if err := o.taskClient.UpdateTaskStatus(ctx, task.ID, processingStatus); err != nil {
		tl.Error("failed to update task status", "status", processingStatus, "error", err)
		o.release(task.ID)
		o.scheduleRetry(task.ID, phaseStr, attempt, err)
		return true
	}

	successStatus := o.statusMapping.SuccessStatusFor(phase)
	errorStatus := o.statusMapping.ErrorStatusFor(phase)
	if err := o.dispatcher.TriggerWorkflow(ctx, task.ID, phaseStr, successStatus, errorStatus, o.specOutput); err != nil {
		tl.Error("failed to trigger workflow", "error", err)
		// ベストエフォートでステータスを元に戻す
		if revertErr := o.taskClient.UpdateTaskStatus(ctx, task.ID, errorStatus); revertErr != nil {
			tl.Error("failed to revert task status", "status", errorStatus, "error", revertErr)
		}
		o.release(task.ID)
		o.scheduleRetry(task.ID, phaseStr, attempt, err)
		return true
	}

	o.state.MarkRunning(task.ID)
	tl.Info("task dispatched")
	return true
}

// release はローカル state とグローバル limiter の両方を解放する
func (o *Orchestrator) release(taskID string) {
	o.state.Release(taskID)
	o.limiter.Release()
}

const (
	// retryBaseDelayMS はリトライバックオフの基底遅延（ミリ秒）
	retryBaseDelayMS = 10000
	// retryMaxDelayMS はリトライバックオフの最大遅延（ミリ秒）
	retryMaxDelayMS = 300000
	// retryMaxExponent はビットシフトオーバーフロー防止のための最大指数
	retryMaxExponent = 5 // 2^5 = 32 → 10000 * 32 = 320000 → capped to 300000
)

// calcRetryDelay はリトライのバックオフ遅延を計算する
func calcRetryDelay(attempt int) time.Duration {
	exp := attempt - 1
	if exp > retryMaxExponent {
		exp = retryMaxExponent
	}
	delay := retryBaseDelayMS * (1 << exp)
	if delay > retryMaxDelayMS {
		delay = retryMaxDelayMS
	}
	return time.Duration(delay) * time.Millisecond
}

// scheduleRetry はリトライタイマーを設定する
func (o *Orchestrator) scheduleRetry(taskID string, phase string, attempt int, err error) {
	o.retryMu.Lock()
	defer o.retryMu.Unlock()

	if o.done {
		return
	}

	delayDuration := calcRetryDelay(attempt)

	tl := logging.TaskLogger(o.logger, taskID, phase)
	tl.Warn("scheduling retry", "attempt", attempt, "delay", delayDuration, "error", err)

	// 既存のタイマーがあればキャンセル
	if existing, ok := o.retryTimers[taskID]; ok {
		existing.timer.Stop()
	}

	timer := time.AfterFunc(delayDuration, func() {
		o.handleRetry(taskID, phase, attempt)
	})

	o.retryTimers[taskID] = &retryEntry{
		taskID:     taskID,
		phase:      phase,
		attempt:    attempt,
		timer:      timer,
		retryAfter: time.Now().Add(delayDuration),
	}
}

// handleRetry はリトライを処理する
func (o *Orchestrator) handleRetry(taskID string, phase string, attempt int) {
	o.retryMu.Lock()
	if o.done {
		o.retryMu.Unlock()
		return
	}
	// 現在の entry の attempt が一致するか確認（Stop 後に旧コールバックが走るレース対策）
	entry, ok := o.retryTimers[taskID]
	if !ok || entry.attempt != attempt {
		o.retryMu.Unlock()
		return
	}
	delete(o.retryTimers, taskID)
	o.retryMu.Unlock()

	tl := logging.TaskLogger(o.logger, taskID, phase)

	ctx := o.ctx
	task, err := o.taskClient.GetTask(ctx, taskID)
	if err != nil {
		tl.Error("failed to get task for retry", "attempt", attempt, "error", err)
		o.scheduleRetry(taskID, phase, attempt+1, err)
		return
	}

	if o.statusMapping.IsTriggerStatus(task.Status) {
		tl.Info("retrying dispatch", "attempt", attempt)
		_ = o.dispatch(ctx, *task, attempt+1)
	} else {
		tl.Info("task no longer in trigger status, releasing", "status", task.Status)
		// リトライ待ちに入る前の dispatch 失敗パスで既に limiter は解放済みのため、state のみ解放する
		o.state.Release(taskID)
	}
}
