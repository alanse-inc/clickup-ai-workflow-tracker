package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/rikeda71/clickup-ai-orchestrator/internal/clickup"
)

var (
	defaultSM     = clickup.DefaultStatusMapping()
	defaultLogger = slog.Default()
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
	updateDelay   time.Duration
	updateStarted chan struct{} // closed on first UpdateTaskStatus call (optional)
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

func (m *mockTaskClient) UpdateTaskStatus(ctx context.Context, taskID string, status string) error {
	if m.updateStarted != nil {
		select {
		case <-m.updateStarted:
		default:
			close(m.updateStarted)
		}
	}
	if m.updateDelay > 0 {
		select {
		case <-time.After(m.updateDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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
	SpecOutput      string
}

func (m *mockWorkflowDispatcher) TriggerWorkflow(_ context.Context, taskID string, phase string, statusOnSuccess string, statusOnError string, specOutput string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.triggerCalls = append(m.triggerCalls, triggerCall{
		TaskID:          taskID,
		Phase:           phase,
		StatusOnSuccess: statusOnSuccess,
		StatusOnError:   statusOnError,
		SpecOutput:      specOutput,
	})
	return m.triggerErr
}

// mockPRChecker は PRChecker のモック
type mockPRChecker struct {
	mu         sync.Mutex
	merged     map[string]bool
	specMerged map[string]bool
	err        error
	specErr    error
	calls      []string
	specCalls  []string
}

func (m *mockPRChecker) IsFeaturePRMerged(_ context.Context, taskID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, taskID)
	if m.err != nil {
		return false, m.err
	}
	return m.merged[taskID], nil
}

func (m *mockPRChecker) IsSpecPRMerged(_ context.Context, taskID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.specCalls = append(m.specCalls, taskID)
	if m.specErr != nil {
		return false, m.specErr
	}
	return m.specMerged[taskID], nil
}

func TestNew_NilLoggerFallback(t *testing.T) {
	fetcher := &mockTaskClient{
		tasks:   []clickup.Task{},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: defaultSM}, nil, nil, "", nil)

	// Should not panic; uses slog.Default()
	o.tick(context.Background())
}

func TestTick_DispatchesTriggerStatusTasks(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		tasks: []clickup.Task{
			{ID: "task-1", Status: sm.ReadyForSpec},
			{ID: "task-2", Status: sm.ReadyForCode},
			{ID: "task-3", Status: sm.GeneratingSpec}, // not trigger
		},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

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
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: defaultSM}, defaultLogger, nil, "", nil)

	// Should not panic
	o.tick(context.Background())

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 0 {
		t.Fatalf("expected 0 trigger calls on error, got %d", len(dispatcher.triggerCalls))
	}
}

func TestReconcile_TerminalStatusReleased(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: sm.Closed},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released after terminal status")
	}
}

func TestReconcile_ProcessingStatusKept(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: sm.GeneratingSpec},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	if !o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to remain running during processing status")
	}
}

func TestReconcile_NonProcessingStatusReleased(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: sm.SpecReview},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released for non-processing status")
	}
}

func TestReconcile_TriggerStatusReleased(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: sm.ReadyForSpec},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

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
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: defaultSM}, defaultLogger, nil, "", nil)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	// Should still be running (skipped due to error)
	if !o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to remain running on API error")
	}
}

func TestReconcile_PRMerge(t *testing.T) {
	sm := defaultSM

	tests := []struct {
		name              string
		prMerged          bool
		prCheckerErr      error
		updateErr         error
		prCheckerNil      bool
		expectReleased    bool
		expectUpdateCalls int
	}{
		{
			name:              "PR マージ済み: closed に更新してリリース",
			prMerged:          true,
			expectReleased:    true,
			expectUpdateCalls: 1,
		},
		{
			name:              "PR 未マージ: 処理中として維持",
			prMerged:          false,
			expectReleased:    false,
			expectUpdateCalls: 0,
		},
		{
			name:              "IsPRMerged エラー: スキップして維持",
			prCheckerErr:      fmt.Errorf("api error"),
			expectReleased:    false,
			expectUpdateCalls: 0,
		},
		{
			name:              "UpdateTaskStatus エラー: 維持",
			prMerged:          true,
			updateErr:         fmt.Errorf("update error"),
			expectReleased:    false,
			expectUpdateCalls: 1,
		},
		{
			name:              "prChecker == nil: 既存の reconciliation_release",
			prCheckerNil:      true,
			expectReleased:    true,
			expectUpdateCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskID := "task-pr-1"
			fetcher := &mockTaskClient{
				taskMap: map[string]*clickup.Task{
					taskID: {ID: taskID, Status: sm.PRReview},
				},
				updateErr: tt.updateErr,
			}
			dispatcher := &mockWorkflowDispatcher{}

			var prChecker PRChecker
			if !tt.prCheckerNil {
				prChecker = &mockPRChecker{
					merged: map[string]bool{taskID: tt.prMerged},
					err:    tt.prCheckerErr,
				}
			}

			o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", prChecker)
			o.state.Claim(taskID)
			o.state.MarkRunning(taskID)

			o.reconcile(context.Background())

			if tt.expectReleased {
				if o.state.IsClaimedOrRunning(taskID) {
					t.Errorf("expected task %s to be released", taskID)
				}
			} else {
				if !o.state.IsClaimedOrRunning(taskID) {
					t.Errorf("expected task %s to remain running", taskID)
				}
			}

			fetcher.mu.Lock()
			updateCount := len(fetcher.updateCalls)
			fetcher.mu.Unlock()
			if updateCount != tt.expectUpdateCalls {
				t.Errorf("UpdateTaskStatus called %d times, want %d", updateCount, tt.expectUpdateCalls)
			}

			if updateCount > 0 {
				fetcher.mu.Lock()
				gotStatus := fetcher.updateCalls[0].Status
				fetcher.mu.Unlock()
				if gotStatus != sm.Closed {
					t.Errorf("UpdateTaskStatus called with status %q, want %q", gotStatus, sm.Closed)
				}
			}
		})
	}
}

