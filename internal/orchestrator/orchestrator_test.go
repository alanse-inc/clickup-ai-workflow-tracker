package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rikeda71/clickup-ai-workflow-tracker/internal/clickup"
)

// mockTaskClient は TaskClient のモック
type mockTaskClient struct {
	mu            sync.Mutex
	tasks         []clickup.Task
	taskMap       map[string]*clickup.Task
	getTasksErr   error
	getTaskErr    error
	updateErr     error
	updateCalls   []updateCall
	getTasksCalls int
	getTaskCalls  []string
}

type updateCall struct {
	TaskID string
	Status string
}

func (m *mockTaskClient) GetTasks(_ context.Context) ([]clickup.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getTasksCalls++
	if m.getTasksErr != nil {
		return nil, m.getTasksErr
	}
	return m.tasks, nil
}

func (m *mockTaskClient) GetTask(_ context.Context, taskID string) (*clickup.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getTaskCalls = append(m.getTaskCalls, taskID)
	if m.getTaskErr != nil {
		return nil, m.getTaskErr
	}
	if task, ok := m.taskMap[taskID]; ok {
		return task, nil
	}
	return nil, fmt.Errorf("task not found: %s", taskID)
}

func (m *mockTaskClient) UpdateTaskStatus(_ context.Context, taskID string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls = append(m.updateCalls, updateCall{TaskID: taskID, Status: status})
	return m.updateErr
}

// mockWorkflowDispatcher は WorkflowDispatcher のモック
type mockWorkflowDispatcher struct {
	mu           sync.Mutex
	triggerErr   error
	triggerCalls []triggerCall
}

type triggerCall struct {
	TaskID          string
	Phase           string
	StatusOnSuccess string
	StatusOnError   string
}

func (m *mockWorkflowDispatcher) TriggerWorkflow(_ context.Context, taskID string, phase string, statusOnSuccess string, statusOnError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.triggerCalls = append(m.triggerCalls, triggerCall{
		TaskID:          taskID,
		Phase:           phase,
		StatusOnSuccess: statusOnSuccess,
		StatusOnError:   statusOnError,
	})
	return m.triggerErr
}

func TestTick_DispatchesTriggerStatusTasks(t *testing.T) {
	fetcher := &mockTaskClient{
		tasks: []clickup.Task{
			{ID: "task-1", Status: clickup.StatusReadyForSpec},
			{ID: "task-2", Status: clickup.StatusReadyForCode},
			{ID: "task-3", Status: clickup.StatusGeneratingSpec}, // not trigger
		},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	o.tick(context.Background())

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()

	if len(dispatcher.triggerCalls) != 2 {
		t.Fatalf("expected 2 trigger calls, got %d", len(dispatcher.triggerCalls))
	}

	// task-1 should be SPEC, task-2 should be CODE
	found := map[string]string{}
	for _, call := range dispatcher.triggerCalls {
		found[call.TaskID] = call.Phase
	}
	if found["task-1"] != string(clickup.PhaseSpec) {
		t.Errorf("expected task-1 phase SPEC, got %s", found["task-1"])
	}
	if found["task-2"] != string(clickup.PhaseCode) {
		t.Errorf("expected task-2 phase CODE, got %s", found["task-2"])
	}
}

func TestTick_GetTasksError(t *testing.T) {
	fetcher := &mockTaskClient{
		getTasksErr: fmt.Errorf("api error"),
		taskMap:     map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	// Should not panic
	o.tick(context.Background())

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 0 {
		t.Fatalf("expected 0 trigger calls on error, got %d", len(dispatcher.triggerCalls))
	}
}

func TestReconcile_TerminalStatusReleased(t *testing.T) {
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: clickup.StatusClosed},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released after terminal status")
	}
}

func TestReconcile_ProcessingStatusKept(t *testing.T) {
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: clickup.StatusGeneratingSpec},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	if !o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to remain running during processing status")
	}
}

func TestReconcile_NonProcessingStatusReleased(t *testing.T) {
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: clickup.StatusSpecReview},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released for non-processing status")
	}
}

func TestReconcile_TriggerStatusReleased(t *testing.T) {
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: clickup.StatusReadyForSpec},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released when reverted to trigger status")
	}
}

func TestReconcile_APIErrorSkips(t *testing.T) {
	fetcher := &mockTaskClient{
		getTaskErr: fmt.Errorf("api error"),
		taskMap:    map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	// Should still be running (skipped due to error)
	if !o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to remain running on API error")
	}
}

