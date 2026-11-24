package auth

import (
	"sync"
	"time"
)

// LoginLimiter 登录失败限流（B-05）：按 IP 与用户名双维度滑动窗口计数，
// 超过阈值后在锁定时长内直接拒绝，避免在线爆破。
//
// 设计要点：
//   - 只用内存，重启即清零（家庭/宿舍场景足够，且不引入额外依赖）；
//   - IP 维度防止同一来源换用户名爆破，用户名维度防止换 IP 打同一账号；
//   - 任一维度达阈值即锁定，成功登录后两个维度都清零。
type LoginLimiter struct {
	maxFailures int
	window      time.Duration
	lockFor     time.Duration

	mu     sync.Mutex
	byIP   map[string]*attemptBucket
	byUser map[string]*attemptBucket

	now func() time.Time
}

type attemptBucket struct {
	failures    []time.Time
	lockedUntil time.Time
}

// NewLoginLimiter 构造限流器；maxFailures <= 0 时使用默认 5 次 / 5 分钟 / 锁定 5 分钟。
func NewLoginLimiter(maxFailures, windowMinutes, lockMinutes int) *LoginLimiter {
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if windowMinutes <= 0 {
		windowMinutes = 5
	}
	if lockMinutes <= 0 {
		lockMinutes = 5
	}
	return &LoginLimiter{
		maxFailures: maxFailures,
		window:      time.Duration(windowMinutes) * time.Minute,
		lockFor:     time.Duration(lockMinutes) * time.Minute,
		byIP:        map[string]*attemptBucket{},
		byUser:      map[string]*attemptBucket{},
		now:         time.Now,
	}
}

// Allow 判断当前是否允许尝试登录；不允许时返回剩余锁定时间。
func (l *LoginLimiter) Allow(ip, username string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if d, locked := l.lockedFor(l.byIP[ip], now); locked {
		return false, d
	}
	if d, locked := l.lockedFor(l.byUser[username], now); locked {
		return false, d
	}
	return true, 0
}

// Fail 记录一次失败；达到阈值时进入锁定并返回是否刚刚触发锁定。
func (l *LoginLimiter) Fail(ip, username string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	ipLocked := l.record(l.byIP, ip, now)
	userLocked := l.record(l.byUser, username, now)
	return ipLocked || userLocked
}

// Reset 登录成功后清零计数。
func (l *LoginLimiter) Reset(ip, username string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byIP, ip)
	delete(l.byUser, username)
}

// Cleanup 清理过期条目，避免长期运行内存增长。
func (l *LoginLimiter) Cleanup() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	prune := func(m map[string]*attemptBucket) {
		for k, b := range m {
			if b == nil {
				delete(m, k)
				continue
			}
			if now.Before(b.lockedUntil) {
				continue
			}
			b.failures = filterRecent(b.failures, now.Add(-l.window))
			if len(b.failures) == 0 {
				delete(m, k)
			}
		}
	}
	prune(l.byIP)
	prune(l.byUser)
}

// lockedFor 返回该桶的剩余锁定时间。
func (l *LoginLimiter) lockedFor(b *attemptBucket, now time.Time) (time.Duration, bool) {
	if b == nil || b.lockedUntil.IsZero() {
		return 0, false
	}
	if now.Before(b.lockedUntil) {
		return b.lockedUntil.Sub(now), true
	}
	return 0, false
}

// record 记录失败并在达到阈值时设置锁定（须持锁调用）。
func (l *LoginLimiter) record(m map[string]*attemptBucket, key string, now time.Time) bool {
	if key == "" {
		return false
	}
	b := m[key]
	if b == nil {
		b = &attemptBucket{}
		m[key] = b
	}
	if now.Before(b.lockedUntil) {
		return false
	}
	b.failures = append(filterRecent(b.failures, now.Add(-l.window)), now)
	if len(b.failures) >= l.maxFailures {
		b.lockedUntil = now.Add(l.lockFor)
		b.failures = nil
		return true
	}
	return false
}

func filterRecent(ts []time.Time, after time.Time) []time.Time {
	if len(ts) == 0 {
		return ts
	}
	out := ts[:0]
	for _, t := range ts {
		if t.After(after) {
			out = append(out, t)
		}
	}
	return out
}