func TestReconcile_SpecPRMerge(t *testing.T) {
	sm := defaultSM

	tests := []struct {
		name              string
		specPRMerged      bool
		specPRCheckerErr  error
		updateErr         error
		prCheckerNil      bool
		expectReleased    bool
		expectUpdateCalls int
		expectStatus      string
	}{
		{
			name:              "SPEC PR マージ済み: ready for code に更新してリリース",
			specPRMerged:      true,
			expectReleased:    true,
			expectUpdateCalls: 1,
			expectStatus:      sm.ReadyForCode,
		},
		{
			name:              "SPEC PR 未マージ: 処理中として維持",
			specPRMerged:      false,
			expectReleased:    false,
			expectUpdateCalls: 0,
		},
		{
			name:              "IsSpecPRMerged エラー: スキップして維持",
			specPRCheckerErr:  fmt.Errorf("api error"),
			expectReleased:    false,
			expectUpdateCalls: 0,
		},
		{
			name:              "UpdateTaskStatus エラー: 維持",
			specPRMerged:      true,
			updateErr:         fmt.Errorf("update error"),
			expectReleased:    false,
			expectUpdateCalls: 1,
		},
		{
			name:              "prChecker == nil: 既存の reconciliation_release",
			prCheckerNil:      true,
			expectReleased:    true,
			expectUpdateCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskID := "task-spec-1"
			fetcher := &mockTaskClient{
				taskMap: map[string]*clickup.Task{
					taskID: {ID: taskID, Status: sm.SpecReview},
				},
				updateErr: tt.updateErr,
			}
			dispatcher := &mockWorkflowDispatcher{}

			var prChecker PRChecker
			if !tt.prCheckerNil {
				prChecker = &mockPRChecker{
					specMerged: map[string]bool{taskID: tt.specPRMerged},
					specErr:    tt.specPRCheckerErr,
				}
			}

			o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", prChecker)
			o.state.Claim(taskID)
			o.state.MarkRunning(taskID)

			o.reconcile(context.Background())

			if tt.expectReleased {
				if o.state.IsClaimedOrRunning(taskID) {
					t.Errorf("expected task %s to be released", taskID)
				}
			} else {
				if !o.state.IsClaimedOrRunning(taskID) {
					t.Errorf("expected task %s to remain running", taskID)
				}
			}

			fetcher.mu.Lock()
			updateCount := len(fetcher.updateCalls)
			fetcher.mu.Unlock()
			if updateCount != tt.expectUpdateCalls {
				t.Errorf("UpdateTaskStatus called %d times, want %d", updateCount, tt.expectUpdateCalls)
			}

			if updateCount > 0 && tt.expectStatus != "" {
				fetcher.mu.Lock()
				gotStatus := fetcher.updateCalls[0].Status
				fetcher.mu.Unlock()
				if gotStatus != tt.expectStatus {
					t.Errorf("UpdateTaskStatus called with status %q, want %q", gotStatus, tt.expectStatus)
				}
			}
		})
	}
}

