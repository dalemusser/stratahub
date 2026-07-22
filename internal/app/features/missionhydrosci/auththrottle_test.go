// internal/app/features/missionhydrosci/auththrottle_test.go
package missionhydrosci

import (
	"testing"
	"time"
)

func TestAuthThrottle_FreeAttemptsThenBackoff(t *testing.T) {
	tr := newAuthThrottle()
	now := time.Now()
	key := "member:ws"

	// The first throttleFreeAttempts failures incur no delay.
	for i := 0; i < throttleFreeAttempts; i++ {
		tr.fail(key, now)
		if wait := tr.retryAfter(key, now); wait != 0 {
			t.Fatalf("failure %d should be free, got wait=%v", i+1, wait)
		}
	}

	// The next failure starts the backoff window.
	tr.fail(key, now)
	if wait := tr.retryAfter(key, now); wait <= 0 {
		t.Fatalf("expected a backoff window after exceeding free attempts, got %v", wait)
	}

	// The window grows with further failures, capped at throttleMaxDelay.
	for i := 0; i < 20; i++ {
		tr.fail(key, now)
	}
	if wait := tr.retryAfter(key, now); wait > throttleMaxDelay {
		t.Fatalf("backoff exceeded cap: %v > %v", wait, throttleMaxDelay)
	}
}

func TestAuthThrottle_SuccessResets(t *testing.T) {
	tr := newAuthThrottle()
	now := time.Now()
	key := "member:ws"

	for i := 0; i < throttleFreeAttempts+3; i++ {
		tr.fail(key, now)
	}
	if tr.retryAfter(key, now) == 0 {
		t.Fatal("expected an active backoff window before success")
	}

	tr.success(key)
	if wait := tr.retryAfter(key, now); wait != 0 {
		t.Fatalf("success should clear backoff, got %v", wait)
	}
}

func TestAuthThrottle_IdleReset(t *testing.T) {
	tr := newAuthThrottle()
	now := time.Now()
	key := "member:ws"

	for i := 0; i < throttleFreeAttempts+5; i++ {
		tr.fail(key, now)
	}
	// Well after the idle-reset window, the streak is forgotten.
	later := now.Add(throttleIdleReset + time.Minute)
	if wait := tr.retryAfter(key, later); wait != 0 {
		t.Fatalf("idle member should be reset, got %v", wait)
	}
}

func TestAuthThrottle_NilSafe(t *testing.T) {
	var tr *authThrottle // e.g. a Handler built without newAuthThrottle
	now := time.Now()
	tr.fail("k", now)
	tr.success("k")
	if wait := tr.retryAfter("k", now); wait != 0 {
		t.Fatalf("nil throttle must never throttle, got %v", wait)
	}
}