func TestDispatch_NormalFlow(t *testing.T) {
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	task := clickup.Task{ID: "task-1", Status: clickup.StatusReadyForSpec}
	o.dispatch(context.Background(), task, 1)

	// Check status update
	fetcher.mu.Lock()
	if len(fetcher.updateCalls) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(fetcher.updateCalls))
	}
	if fetcher.updateCalls[0].Status != clickup.StatusGeneratingSpec {
		t.Errorf("expected status %s, got %s", clickup.StatusGeneratingSpec, fetcher.updateCalls[0].Status)
	}
	fetcher.mu.Unlock()

	// Check workflow trigger
	dispatcher.mu.Lock()
	if len(dispatcher.triggerCalls) != 1 {
		t.Fatalf("expected 1 trigger call, got %d", len(dispatcher.triggerCalls))
	}
	call := dispatcher.triggerCalls[0]
	if call.TaskID != "task-1" {
		t.Errorf("expected task ID task-1, got %s", call.TaskID)
	}
	if call.Phase != string(clickup.PhaseSpec) {
		t.Errorf("expected phase SPEC, got %s", call.Phase)
	}
	if call.StatusOnSuccess != clickup.StatusSpecReview {
		t.Errorf("expected success status %s, got %s", clickup.StatusSpecReview, call.StatusOnSuccess)
	}
	if call.StatusOnError != clickup.StatusReadyForSpec {
		t.Errorf("expected error status %s, got %s", clickup.StatusReadyForSpec, call.StatusOnError)
	}
	dispatcher.mu.Unlock()

	// Check state
	if !o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be claimed and running")
	}
}

func TestDispatch_AlreadyClaimed(t *testing.T) {
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	o.state.Claim("task-1")

	task := clickup.Task{ID: "task-1", Status: clickup.StatusReadyForSpec}
	o.dispatch(context.Background(), task, 1)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 0 {
		t.Fatal("expected 0 trigger calls for already claimed task")
	}
}

func TestDispatch_UpdateStatusError(t *testing.T) {
	fetcher := &mockTaskClient{
		updateErr: fmt.Errorf("update error"),
		taskMap:   map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)
	defer o.shutdown()

	task := clickup.Task{ID: "task-1", Status: clickup.StatusReadyForSpec}
	o.dispatch(context.Background(), task, 1)

	// Task should be released
	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released after update error")
	}

	// No workflow trigger
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 0 {
		t.Fatal("expected 0 trigger calls after update error")
	}
}

func TestDispatch_TriggerWorkflowError(t *testing.T) {
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{
		triggerErr: fmt.Errorf("trigger error"),
	}
	o := New(fetcher, dispatcher, time.Second)
	defer o.shutdown()

	task := clickup.Task{ID: "task-1", Status: clickup.StatusReadyForSpec}
	o.dispatch(context.Background(), task, 1)

	// Task should be released
	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released after trigger error")
	}
}

func TestDispatch_DuplicatePrevention(t *testing.T) {
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	task := clickup.Task{ID: "task-1", Status: clickup.StatusReadyForSpec}
	o.dispatch(context.Background(), task, 1)
	o.dispatch(context.Background(), task, 1) // second dispatch should be skipped

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 1 {
		t.Fatalf("expected 1 trigger call, got %d (duplicate prevention failed)", len(dispatcher.triggerCalls))
	}
}

func TestDispatch_CodePhase(t *testing.T) {
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)

	task := clickup.Task{ID: "task-1", Status: clickup.StatusReadyForCode}
	o.dispatch(context.Background(), task, 1)

	fetcher.mu.Lock()
	if len(fetcher.updateCalls) != 1 || fetcher.updateCalls[0].Status != clickup.StatusImplementing {
		t.Errorf("expected status update to %s", clickup.StatusImplementing)
	}
	fetcher.mu.Unlock()

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 1 {
		t.Fatalf("expected 1 trigger call, got %d", len(dispatcher.triggerCalls))
	}
	call := dispatcher.triggerCalls[0]
	if call.Phase != string(clickup.PhaseCode) {
		t.Errorf("expected phase CODE, got %s", call.Phase)
	}
	if call.StatusOnSuccess != clickup.StatusPRReview {
		t.Errorf("expected success status %s, got %s", clickup.StatusPRReview, call.StatusOnSuccess)
	}
	if call.StatusOnError != clickup.StatusReadyForCode {
		t.Errorf("expected error status %s, got %s", clickup.StatusReadyForCode, call.StatusOnError)
	}
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	fetcher := &mockTaskClient{
		tasks:   []clickup.Task{},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		o.Run(ctx)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}