func TestDispatch_NormalFlow(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	task := clickup.Task{ID: "task-1", Status: sm.ReadyForSpec}
	o.dispatch(context.Background(), task, 1)

	// Check status update
	fetcher.mu.Lock()
	if len(fetcher.updateCalls) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(fetcher.updateCalls))
	}
	if fetcher.updateCalls[0].Status != sm.GeneratingSpec {
		t.Errorf("expected status %s, got %s", sm.GeneratingSpec, fetcher.updateCalls[0].Status)
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
	if call.StatusOnSuccess != sm.SpecReview {
		t.Errorf("expected success status %s, got %s", sm.SpecReview, call.StatusOnSuccess)
	}
	if call.StatusOnError != sm.ReadyForSpec {
		t.Errorf("expected error status %s, got %s", sm.ReadyForSpec, call.StatusOnError)
	}
	if call.SpecOutput != "" {
		t.Errorf("expected empty spec output (zero value), got %s", call.SpecOutput)
	}
	dispatcher.mu.Unlock()

	// Check state
	if !o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be claimed and running")
	}
}

func TestDispatch_AlreadyClaimed(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	o.state.Claim("task-1")

	task := clickup.Task{ID: "task-1", Status: sm.ReadyForSpec}
	o.dispatch(context.Background(), task, 1)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 0 {
		t.Fatal("expected 0 trigger calls for already claimed task")
	}
}

func TestDispatch_UpdateStatusError(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		updateErr: fmt.Errorf("update error"),
		taskMap:   map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)
	defer o.shutdown()

	task := clickup.Task{ID: "task-1", Status: sm.ReadyForSpec}
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
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{
		triggerErr: fmt.Errorf("trigger error"),
	}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)
	defer o.shutdown()

	task := clickup.Task{ID: "task-1", Status: sm.ReadyForSpec}
	o.dispatch(context.Background(), task, 1)

	// Task should be released
	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released after trigger error")
	}
}

func TestDispatch_DuplicatePrevention(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	task := clickup.Task{ID: "task-1", Status: sm.ReadyForSpec}
	o.dispatch(context.Background(), task, 1)
	o.dispatch(context.Background(), task, 1) // second dispatch should be skipped

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 1 {
		t.Fatalf("expected 1 trigger call, got %d (duplicate prevention failed)", len(dispatcher.triggerCalls))
	}
}

func TestDispatch_CodePhase(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	task := clickup.Task{ID: "task-1", Status: sm.ReadyForCode}
	o.dispatch(context.Background(), task, 1)

	fetcher.mu.Lock()
	if len(fetcher.updateCalls) != 1 || fetcher.updateCalls[0].Status != sm.Implementing {
		t.Errorf("expected status update to %s", sm.Implementing)
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
	if call.StatusOnSuccess != sm.PRReview {
		t.Errorf("expected success status %s, got %s", sm.PRReview, call.StatusOnSuccess)
	}
	if call.StatusOnError != sm.ReadyForCode {
		t.Errorf("expected error status %s, got %s", sm.ReadyForCode, call.StatusOnError)
	}
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	fetcher := &mockTaskClient{
		tasks:   []clickup.Task{},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: 50 * time.Millisecond, StatusMapping: defaultSM}, defaultLogger, nil, "", nil)

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
	sm := defaultSM
	tests := []struct {
		name           string
		taskStatus     string
		wantDispatched bool
		wantReleased   bool
	}{
		{
			name:           "redispatches when task is in trigger status",
			taskStatus:     sm.ReadyForSpec,
			wantDispatched: true,
			// dispatch 内で再 Claim → MarkRunning されるため released=false
			wantReleased: false,
		},
		{
			name:           "releases when task is in non-trigger status",
			taskStatus:     sm.SpecReview,
			wantDispatched: false,
			wantReleased:   true,
		},
		{
			name:           "releases when task is in terminal status",
			taskStatus:     sm.Closed,
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
			o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)
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

func TestTick_MaxConcurrentTasksLimit(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		tasks: []clickup.Task{
			{ID: "task-1", Status: sm.ReadyForSpec},
			{ID: "task-2", Status: sm.ReadyForCode},
			{ID: "task-3", Status: sm.ReadyForSpec},
		},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	limiter := NewConcurrencyLimiter(2)
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, limiter, "", nil)

	// 外部で1スロット消費済み（別プロジェクトのタスク想定）
	limiter.TryAcquire()

	// 残り1スロットなので1件だけディスパッチできる
	o.tick(context.Background())

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 1 {
		t.Fatalf("expected 1 trigger call (limit=2, 1 already acquired), got %d", len(dispatcher.triggerCalls))
	}
}

func TestTick_NilLimiterIsUnlimited(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		tasks: []clickup.Task{
			{ID: "task-1", Status: sm.ReadyForSpec},
			{ID: "task-2", Status: sm.ReadyForCode},
			{ID: "task-3", Status: sm.ReadyForSpec},
		},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	o.tick(context.Background())

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 3 {
		t.Fatalf("expected 3 trigger calls (unlimited), got %d", len(dispatcher.triggerCalls))
	}
}

func TestReconcile_LimiterRelease(t *testing.T) {
	sm := defaultSM
	tests := []struct {
		name            string
		taskStatus      string
		wantStateKept   bool
		wantLimiterKept bool
	}{
		{
			name:            "terminal status releases limiter",
			taskStatus:      sm.Closed,
			wantStateKept:   false,
			wantLimiterKept: false,
		},
		{
			name:            "non-processing status releases limiter",
			taskStatus:      sm.SpecReview,
			wantStateKept:   false,
			wantLimiterKept: false,
		},
		{
			name:            "trigger status releases limiter",
			taskStatus:      sm.ReadyForSpec,
			wantStateKept:   false,
			wantLimiterKept: false,
		},
		{
			name:            "processing status keeps limiter",
			taskStatus:      sm.GeneratingSpec,
			wantStateKept:   true,
			wantLimiterKept: true,
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
			limiter := NewConcurrencyLimiter(3)
			o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, limiter, "", nil)

			o.state.Claim("task-1")
			_ = limiter.TryAcquire()
			o.state.MarkRunning("task-1")

			o.reconcile(context.Background())

			if o.state.IsClaimedOrRunning("task-1") != tt.wantStateKept {
				t.Errorf("state kept = %v, want %v", o.state.IsClaimedOrRunning("task-1"), tt.wantStateKept)
			}
			if (limiter.ActiveCount() == 1) != tt.wantLimiterKept {
				t.Errorf("limiter active = %d, wantLimiterKept = %v", limiter.ActiveCount(), tt.wantLimiterKept)
			}
		})
	}
}

