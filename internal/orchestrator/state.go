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

// ClaimIfUnderLimit は上限チェックとクレームを原子的に行う。
// maxConcurrent が 0 の場合は上限なし。既にクレーム済みまたは上限到達なら false を返す。
func (s *AgentState) ClaimIfUnderLimit(taskID string, maxConcurrent int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.claimed[taskID]; ok {
		return false
	}

	if maxConcurrent > 0 {
		count := len(s.claimed)
		for id := range s.runningTasks {
			if _, ok := s.claimed[id]; !ok {
				count++
			}
		}
		if count >= maxConcurrent {
			return false
		}
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

// ActiveCount はクレーム済みまたは実行中のタスク数を返す
func (s *AgentState) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// claimed をベースにして runningTasks のうち claimed に無いものだけ加算
	count := len(s.claimed)
	for id := range s.runningTasks {
		if _, ok := s.claimed[id]; !ok {
			count++
		}
	}
	return count
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