func TestCalcRetryDelay(t *testing.T) {
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 10 * time.Second},   // 10000 * 2^0 = 10000ms
		{2, 20 * time.Second},   // 10000 * 2^1 = 20000ms
		{3, 40 * time.Second},   // 10000 * 2^2 = 40000ms
		{4, 80 * time.Second},   // 10000 * 2^3 = 80000ms
		{5, 160 * time.Second},  // 10000 * 2^4 = 160000ms
		{6, 300 * time.Second},  // 10000 * 2^5 = 320000ms → capped to 300000ms
		{7, 300 * time.Second},  // exponent capped at 5
		{10, 300 * time.Second}, // exponent capped at 5
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt=%d", tt.attempt), func(t *testing.T) {
			got := calcRetryDelay(tt.attempt)
			if got != tt.expected {
				t.Errorf("calcRetryDelay(%d) = %v, want %v", tt.attempt, got, tt.expected)
			}
		})
	}
}

func TestHandleRetry(t *testing.T) {
	tests := []struct {
		name           string
		taskStatus     string
		wantDispatched bool
		wantReleased   bool
	}{
		{
			name:           "redispatches when task is in trigger status",
			taskStatus:     clickup.StatusReadyForSpec,
			wantDispatched: true,
			// dispatch 内で再 Claim → MarkRunning されるため released=false
			wantReleased: false,
		},
		{
			name:           "releases when task is in non-trigger status",
			taskStatus:     clickup.StatusSpecReview,
			wantDispatched: false,
			wantReleased:   true,
		},
		{
			name:           "releases when task is in terminal status",
			taskStatus:     clickup.StatusClosed,
			wantDispatched: false,
			wantReleased:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := &mockTaskClient{
				taskMap: map[string]*clickup.Task{
					"task-1": {ID: "task-1", Status: tt.taskStatus},
				},
			}
			dispatcher := &mockWorkflowDispatcher{}
			o := New(fetcher, dispatcher, time.Second)
			defer o.shutdown()

			o.ctx = context.Background()
			if !tt.wantDispatched {
				// 非ディスパッチケースでは事前に Claim しておく（handleRetry 内で Release される）
				o.state.Claim("task-1")
			}

			o.retryMu.Lock()
			o.retryTimers["task-1"] = &retryEntry{taskID: "task-1", phase: "SPEC", attempt: 1}
			o.retryMu.Unlock()

			o.handleRetry("task-1", "SPEC", 1)

			dispatcher.mu.Lock()
			dispatched := len(dispatcher.triggerCalls) > 0
			dispatcher.mu.Unlock()

			if dispatched != tt.wantDispatched {
				t.Errorf("dispatched = %v, want %v", dispatched, tt.wantDispatched)
			}

			released := !o.state.IsClaimedOrRunning("task-1")
			if released != tt.wantReleased {
				t.Errorf("released = %v, want %v", released, tt.wantReleased)
			}
		})
	}
}

func TestScheduleRetry_CancelsExistingTimer(t *testing.T) {
	// handleRetry が呼ばれたときの attempt を記録するため、
	// トリガー状態のタスクを用意して dispatch まで到達させる
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: clickup.StatusReadyForSpec},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, time.Second)
	o.ctx = context.Background()
	defer o.shutdown()

	// attempt=1 で短い遅延のリトライをスケジュール
	o.scheduleRetry("task-1", "SPEC", 1, fmt.Errorf("error1"))

	// すぐに attempt=2 で上書き → attempt=1 のタイマーはキャンセルされるはず
	o.scheduleRetry("task-1", "SPEC", 2, fmt.Errorf("error2"))

	// エントリが attempt=2 に更新されていることを確認
	o.retryMu.Lock()
	entry, ok := o.retryTimers["task-1"]
	if !ok {
		o.retryMu.Unlock()
		t.Fatal("expected retry timer to exist")
	}
	if entry.attempt != 2 {
		o.retryMu.Unlock()
		t.Fatalf("expected attempt 2 (latest), got %d", entry.attempt)
	}
	o.retryMu.Unlock()

	// attempt=2 のタイマーが発火するまで待つ（バックオフ: 10s*2^1=20s だが実際のタイマーを使うため、
	// ここではタイマーを手動発火させて旧 attempt のガードを検証する）
	// handleRetry は attempt 不一致時に何もしないことを検証
	// shutdown で timer.Stop() が呼ばれるため、ダミータイマーを設定
	o.retryMu.Lock()
	o.retryTimers["task-1"] = &retryEntry{
		taskID:  "task-1",
		phase:   "SPEC",
		attempt: 2,
		timer:   time.NewTimer(time.Hour), // shutdown 用ダミー
	}
	o.retryMu.Unlock()

	// 旧 attempt=1 のコールバックが走った場合: attempt 不一致で早期リターンするはず
	o.handleRetry("task-1", "SPEC", 1)

	// dispatch が呼ばれていないことを確認（attempt 不一致でガードされた）
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 0 {
		t.Errorf("expected 0 trigger calls (old attempt should be ignored), got %d", len(dispatcher.triggerCalls))
	}
}
