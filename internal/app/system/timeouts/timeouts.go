// Package timeouts provides centralized timeout values for handler operations.
//
// These timeouts are used with context.WithTimeout for database operations
// and other I/O in HTTP handlers. Using centralized values ensures consistency
// and makes it easy to adjust timeouts across the application.
//
// Timeouts can be configured at startup using Configure(). If not configured,
// sensible defaults are used.
//
// Guidelines for choosing a timeout:
//   - Ping: health checks and connectivity verification
//   - Short: simple single-document reads or lookups
//   - Medium: list queries, moderate writes, multi-step reads
//   - Long: complex writes, operations touching multiple collections
//   - Batch: bulk imports, CSV uploads, large batch operations
package timeouts

import (
	"context"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Default timeout values (used if Configure is not called).
const (
	DefaultPing   = 2 * time.Second
	DefaultShort  = 5 * time.Second
	DefaultMedium = 10 * time.Second
	DefaultLong   = 30 * time.Second
	DefaultBatch  = 60 * time.Second
)

// mu protects all timeout values from concurrent access.
var mu sync.RWMutex

// Configurable timeout values. These start with defaults and can be
// overridden by calling Configure(). Access via getter functions.
var (
	ping   = DefaultPing
	short  = DefaultShort
	medium = DefaultMedium
	long   = DefaultLong
	batch  = DefaultBatch
)

// Ping returns the timeout for health checks and connectivity verification.
// Used by health endpoints to verify database connectivity.
func Ping() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	return ping
}

// Short returns the timeout for simple operations like single-document reads.
// Examples: get by ID, lookup by email, render a form.
func Short() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	return short
}

// Medium returns the timeout for moderate operations like list queries.
// Examples: paginated lists, filtered queries, simple creates/updates.
func Medium() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	return medium
}

// Long returns the timeout for complex operations touching multiple collections.
// Examples: creates with related records, complex updates, deletes with cleanup.
func Long() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	return long
}

// Batch returns the timeout for bulk operations like CSV imports.
// Examples: uploading member CSVs, batch upserts, large data migrations.
func Batch() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	return batch
}

// Config holds timeout configuration values.
// Zero values are ignored (defaults are kept).
type Config struct {
	Ping   time.Duration
	Short  time.Duration
	Medium time.Duration
	Long   time.Duration
	Batch  time.Duration
}

// Configure sets custom timeout values. Zero values in the config are ignored,
// keeping the current (or default) values. This should be called during
// application startup before handlers are registered.
//
// Example:
//
//	timeouts.Configure(timeouts.Config{
//	    Short:  10 * time.Second,  // double the default
//	    Medium: 20 * time.Second,
//	})
func Configure(cfg Config) {
	mu.Lock()
	defer mu.Unlock()
	if cfg.Ping > 0 {
		ping = cfg.Ping
	}
	if cfg.Short > 0 {
		short = cfg.Short
	}
	if cfg.Medium > 0 {
		medium = cfg.Medium
	}
	if cfg.Long > 0 {
		long = cfg.Long
	}
	if cfg.Batch > 0 {
		batch = cfg.Batch
	}
}

// Reset restores all timeouts to their default values.
// Useful for testing.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	ping = DefaultPing
	short = DefaultShort
	medium = DefaultMedium
	long = DefaultLong
	batch = DefaultBatch
}

// ConfigureFromEnv reads timeout configuration from environment variables.
// Environment variables (all optional, defaults used if not set or invalid):
//   - TIMEOUT_PING: e.g., "2s", "500ms"
//   - TIMEOUT_SHORT: e.g., "5s"
//   - TIMEOUT_MEDIUM: e.g., "10s"
//   - TIMEOUT_LONG: e.g., "30s"
//   - TIMEOUT_BATCH: e.g., "60s", "2m"
//
// Returns the number of timeouts successfully configured from environment.
func ConfigureFromEnv() int {
	mu.Lock()
	defer mu.Unlock()
	configured := 0

	if v := os.Getenv("TIMEOUT_PING"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			ping = d
			configured++
		}
	}
	if v := os.Getenv("TIMEOUT_SHORT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			short = d
			configured++
		}
	}
	if v := os.Getenv("TIMEOUT_MEDIUM"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			medium = d
			configured++
		}
	}
	if v := os.Getenv("TIMEOUT_LONG"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			long = d
			configured++
		}
	}
	if v := os.Getenv("TIMEOUT_BATCH"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			batch = d
			configured++
		}
	}

	return configured
}

// Current returns the current timeout configuration as a Config struct.
// Useful for logging or debugging.
func Current() Config {
	mu.RLock()
	defer mu.RUnlock()
	return Config{
		Ping:   ping,
		Short:  short,
		Medium: medium,
		Long:   long,
		Batch:  batch,
	}
}

// WithTimeout creates a context with timeout and returns a cancel function that
// logs a warning if the context was canceled due to deadline exceeded.
// Use this for long-running or critical operations where timeout debugging is important.
//
// Example:
//
//	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "batch member import")
//	defer cancel()
func WithTimeout(parent context.Context, timeout time.Duration, log *zap.Logger, operation string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, func() {
		if ctx.Err() == context.DeadlineExceeded && log != nil {
			log.Warn("operation timed out",
				zap.String("operation", operation),
				zap.Duration("timeout", timeout),
			)
		}
		cancel()
	}
}
