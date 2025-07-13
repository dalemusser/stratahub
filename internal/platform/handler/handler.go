package handler

import (
	"github.com/dalemusser/gowebcore/auth"
	"github.com/dalemusser/gowebcore/config"
	"github.com/dalemusser/gowebcore/db"
)

type Handler struct {
	Cfg     *config.Base
	DB      *db.Manager
	Session *auth.Session
}

func New(cfg *config.Base, dbm *db.Manager, sess *auth.Session) *Handler {
	return &Handler{Cfg: cfg, DB: dbm, Session: sess}
}
