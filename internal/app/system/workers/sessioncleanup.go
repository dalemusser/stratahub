// internal/app/system/workers/sessioncleanup.go
package workers

import (
	"context"
	"sync"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	"go.uber.org/zap"
)

// SessionCleanup is a background worker that closes inactive sessions.
type SessionCleanup struct {
	sessions          *sessions.Store
	log               *zap.Logger
	interval          time.Duration
	inactiveThreshold time.Duration
	stopCh            chan struct{}
	wg                sync.WaitGroup
}

// NewSessionCleanup creates a new session cleanup worker.
//
// Parameters:
//   - sessStore: the sessions store
//   - logger: zap logger for logging
//   - interval: how often to run cleanup (e.g., 1 minute)
//   - inactiveThreshold: how long a session must be inactive before closing (e.g., 10 minutes)
func NewSessionCleanup(sessStore *sessions.Store, logger *zap.Logger, interval, inactiveThreshold time.Duration) *SessionCleanup {
	return &SessionCleanup{
		sessions:          sessStore,
		log:               logger,
		interval:          interval,
		inactiveThreshold: inactiveThreshold,
		stopCh:            make(chan struct{}),
	}
}

// Start begins the background cleanup loop.
func (w *SessionCleanup) Start() {
	w.wg.Add(1)
	go w.run()
	w.log.Info("session cleanup worker started",
		zap.Duration("interval", w.interval),
		zap.Duration("inactive_threshold", w.inactiveThreshold))
}

// Stop signals the worker to stop and waits for it to finish.
func (w *SessionCleanup) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	w.log.Info("session cleanup worker stopped")
}

func (w *SessionCleanup) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.cleanup()
		}
	}
}

func (w *SessionCleanup) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	count, err := w.sessions.CloseInactiveSessionsSimple(ctx, w.inactiveThreshold)
	if err != nil {
		w.log.Error("failed to close inactive sessions", zap.Error(err))
		return
	}

	if count > 0 {
		w.log.Info("closed inactive sessions", zap.Int64("count", count))
	}
}
