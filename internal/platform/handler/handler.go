// internal/platform/handler/handler.go
package handler

import (
	"github.com/dalemusser/gowebcore/config"
	"github.com/dalemusser/gowebcore/db"

	"github.com/dalemusser/stratahub/internal/platform/session"
)

// Handler is passed to every feature’s MountRoutes; it carries common deps.
type Handler struct {
	Cfg     *config.Base     // log-level, host, TLS flags …
	DB      *db.Manager      // database connections / helpers
	Session *session.Manager // our new session manager (cookie store)
}

// New wires the shared dependencies into one bundle.
func New(cfg *config.Base, dbm *db.Manager, sess *session.Manager) *Handler {
	return &Handler{
		Cfg:     cfg,
		DB:      dbm,
		Session: sess,
	}
}
