// internal/app/system/ratelimit/ratelimit.go
package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter provides rate limiting using a sliding window algorithm.
// It is safe for concurrent use.
type Limiter struct {
	mu       sync.Mutex
	windows  map[string]*window
	limit    int           // max requests per window
	duration time.Duration // window duration
	cleanup  time.Duration // how often to clean old entries
}

type window struct {
	count     int
	expiresAt time.Time
}

// New creates a new rate limiter.
// limit: maximum requests allowed per duration
// duration: the time window for counting requests
func New(limit int, duration time.Duration) *Limiter {
	l := &Limiter{
		windows:  make(map[string]*window),
		limit:    limit,
		duration: duration,
		cleanup:  duration * 2, // cleanup entries older than 2x duration
	}
	go l.cleanupLoop()
	return l
}

// Allow checks if a request from the given key should be allowed.
// Returns true if allowed, false if rate limited.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	w, exists := l.windows[key]

	// If no window exists or window expired, create new one
	if !exists || now.After(w.expiresAt) {
		l.windows[key] = &window{
			count:     1,
			expiresAt: now.Add(l.duration),
		}
		return true
	}

	// Window still active - check limit
	if w.count >= l.limit {
		return false
	}

	w.count++
	return true
}

// Remaining returns how many requests are left for this key in the current window.
func (l *Limiter) Remaining(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	w, exists := l.windows[key]

	if !exists || now.After(w.expiresAt) {
		return l.limit
	}

	remaining := l.limit - w.count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Reset clears the rate limit for a specific key.
// Useful after successful authentication to reward good behavior.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.windows, key)
}

// cleanupLoop periodically removes expired entries to prevent memory leaks.
func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(l.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for key, w := range l.windows {
			if now.After(w.expiresAt) {
				delete(l.windows, key)
			}
		}
		l.mu.Unlock()
	}
}

// ClientIP extracts the client IP from an HTTP request.
// It checks X-Forwarded-For and X-Real-IP headers first (for proxied requests),
// then falls back to RemoteAddr.
func ClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (comma-separated list, first is client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr (strip port)
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have a port
		return r.RemoteAddr
	}
	return ip
}

// LoginLimiter provides specialized rate limiting for login attempts.
// It tracks both IP-based and email-based limits to prevent:
// - Distributed attacks from multiple IPs
// - Targeted attacks on specific accounts
type LoginLimiter struct {
	ipLimiter    *Limiter
	emailLimiter *Limiter
}

// NewLoginLimiter creates a limiter configured for login protection.
// Defaults: 10 attempts per IP per minute, 5 attempts per email per 5 minutes.
func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{
		ipLimiter:    New(10, time.Minute),          // 10 attempts per minute per IP
		emailLimiter: New(5, 5*time.Minute),         // 5 attempts per 5 minutes per email
	}
}

// NewLoginLimiterWithConfig creates a login limiter with custom limits.
func NewLoginLimiterWithConfig(ipLimit int, ipDuration time.Duration, emailLimit int, emailDuration time.Duration) *LoginLimiter {
	return &LoginLimiter{
		ipLimiter:    New(ipLimit, ipDuration),
		emailLimiter: New(emailLimit, emailDuration),
	}
}

// Check verifies if a login attempt should be allowed.
// Returns (allowed, reason) where reason explains why it was blocked.
func (ll *LoginLimiter) Check(r *http.Request, email string) (bool, string) {
	ip := ClientIP(r)

	// Check IP limit first
	if !ll.ipLimiter.Allow(ip) {
		return false, "Too many login attempts. Please wait a minute before trying again."
	}

	// Check email limit (only if email provided)
	if email != "" {
		emailKey := strings.ToLower(strings.TrimSpace(email))
		if !ll.emailLimiter.Allow(emailKey) {
			return false, "Too many login attempts for this account. Please wait a few minutes."
		}
	}

	return true, ""
}

// ResetEmail clears the rate limit for a specific email after successful login.
func (ll *LoginLimiter) ResetEmail(email string) {
	if email != "" {
		emailKey := strings.ToLower(strings.TrimSpace(email))
		ll.emailLimiter.Reset(emailKey)
	}
}
