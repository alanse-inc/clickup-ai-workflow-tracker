package orchestrator

import (
	"sort"
	"sync"
	"testing"
)

func TestClaim_Success(t *testing.T) {
	s := NewAgentState()
	if !s.Claim("task-1") {
		t.Fatal("expected Claim to return true for new task")
	}
}

func TestClaim_Duplicate(t *testing.T) {
	s := NewAgentState()
	s.Claim("task-1")
	if s.Claim("task-1") {
		t.Fatal("expected Claim to return false for already claimed task")
	}
}

func TestMarkRunning_And_RunningTaskIDs(t *testing.T) {
	s := NewAgentState()
	s.Claim("task-1")
	s.MarkRunning("task-1")
	s.Claim("task-2")
	s.MarkRunning("task-2")

	ids := s.RunningTaskIDs()
	sort.Strings(ids)
	if len(ids) != 2 {
		t.Fatalf("expected 2 running tasks, got %d", len(ids))
	}
	if ids[0] != "task-1" || ids[1] != "task-2" {
		t.Fatalf("unexpected running task IDs: %v", ids)
	}
}

func TestRelease(t *testing.T) {
	s := NewAgentState()
	s.Claim("task-1")
	s.MarkRunning("task-1")

	s.Release("task-1")

	if s.IsClaimedOrRunning("task-1") {
		t.Fatal("expected task to not be claimed or running after release")
	}

	ids := s.RunningTaskIDs()
	if len(ids) != 0 {
		t.Fatalf("expected 0 running tasks after release, got %d", len(ids))
	}

	// Re-claim should succeed after release
	if !s.Claim("task-1") {
		t.Fatal("expected Claim to succeed after release")
	}
}

func TestIsClaimedOrRunning(t *testing.T) {
	s := NewAgentState()

	if s.IsClaimedOrRunning("task-1") {
		t.Fatal("expected false for unknown task")
	}

	s.Claim("task-1")
	if !s.IsClaimedOrRunning("task-1") {
		t.Fatal("expected true for claimed task")
	}

	s.MarkRunning("task-1")
	if !s.IsClaimedOrRunning("task-1") {
		t.Fatal("expected true for running task")
	}
}

func TestRunningTaskIDs_Empty(t *testing.T) {
	s := NewAgentState()
	ids := s.RunningTaskIDs()
	if len(ids) != 0 {
		t.Fatalf("expected 0 running tasks, got %d", len(ids))
	}
}

func TestActiveCount(t *testing.T) {
	s := NewAgentState()

	if s.ActiveCount() != 0 {
		t.Fatalf("expected 0 active count initially, got %d", s.ActiveCount())
	}

	// Claim は claimed に追加される
	s.Claim("task-1")
	if s.ActiveCount() != 1 {
		t.Fatalf("expected 1 after claim, got %d", s.ActiveCount())
	}

	// MarkRunning は runningTasks に追加するが claimed は残るため、重複カウントしない
	s.MarkRunning("task-1")
	if s.ActiveCount() != 1 {
		t.Fatalf("expected 1 after mark running, got %d", s.ActiveCount())
	}

	// 別タスクを Claim
	s.Claim("task-2")
	if s.ActiveCount() != 2 {
		t.Fatalf("expected 2 after second claim, got %d", s.ActiveCount())
	}

	s.Release("task-1")
	if s.ActiveCount() != 1 {
		t.Fatalf("expected 1 after release, got %d", s.ActiveCount())
	}

	s.Release("task-2")
	if s.ActiveCount() != 0 {
		t.Fatalf("expected 0 after all released, got %d", s.ActiveCount())
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewAgentState()
	const goroutines = 100

	var wg sync.WaitGroup
	claimedCount := 0
	var mu sync.Mutex

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if s.Claim("task-1") {
				mu.Lock()
				claimedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if claimedCount != 1 {
		t.Fatalf("expected exactly 1 successful claim, got %d", claimedCount)
	}

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			taskID := "concurrent-task"
			s.Claim(taskID)
			s.MarkRunning(taskID)
			s.IsClaimedOrRunning(taskID)
			s.RunningTaskIDs()
			s.Release(taskID)
		}()
	}
	wg.Wait()
}
