package orchestrator

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/rikeda71/clickup-ai-workflow-tracker/internal/clickup"
)

// TaskClient は ClickUp のタスク操作を行うインターフェース
type TaskClient interface {
	GetTasks(ctx context.Context) ([]clickup.Task, error)
	GetTask(ctx context.Context, taskID string) (*clickup.Task, error)
	UpdateTaskStatus(ctx context.Context, taskID string, status string) error
}

// WorkflowDispatcher は GitHub Actions をトリガーするインターフェース
type WorkflowDispatcher interface {
	TriggerWorkflow(ctx context.Context, taskID string, phase string, statusOnSuccess string, statusOnError string) error
}

// Orchestrator はポーリングループとディスパッチロジックを管理する
type Orchestrator struct {
	taskClient   TaskClient
	dispatcher   WorkflowDispatcher
	state        *AgentState
	pollInterval time.Duration
	retryTimers  map[string]*retryEntry
	retryMu      sync.Mutex
	ctx          context.Context
	done         bool // shutdown が完了したかどうか
}

type retryEntry struct {
	taskID  string
	phase   string
	attempt int
	timer   *time.Timer
}

// New は新しい Orchestrator を返す
func New(taskClient TaskClient, dispatcher WorkflowDispatcher, pollInterval time.Duration) *Orchestrator {
	return &Orchestrator{
		taskClient:   taskClient,
		dispatcher:   dispatcher,
		state:        NewAgentState(),
		pollInterval: pollInterval,
		retryTimers:  make(map[string]*retryEntry),
	}
}

// Run はメインポーリングループ。即時ティック実行後、ctx がキャンセルされるまで pollInterval ごとにティック実行する
func (o *Orchestrator) Run(ctx context.Context) {
	slog.Info("orchestrator started", "poll_interval", o.pollInterval.String())

	o.ctx = ctx

	// SPEC 8.1: 起動時に即時ティックを実行
	o.tick(ctx)

	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			o.shutdown()
			slog.Info("orchestrator stopped")
			return
		case <-ticker.C:
			o.tick(ctx)
		}
	}
}

// shutdown は全リトライタイマーを停止する
func (o *Orchestrator) shutdown() {
	o.retryMu.Lock()
	defer o.retryMu.Unlock()

	o.done = true
	for taskID, entry := range o.retryTimers {
		entry.timer.Stop()
		delete(o.retryTimers, taskID)
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
		slog.Error("failed to fetch tasks", "error", err)
		return
	}

	for _, task := range tasks {
		if clickup.IsTriggerStatus(task.Status) && !o.hasRetryPending(task.ID) {
			o.dispatch(ctx, task, 1)
		}
	}
}

// reconcile は実行中タスクのリコンシリエーションを行う
func (o *Orchestrator) reconcile(ctx context.Context) {
	runningIDs := o.state.RunningTaskIDs()
	for _, taskID := range runningIDs {
		task, err := o.taskClient.GetTask(ctx, taskID)
		if err != nil {
			slog.Warn("failed to get task for reconciliation, skipping", "task_id", taskID, "error", err)
			continue
		}

		if clickup.IsTerminalStatus(task.Status) {
			slog.Info("task reached terminal status, releasing", "task_id", taskID, "status", task.Status)
			o.state.Release(taskID)
			continue
		}

		if clickup.IsProcessingStatus(task.Status) {
			continue
		}

		// 処理中でも終端でもない場合（トリガー状態に戻った場合や手動変更を含む）はリリース
		slog.Info("reconciliation_release", "task_id", taskID, "status", task.Status)
		o.state.Release(taskID)
	}
}

