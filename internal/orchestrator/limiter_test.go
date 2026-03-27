package orchestrator

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrencyLimiter_Basic(t *testing.T) {
	l := NewConcurrencyLimiter(2)

	if !l.TryAcquire() {
		t.Fatal("expected first acquire to succeed")
	}
	if !l.TryAcquire() {
		t.Fatal("expected second acquire to succeed")
	}
	if l.TryAcquire() {
		t.Fatal("expected third acquire to fail (limit reached)")
	}

	if l.ActiveCount() != 2 {
		t.Fatalf("expected active count 2, got %d", l.ActiveCount())
	}

	l.Release()
	if l.ActiveCount() != 1 {
		t.Fatalf("expected active count 1 after release, got %d", l.ActiveCount())
	}

	if !l.TryAcquire() {
		t.Fatal("expected acquire to succeed after release")
	}
}

func TestConcurrencyLimiter_Unlimited(t *testing.T) {
	l := NewConcurrencyLimiter(0)
	for range 100 {
		if !l.TryAcquire() {
			t.Fatal("expected acquire to always succeed with no limit")
		}
	}
}

func TestConcurrencyLimiter_Nil(t *testing.T) {
	var l *ConcurrencyLimiter
	// nil limiter should not panic and always allow
	if !l.TryAcquire() {
		t.Fatal("expected nil limiter TryAcquire to return true")
	}
	l.Release() // should not panic
	if l.ActiveCount() != 0 {
		t.Fatal("expected nil limiter ActiveCount to return 0")
	}
}

func TestConcurrencyLimiter_ConcurrentRace(t *testing.T) {
	l := NewConcurrencyLimiter(1)
	const goroutines = 100

	var wg sync.WaitGroup
	var acquired atomic.Int32

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if l.TryAcquire() {
				acquired.Add(1)
			}
		}()
	}
	wg.Wait()

	if acquired.Load() != 1 {
		t.Fatalf("expected exactly 1 acquire with limit=1, got %d", acquired.Load())
	}
}

func TestConcurrencyLimiter_ReleaseDoesNotGoNegative(t *testing.T) {
	l := NewConcurrencyLimiter(1)
	l.Release() // release without acquire
	if l.ActiveCount() != 0 {
		t.Fatalf("expected active count 0, got %d", l.ActiveCount())
	}
}

func TestConcurrencyLimiter_MaxConcurrent(t *testing.T) {
	tests := []struct {
		name    string
		limiter *ConcurrencyLimiter
		want    int
	}{
		{
			name:    "positive limit",
			limiter: NewConcurrencyLimiter(5),
			want:    5,
		},
		{
			name:    "zero unlimited",
			limiter: NewConcurrencyLimiter(0),
			want:    0,
		},
		{
			name:    "nil limiter",
			limiter: nil,
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.limiter.MaxConcurrent()
			if got != tt.want {
				t.Errorf("MaxConcurrent() = %d, want %d", got, tt.want)
			}
		})
	}
}
