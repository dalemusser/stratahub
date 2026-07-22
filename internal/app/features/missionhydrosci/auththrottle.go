// internal/app/features/missionhydrosci/auththrottle.go
package missionhydrosci

import (
	"sync"
	"time"
)

// authThrottle applies lightweight per-member backoff to failed MHS member-auth
// attempts (keyword, staff token, staff password/email code). It is a deterrent
// against a student idly scripting the manage-gate endpoints, NOT a hardened
// brute-force control — the threat is low-effort by design (a member can only
// affect their own account/device, and keyword auth is not used in production),
// so backoff is proportional and we deliberately avoid a hard lockout.
//
// State is in-memory per process: it resets on restart and is not shared across
// instances. That is acceptable for this threat model; if MHS ever runs behind
// multiple instances with a credible brute-force concern, move this to a shared
// store.
type authThrottle struct {
	mu    sync.Mutex
	state map[string]*throttleEntry
}

type throttleEntry struct {
	fails       int
	nextAllowed time.Time
}

const (
	// throttleFreeAttempts is how many consecutive failures incur no delay —
	// enough that a teacher fat-fingering a password/keyword a couple of times
	// is never made to wait.
	throttleFreeAttempts = 3
	// throttleBaseDelay and throttleMaxDelay bound the exponential backoff
	// applied after the free attempts are used up.
	throttleBaseDelay = 2 * time.Second
	throttleMaxDelay  = 60 * time.Second
	// throttleIdleReset clears a member's failure streak once they have gone
	// quiet for this long, and bounds memory (idle entries are dropped).
	throttleIdleReset = 10 * time.Minute
)

func newAuthThrottle() *authThrottle {
	return &authThrottle{state: make(map[string]*throttleEntry)}
}

// retryAfter returns a non-zero duration when key is currently in a backoff
// window and must wait before another attempt is processed. A nil throttle
// (e.g. a Handler built without newAuthThrottle in a test) never throttles.
func (t *authThrottle) retryAfter(key string, now time.Time) time.Duration {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.state[key]
	if e == nil {
		return 0
	}
	// Drop a streak (and its memory) once the member has been idle a while.
	if now.Sub(e.nextAllowed) > throttleIdleReset {
		delete(t.state, key)
		return 0
	}
	if now.Before(e.nextAllowed) {
		return e.nextAllowed.Sub(now)
	}
	return 0
}

// fail records a failed attempt for key and extends its backoff window.
func (t *authThrottle) fail(key string, now time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.state[key]
	if e == nil {
		e = &throttleEntry{}
		t.state[key] = e
	}
	e.fails++
	e.nextAllowed = now.Add(backoffDelay(e.fails))
}

// success clears any backoff state for key.
func (t *authThrottle) success(key string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.state, key)
}

// backoffDelay is 0 for the first throttleFreeAttempts failures, then doubles
// from throttleBaseDelay up to throttleMaxDelay.
func backoffDelay(fails int) time.Duration {
	if fails <= throttleFreeAttempts {
		return 0
	}
	d := throttleBaseDelay
	for i := 0; i < fails-throttleFreeAttempts-1; i++ {
		d *= 2
		if d >= throttleMaxDelay {
			return throttleMaxDelay
		}
	}
	if d > throttleMaxDelay {
		d = throttleMaxDelay
	}
	return d
}