func TestReconcile_APIError_KeepsLimiter(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		getTaskErr: fmt.Errorf("api error"),
		taskMap:    map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	limiter := NewConcurrencyLimiter(3)
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, limiter, "", nil)

	o.state.Claim("task-1")
	_ = limiter.TryAcquire()
	o.state.MarkRunning("task-1")

	o.reconcile(context.Background())

	if !o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to remain running on API error")
	}
	if limiter.ActiveCount() != 1 {
		t.Fatalf("expected limiter active=1, got %d", limiter.ActiveCount())
	}
}

func TestReconcile_MultipleRunning_PartialRelease(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: sm.Closed},
			"task-2": {ID: "task-2", Status: sm.GeneratingSpec},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	limiter := NewConcurrencyLimiter(3)
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, limiter, "", nil)

	o.state.Claim("task-1")
	_ = limiter.TryAcquire()
	o.state.MarkRunning("task-1")

	o.state.Claim("task-2")
	_ = limiter.TryAcquire()
	o.state.MarkRunning("task-2")

	o.reconcile(context.Background())

	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released (terminal status)")
	}
	if !o.state.IsClaimedOrRunning("task-2") {
		t.Fatal("expected task-2 to remain running (processing status)")
	}
	if limiter.ActiveCount() != 1 {
		t.Fatalf("expected limiter active=1, got %d", limiter.ActiveCount())
	}
}

func TestDispatchReconcileRedispatch_LimiterCycle(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: sm.Closed},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	limiter := NewConcurrencyLimiter(1)
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, limiter, "", nil)

	// Step 1: dispatch task-1
	task1 := clickup.Task{ID: "task-1", Status: sm.ReadyForSpec}
	o.dispatch(context.Background(), task1, 1)

	if limiter.ActiveCount() != 1 {
		t.Fatalf("expected limiter active=1 after dispatch, got %d", limiter.ActiveCount())
	}
	if !o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be running after dispatch")
	}

	// Step 2: task-1 reaches terminal status → reconcile releases slot
	o.reconcile(context.Background())

	if limiter.ActiveCount() != 0 {
		t.Fatalf("expected limiter active=0 after reconcile, got %d", limiter.ActiveCount())
	}
	if o.state.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task-1 to be released after reconcile")
	}

	// Step 3: re-dispatch task-1 (back to trigger status)
	fetcher.mu.Lock()
	fetcher.taskMap["task-1"] = &clickup.Task{ID: "task-1", Status: sm.ReadyForSpec}
	fetcher.mu.Unlock()

	o.dispatch(context.Background(), task1, 1)

	dispatcher.mu.Lock()
	calls := len(dispatcher.triggerCalls)
	dispatcher.mu.Unlock()

	if calls != 2 {
		t.Fatalf("expected TriggerWorkflow called 2 times total, got %d", calls)
	}
	if limiter.ActiveCount() != 1 {
		t.Fatalf("expected limiter active=1 after re-dispatch, got %d", limiter.ActiveCount())
	}
}

