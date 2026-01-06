// internal/app/features/activity/handler.go
package activity

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler owns the activity dashboard handlers for leaders.
type Handler struct {
	DB         *mongo.Database
	Sessions   *sessions.Store
	Activity   *activity.Store
	SessionMgr *auth.SessionManager
	Log        *zap.Logger
	ErrLog     *uierrors.ErrorLogger
}

// NewHandler creates a new activity Handler.
func NewHandler(db *mongo.Database, sessStore *sessions.Store, activityStore *activity.Store, sessionMgr *auth.SessionManager, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:         db,
		Sessions:   sessStore,
		Activity:   activityStore,
		SessionMgr: sessionMgr,
		ErrLog:     errLog,
		Log:        logger,
	}
}
