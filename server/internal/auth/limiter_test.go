package auth

import (
	"testing"
	"time"
)

func TestLoginLimiterLocksAfterThreshold(t *testing.T) {
	lim := NewLoginLimiter(3, 5, 5)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	lim.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if ok, _ := lim.Allow("1.2.3.4", "alice"); !ok {
			t.Fatalf("第 %d 次失败前不应锁定", i+1)
		}
		lim.Fail("1.2.3.4", "alice")
	}
	if ok, _ := lim.Allow("1.2.3.4", "alice"); !ok {
		t.Fatal("未达阈值不应锁定")
	}
	lim.Fail("1.2.3.4", "alice")
	ok, retry := lim.Allow("1.2.3.4", "alice")
	if ok {
		t.Fatal("达到阈值后应锁定")
	}
	if retry <= 0 || retry > 5*time.Minute {
		t.Fatalf("剩余锁定时间异常: %v", retry)
	}

	// 锁定期内换 IP 打同一账号仍应被拒绝（用户名维度）。
	if ok, _ := lim.Allow("9.9.9.9", "alice"); ok {
		t.Fatal("用户名维度应独立生效")
	}
	// 锁定期内同 IP 换账号也应被拒绝（IP 维度）。
	if ok, _ := lim.Allow("1.2.3.4", "bob"); ok {
		t.Fatal("IP 维度应独立生效")
	}

	// 锁定到期后恢复。
	now = now.Add(5*time.Minute + time.Second)
	if ok, _ := lim.Allow("1.2.3.4", "alice"); !ok {
		t.Fatal("锁定到期后应放行")
	}
}

func TestLoginLimiterWindowExpiry(t *testing.T) {
	lim := NewLoginLimiter(3, 1, 5)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	lim.now = func() time.Time { return now }

	lim.Fail("ip", "u")
	lim.Fail("ip", "u")
	// 窗口过后旧计数应失效。
	now = now.Add(2 * time.Minute)
	lim.Fail("ip", "u")
	if ok, _ := lim.Allow("ip", "u"); !ok {
		t.Fatal("窗口外的失败不应累计到锁定")
	}
}

func TestLoginLimiterReset(t *testing.T) {
	lim := NewLoginLimiter(2, 5, 5)
	lim.Fail("ip", "u")
	lim.Reset("ip", "u")
	lim.Fail("ip", "u")
	if ok, _ := lim.Allow("ip", "u"); !ok {
		t.Fatal("成功登录清零后不应锁定")
	}
}