func TestDispatchReconcileRedispatch_MaxConcurrencyRespected(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: sm.Closed},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	limiter := NewConcurrencyLimiter(1)
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, limiter, "", nil)

	// Setup: task-1 is already running (consuming the only slot)
	o.state.Claim("task-1")
	_ = limiter.TryAcquire()
	o.state.MarkRunning("task-1")

	// Step 1: try to dispatch task-2 → limiter is full, should fail
	task2 := clickup.Task{ID: "task-2", Status: sm.ReadyForSpec}
	result := o.dispatch(context.Background(), task2, 1)

	if result {
		// dispatch returns false when limiter is full (caller should stop)
		t.Fatal("expected dispatch to return false when limiter is full")
	}

	dispatcher.mu.Lock()
	calls := len(dispatcher.triggerCalls)
	dispatcher.mu.Unlock()

	if calls != 0 {
		t.Fatalf("expected 0 trigger calls when limiter full, got %d", calls)
	}

	// Step 2: task-1 reaches terminal status → reconcile releases slot
	o.reconcile(context.Background())

	if limiter.ActiveCount() != 0 {
		t.Fatalf("expected limiter active=0 after reconcile, got %d", limiter.ActiveCount())
	}

	// Step 3: now task-2 can be dispatched
	o.dispatch(context.Background(), task2, 1)

	dispatcher.mu.Lock()
	calls = len(dispatcher.triggerCalls)
	dispatcher.mu.Unlock()

	if calls != 1 {
		t.Fatalf("expected 1 trigger call after slot freed, got %d", calls)
	}
	if limiter.ActiveCount() != 1 {
		t.Fatalf("expected limiter active=1 after task-2 dispatch, got %d", limiter.ActiveCount())
	}
}

