package orchestrator

import "sync"

// ConcurrencyLimiter はプロジェクトごとの並行タスク数カウンタ。
// maxConcurrent が 0 の場合は上限なし。
type ConcurrencyLimiter struct {
	mu            sync.Mutex
	active        int
	maxConcurrent int
}

// NewConcurrencyLimiter は新しい ConcurrencyLimiter を返す。
// maxConcurrent が 0 の場合は上限なし。
func NewConcurrencyLimiter(maxConcurrent int) *ConcurrencyLimiter {
	return &ConcurrencyLimiter{maxConcurrent: maxConcurrent}
}

// TryAcquire は上限に達していなければカウントを増やし true を返す。
// 上限に達している場合は false を返す。上限なし（0）の場合は常に true。
func (l *ConcurrencyLimiter) TryAcquire() bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.maxConcurrent > 0 && l.active >= l.maxConcurrent {
		return false
	}
	l.active++
	return true
}

// Release はカウントを1つ減らす
func (l *ConcurrencyLimiter) Release() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.active > 0 {
		l.active--
	}
}

// MaxConcurrent は最大並行数を返す。0 は無制限を示す。
func (l *ConcurrencyLimiter) MaxConcurrent() int {
	if l == nil {
		return 0
	}
	return l.maxConcurrent // immutable、ロック不要
}

// ActiveCount は現在のアクティブ数を返す
func (l *ConcurrencyLimiter) ActiveCount() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}
