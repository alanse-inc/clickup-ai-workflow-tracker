package orchestrator

import (
	"sync"
	"time"
)

// AgentState はタスクのインメモリ状態を管理する
type AgentState struct {
	mu           sync.RWMutex
	runningTasks map[string]time.Time // Key: ClickUp Task ID, Value: started_at
	claimed      map[string]struct{}  // Set of claimed task IDs
}

// NewAgentState は新しい AgentState を返す
func NewAgentState() *AgentState {
	return &AgentState{
		runningTasks: make(map[string]time.Time),
		claimed:      make(map[string]struct{}),
	}
}

// Claim はタスクをクレームする。既にクレーム済みなら false を返す
func (s *AgentState) Claim(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.claimed[taskID]; ok {
		return false
	}
	s.claimed[taskID] = struct{}{}
	return true
}

// MarkRunning はタスクを実行中としてマークする
func (s *AgentState) MarkRunning(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.runningTasks[taskID] = time.Now()
}

// Release はタスクのクレームと実行中状態を解除する
func (s *AgentState) Release(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.claimed, taskID)
	delete(s.runningTasks, taskID)
}

// IsClaimedOrRunning はタスクがクレーム済みまたは実行中かどうかを返す
func (s *AgentState) IsClaimedOrRunning(taskID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.claimed[taskID]; ok {
		return true
	}
	if _, ok := s.runningTasks[taskID]; ok {
		return true
	}
	return false
}

// RunningTaskIDs は実行中のタスクIDリストを返す
func (s *AgentState) RunningTaskIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.runningTasks))
	for id := range s.runningTasks {
		ids = append(ids, id)
	}
	return ids
}

// RunningTasksSnapshot は running tasks の map コピーを返す
func (s *AgentState) RunningTasksSnapshot() map[string]time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := make(map[string]time.Time, len(s.runningTasks))
	for id, t := range s.runningTasks {
		snapshot[id] = t
	}
	return snapshot
}