func TestScheduleRetry_CancelsExistingTimer(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{
		taskMap: map[string]*clickup.Task{
			"task-1": {ID: "task-1", Status: sm.ReadyForSpec},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)
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

	// handleRetry は attempt 不一致時に何もしないことを検証
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

func TestRecoverProcessingTasks(t *testing.T) {
	sm := defaultSM
	tests := []struct {
		name            string
		tasks           []clickup.Task
		getTasksErr     error
		updateErr       error
		wantUpdateCalls []updateCall
	}{
		{
			name:  "generating spec タスクを ready for spec に巻き戻す",
			tasks: []clickup.Task{{ID: "task-1", Status: sm.GeneratingSpec}},
			wantUpdateCalls: []updateCall{
				{TaskID: "task-1", Status: sm.ReadyForSpec},
			},
		},
		{
			name:  "implementing タスクを ready for code に巻き戻す",
			tasks: []clickup.Task{{ID: "task-1", Status: sm.Implementing}},
			wantUpdateCalls: []updateCall{
				{TaskID: "task-1", Status: sm.ReadyForCode},
			},
		},
		{
			name: "複数の processing タスクをまとめて復旧する",
			tasks: []clickup.Task{
				{ID: "task-1", Status: sm.GeneratingSpec},
				{ID: "task-2", Status: sm.Implementing},
				{ID: "task-3", Status: sm.ReadyForSpec},
				{ID: "task-4", Status: sm.Closed},
			},
			wantUpdateCalls: []updateCall{
				{TaskID: "task-1", Status: sm.ReadyForSpec},
				{TaskID: "task-2", Status: sm.ReadyForCode},
			},
		},
		{
			name:            "GetTasks エラー時は UpdateTaskStatus を呼ばない",
			tasks:           nil,
			getTasksErr:     fmt.Errorf("api error"),
			wantUpdateCalls: nil,
		},
		{
			name: "UpdateTaskStatus エラーは継続する（全タスクに試みる）",
			tasks: []clickup.Task{
				{ID: "task-1", Status: sm.GeneratingSpec},
				{ID: "task-2", Status: sm.Implementing},
			},
			updateErr: fmt.Errorf("update error"),
			wantUpdateCalls: []updateCall{
				{TaskID: "task-1", Status: sm.ReadyForSpec},
				{TaskID: "task-2", Status: sm.ReadyForCode},
			},
		},
		{
			name: "processing タスクがない場合は UpdateTaskStatus を呼ばない",
			tasks: []clickup.Task{
				{ID: "task-1", Status: sm.ReadyForSpec},
				{ID: "task-2", Status: sm.SpecReview},
			},
			wantUpdateCalls: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := &mockTaskClient{
				tasks:       tt.tasks,
				taskMap:     map[string]*clickup.Task{},
				getTasksErr: tt.getTasksErr,
				updateErr:   tt.updateErr,
			}
			dispatcher := &mockWorkflowDispatcher{}
			o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

			o.recoverProcessingTasks(context.Background())

			fetcher.mu.Lock()
			defer fetcher.mu.Unlock()

			if len(fetcher.updateCalls) != len(tt.wantUpdateCalls) {
				t.Fatalf("expected %d update calls, got %d: %v", len(tt.wantUpdateCalls), len(fetcher.updateCalls), fetcher.updateCalls)
			}
			for i, want := range tt.wantUpdateCalls {
				got := fetcher.updateCalls[i]
				if got.TaskID != want.TaskID || got.Status != want.Status {
					t.Errorf("update call[%d] = %v, want %v", i, got, want)
				}
			}
		})
	}
}

// sequentialTaskClient は GetTasks の呼び出し回数に応じて異なるタスクリストを返すモック
type sequentialTaskClient struct {
	mockTaskClient
	callIndex int
	taskSets  [][]clickup.Task
}

func (s *sequentialTaskClient) GetTasks(_ context.Context) ([]clickup.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getTasksCalls++
	if s.getTasksErr != nil {
		return nil, s.getTasksErr
	}
	if s.callIndex < len(s.taskSets) {
		tasks := s.taskSets[s.callIndex]
		s.callIndex++
		return tasks, nil
	}
	return []clickup.Task{}, nil
}

func TestRun_RecoveryCalledBeforeFirstTick(t *testing.T) {
	sm := defaultSM

	fetcher := &sequentialTaskClient{
		mockTaskClient: mockTaskClient{taskMap: map[string]*clickup.Task{}},
		taskSets: [][]clickup.Task{
			{{ID: "task-1", Status: sm.GeneratingSpec}},
			{{ID: "task-1", Status: sm.ReadyForSpec}},
		},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Hour, StatusMapping: sm}, defaultLogger, nil, "", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.ctx = ctx

	o.recoverProcessingTasks(ctx)

	fetcher.mu.Lock()
	if len(fetcher.updateCalls) != 1 {
		fetcher.mu.Unlock()
		t.Fatalf("expected 1 update call after recovery, got %d", len(fetcher.updateCalls))
	}
	if fetcher.updateCalls[0] != (updateCall{TaskID: "task-1", Status: sm.ReadyForSpec}) {
		fetcher.mu.Unlock()
		t.Errorf("unexpected update call: %v", fetcher.updateCalls[0])
	}
	fetcher.mu.Unlock()

	o.tick(ctx)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 1 {
		t.Fatalf("expected 1 trigger call after tick, got %d", len(dispatcher.triggerCalls))
	}
	if dispatcher.triggerCalls[0].TaskID != "task-1" {
		t.Errorf("expected trigger for task-1, got %s", dispatcher.triggerCalls[0].TaskID)
	}
	if dispatcher.triggerCalls[0].Phase != string(clickup.PhaseSpec) {
		t.Errorf("expected phase SPEC, got %s", dispatcher.triggerCalls[0].Phase)
	}
}

func TestShutdown_NoActiveDispatch(t *testing.T) {
	fetcher := &mockTaskClient{taskMap: map[string]*clickup.Task{}}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{
		PollInterval:    time.Second,
		StatusMapping:   defaultSM,
		ShutdownTimeout: 30 * time.Second,
	}, defaultLogger, nil, "", nil)

	done := make(chan struct{})
	go func() {
		o.shutdown()
		close(done)
	}()

	select {
	case <-done:
		// OK: no active dispatch, should return immediately
	case <-time.After(time.Second):
		t.Fatal("shutdown did not complete promptly when no dispatch active")
	}
}

func TestShutdown_WaitsForDispatch(t *testing.T) {
	sm := defaultSM
	dispatchDelay := 100 * time.Millisecond
	fetcher := &mockTaskClient{
		taskMap:     map[string]*clickup.Task{},
		updateDelay: dispatchDelay,
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{
		PollInterval:    time.Second,
		StatusMapping:   sm,
		ShutdownTimeout: 5 * time.Second,
	}, defaultLogger, nil, "", nil)

	task := clickup.Task{ID: "task-1", Status: sm.ReadyForSpec}

	dispatchDone := make(chan struct{})
	go func() {
		o.dispatch(context.Background(), task, 1)
		close(dispatchDone)
	}()

	// Give dispatch time to start
	time.Sleep(10 * time.Millisecond)

	shutdownDone := make(chan struct{})
	go func() {
		o.shutdown()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not complete after dispatch finished")
	}

	// Verify dispatch completed (UpdateTaskStatus was called)
	fetcher.mu.Lock()
	updateCount := len(fetcher.updateCalls)
	fetcher.mu.Unlock()
	if updateCount == 0 {
		t.Fatal("expected UpdateTaskStatus to have been called before shutdown completed")
	}
}