// dispatch はタスクのディスパッチを行う。attempt はリトライ回数で、失敗時に scheduleRetry に引き継がれる。
func (o *Orchestrator) dispatch(ctx context.Context, task clickup.Task, attempt int) {
	if !o.state.Claim(task.ID) {
		slog.Warn("task_already_claimed", "task_id", task.ID, "status", task.Status)
		return
	}

	phase, err := clickup.PhaseFromStatus(task.Status)
	if err != nil {
		slog.Error("failed to determine phase", "task_id", task.ID, "status", task.Status, "error", err)
		o.state.Release(task.ID)
		return
	}

	phaseStr := string(phase)
	processingStatus := clickup.ProcessingStatusFor(phase)
	if err := o.taskClient.UpdateTaskStatus(ctx, task.ID, processingStatus); err != nil {
		slog.Error("failed to update task status", "task_id", task.ID, "phase", phaseStr, "status", processingStatus, "error", err)
		o.state.Release(task.ID)
		o.scheduleRetry(task.ID, phaseStr, attempt, err)
		return
	}

	successStatus := clickup.SuccessStatusFor(phase)
	errorStatus := clickup.ErrorStatusFor(phase)
	if err := o.dispatcher.TriggerWorkflow(ctx, task.ID, phaseStr, successStatus, errorStatus); err != nil {
		slog.Error("failed to trigger workflow", "task_id", task.ID, "phase", phaseStr, "error", err)
		// ベストエフォートでステータスを元に戻す
		if revertErr := o.taskClient.UpdateTaskStatus(ctx, task.ID, errorStatus); revertErr != nil {
			slog.Error("failed to revert task status", "task_id", task.ID, "status", errorStatus, "error", revertErr)
		}
		o.state.Release(task.ID)
		o.scheduleRetry(task.ID, phaseStr, attempt, err)
		return
	}

	o.state.MarkRunning(task.ID)
	slog.Info("task dispatched", "task_id", task.ID, "phase", phaseStr)
}

const (
	// retryBaseDelayMS はリトライバックオフの基底遅延（ミリ秒）
	retryBaseDelayMS = 10000
	// retryMaxDelayMS はリトライバックオフの最大遅延（ミリ秒）
	retryMaxDelayMS = 300000
	// retryMaxExponent はビットシフトオーバーフロー防止のための最大指数
	retryMaxExponent = 5 // 2^5 = 32 → 10000 * 32 = 320000 → capped to 300000
)

// scheduleRetry はリトライタイマーを設定する
func (o *Orchestrator) scheduleRetry(taskID string, phase string, attempt int, err error) {
	o.retryMu.Lock()
	defer o.retryMu.Unlock()

	if o.done {
		return
	}

	// delay = min(retryBaseDelayMS * 2^(attempt-1), retryMaxDelayMS) ms
	exp := attempt - 1
	if exp > retryMaxExponent {
		exp = retryMaxExponent
	}
	delay := retryBaseDelayMS * (1 << exp)
	if delay > retryMaxDelayMS {
		delay = retryMaxDelayMS
	}
	delayDuration := time.Duration(delay) * time.Millisecond

	slog.Warn("scheduling retry", "task_id", taskID, "phase", phase, "attempt", attempt, "delay_ms", delay, "error", err)

	// 既存のタイマーがあればキャンセル
	if existing, ok := o.retryTimers[taskID]; ok {
		existing.timer.Stop()
	}

	timer := time.AfterFunc(delayDuration, func() {
		o.handleRetry(taskID, phase, attempt)
	})

	o.retryTimers[taskID] = &retryEntry{
		taskID:  taskID,
		phase:   phase,
		attempt: attempt,
		timer:   timer,
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

	ctx := o.ctx
	task, err := o.taskClient.GetTask(ctx, taskID)
	if err != nil {
		slog.Error("failed to get task for retry", "task_id", taskID, "phase", phase, "attempt", attempt, "error", err)
		o.scheduleRetry(taskID, phase, attempt+1, err)
		return
	}

	if clickup.IsTriggerStatus(task.Status) {
		slog.Info("retrying dispatch", "task_id", taskID, "phase", phase, "attempt", attempt)
		o.dispatch(ctx, *task, attempt+1)
	} else {
		slog.Info("task no longer in trigger status, releasing", "task_id", taskID, "status", task.Status)
		o.state.Release(taskID)
	}
}
