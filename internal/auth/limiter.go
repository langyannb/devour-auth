package auth

import (
	"sync"
	"time"
)

// Limiter 进程内滑动窗口计数：登录按 IP、按卡密分开计数（design.md 16.4）。
//
// shortcut: 只对单进程有效；多实例部署要换共享存储，I1 不涉及。
type Limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
	now  func() time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{hits: make(map[string][]time.Time), now: time.Now}
}

// Allow 在 window 内最多放行 limit 次。
func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = make(map[string][]time.Time)
	}
	now := l.now()
	cut := now.Add(-window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

// Reset 清空某个 key 的计数，登录成功后调用。
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