func TestShutdown_TimeoutForcesStop(t *testing.T) {
	fetcher := &mockTaskClient{taskMap: map[string]*clickup.Task{}}
	dispatcher := &mockWorkflowDispatcher{}
	shutdownTimeout := 100 * time.Millisecond
	o := New(fetcher, dispatcher, Config{
		PollInterval:    time.Second,
		StatusMapping:   defaultSM,
		ShutdownTimeout: shutdownTimeout,
	}, defaultLogger, nil, "", nil)

	// dispatchWg を直接操作して「実行中の dispatch がある」状態をシミュレート
	o.dispatchWg.Add(1)
	defer o.dispatchWg.Done()

	start := time.Now()
	o.shutdown()
	elapsed := time.Since(start)

	if elapsed > shutdownTimeout*3 {
		t.Errorf("shutdown took %v, expected around %v", elapsed, shutdownTimeout)
	}
	if elapsed < shutdownTimeout/2 {
		t.Errorf("shutdown returned too early (%v), expected to wait at least ~%v", elapsed, shutdownTimeout)
	}
}

func TestStatus_RunningTasksAfterMarkRunning(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{taskMap: map[string]*clickup.Task{}}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "owner/repo", nil)

	o.state.Claim("task-1")
	o.state.MarkRunning("task-1")

	s := o.Status()
	if s.Project != "owner/repo" {
		t.Errorf("project = %q, want owner/repo", s.Project)
	}
	found := false
	for _, rt := range s.RunningTasks {
		if rt.TaskID == "task-1" {
			found = true
		}
	}
	if !found {
		t.Error("expected task-1 in RunningTasks after MarkRunning")
	}
}

func TestStatus_RetryPendingAfterScheduleRetry(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{taskMap: map[string]*clickup.Task{}}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	o.scheduleRetry("task-2", "CODE", 2, fmt.Errorf("trigger failed"))

	s := o.Status()
	if len(s.RetryPending) != 1 {
		t.Fatalf("retry_pending len = %d, want 1", len(s.RetryPending))
	}
	rp := s.RetryPending[0]
	if rp.TaskID != "task-2" {
		t.Errorf("task_id = %q, want task-2", rp.TaskID)
	}
	if rp.Phase != "CODE" {
		t.Errorf("phase = %q, want CODE", rp.Phase)
	}
	if rp.Attempt != 2 {
		t.Errorf("attempt = %d, want 2", rp.Attempt)
	}
	if rp.RetryAfter.IsZero() {
		t.Error("retry_after should not be zero")
	}
}

func TestStatus_RetryPendingEmptyAfterShutdown(t *testing.T) {
	sm := defaultSM
	fetcher := &mockTaskClient{taskMap: map[string]*clickup.Task{}}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{PollInterval: time.Second, StatusMapping: sm}, defaultLogger, nil, "", nil)

	o.scheduleRetry("task-3", "SPEC", 1, fmt.Errorf("error"))
	o.shutdown()

	s := o.Status()
	if len(s.RetryPending) != 0 {
		t.Errorf("retry_pending len = %d, want 0 after shutdown", len(s.RetryPending))
	}
}

// TestRun_RecoveredTaskNotRedispatchedOnFirstTick verifies that a task still in processing
// status after recovery is not dispatched on the first tick (processing ≠ trigger status).
func TestRun_RecoveredTaskNotRedispatchedOnFirstTick(t *testing.T) {
	sm := defaultSM
	// Mock always returns GeneratingSpec — simulating the case where the ClickUp board
	// still reflects the processing status during the first tick after recovery.
	fetcher := &mockTaskClient{
		tasks: []clickup.Task{
			{ID: "task-1", Status: sm.GeneratingSpec},
		},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{
		PollInterval:  time.Hour,
		StatusMapping: sm,
	}, defaultLogger, nil, "", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.ctx = ctx

	o.recoverProcessingTasks(ctx)
	o.tick(ctx)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if len(dispatcher.triggerCalls) != 0 {
		t.Errorf("expected 0 TriggerWorkflow calls for task in processing status, got %d", len(dispatcher.triggerCalls))
	}
}

// TestShutdown_TimesOutWithoutStatusUpdate verifies that when shutdown times out while a
// dispatch is in-flight, the error-status update has not yet been applied (dispatch abandoned).
func TestShutdown_TimesOutWithoutStatusUpdate(t *testing.T) {
	sm := defaultSM
	const dispatchDelay = 200 * time.Millisecond
	const shutdownTimeout = 10 * time.Millisecond

	updateStarted := make(chan struct{})
	fetcher := &mockTaskClient{
		taskMap:       map[string]*clickup.Task{},
		updateDelay:   dispatchDelay,
		updateStarted: updateStarted,
	}
	dispatcher := &mockWorkflowDispatcher{}
	o := New(fetcher, dispatcher, Config{
		PollInterval:    time.Hour,
		StatusMapping:   sm,
		ShutdownTimeout: shutdownTimeout,
	}, defaultLogger, nil, "", nil)

	task := clickup.Task{ID: "task-1", Status: sm.ReadyForSpec}

	go o.dispatch(context.Background(), task, 1)
	<-updateStarted // wait until dispatch reaches UpdateTaskStatus

	start := time.Now()
	o.shutdown()
	elapsed := time.Since(start)

	if elapsed > shutdownTimeout*5 {
		t.Errorf("shutdown took %v, expected ~%v", elapsed, shutdownTimeout)
	}

	// After timeout, no UpdateTaskStatus call should have completed yet
	// (the dispatch goroutine is still waiting in its updateDelay)
	fetcher.mu.Lock()
	calls := len(fetcher.updateCalls)
	fetcher.mu.Unlock()

	if calls != 0 {
		t.Errorf("expected 0 update calls before dispatch delay expired, got %d", calls)
	}
}

// TestMultiProjectSharedLimiter verifies that two orchestrators sharing a ConcurrencyLimiter
// with maxConcurrent=2 dispatch at most 2 tasks combined, even with 4 ready tasks total.
func TestMultiProjectSharedLimiter(t *testing.T) {
	sm := defaultSM
	const maxConcurrent = 2

	limiter := NewConcurrencyLimiter(maxConcurrent)

	// Project A: 2 ready tasks
	fetcherA := &mockTaskClient{
		tasks: []clickup.Task{
			{ID: "task-a1", Status: sm.ReadyForSpec},
			{ID: "task-a2", Status: sm.ReadyForSpec},
		},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcherA := &mockWorkflowDispatcher{}
	orchA := New(fetcherA, dispatcherA, Config{
		PollInterval:  time.Hour,
		StatusMapping: sm,
	}, defaultLogger, limiter, "org/repo-a", nil)

	// Project B: 2 ready tasks
	fetcherB := &mockTaskClient{
		tasks: []clickup.Task{
			{ID: "task-b1", Status: sm.ReadyForSpec},
			{ID: "task-b2", Status: sm.ReadyForSpec},
		},
		taskMap: map[string]*clickup.Task{},
	}
	dispatcherB := &mockWorkflowDispatcher{}
	orchB := New(fetcherB, dispatcherB, Config{
		PollInterval:  time.Hour,
		StatusMapping: sm,
	}, defaultLogger, limiter, "org/repo-b", nil)

	// Start gate: both orchestrators tick simultaneously
	start := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		orchA.tick(context.Background())
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		orchB.tick(context.Background())
	}()

	close(start)
	wg.Wait()

	dispatcherA.mu.Lock()
	callsA := len(dispatcherA.triggerCalls)
	dispatcherA.mu.Unlock()

	dispatcherB.mu.Lock()
	callsB := len(dispatcherB.triggerCalls)
	dispatcherB.mu.Unlock()

	total := callsA + callsB
	if total != maxConcurrent {
		t.Errorf("expected %d total trigger calls with shared limiter, got %d (A=%d, B=%d)",
			maxConcurrent, total, callsA, callsB)
	}
	// release() is deferred until reconcile(), so slots remain active after tick completes.
	if got := limiter.ActiveCount(); got != maxConcurrent {
		t.Errorf("expected limiter active=%d, got %d", maxConcurrent, got)
	}
}

func TestDispatch_SpecOutputPropagated(t *testing.T) {
	tests := []struct {
		name           string
		specOutput     string
		wantSpecOutput string
	}{
		{
			name:           "repo mode propagated",
			specOutput:     "repo",
			wantSpecOutput: "repo",
		},
		{
			name:           "clickup mode propagated",
			specOutput:     "clickup",
			wantSpecOutput: "clickup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := defaultSM
			fetcher := &mockTaskClient{
				tasks: []clickup.Task{
					{ID: "task-1", Status: sm.ReadyForSpec},
				},
				taskMap: map[string]*clickup.Task{},
			}
			dispatcher := &mockWorkflowDispatcher{}
			o := New(fetcher, dispatcher, Config{
				PollInterval:  time.Second,
				StatusMapping: sm,
				SpecOutput:    tt.specOutput,
			}, defaultLogger, nil, "", nil)

			o.tick(context.Background())

			dispatcher.mu.Lock()
			calls := dispatcher.triggerCalls
			dispatcher.mu.Unlock()

			if len(calls) != 1 {
				t.Fatalf("expected 1 trigger call, got %d", len(calls))
			}
			if calls[0].SpecOutput != tt.wantSpecOutput {
				t.Errorf("SpecOutput = %q, want %q", calls[0].SpecOutput, tt.wantSpecOutput)
			}
		})
	}
}
