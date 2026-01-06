// internal/app/features/materials/handler.go
package materials

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/auditlog"
	"github.com/dalemusser/waffle/pantry/storage"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// AdminHandler owns all admin-facing Material handlers
// (list, new, edit, view, delete, assign).
//
// It is constructed once at startup in bootstrap, using the
// shared Mongo database handle, file storage, and logger.
type AdminHandler struct {
	DB       *mongo.Database
	Storage  storage.Store
	Log      *zap.Logger
	ErrLog   *uierrors.ErrorLogger
	AuditLog *auditlog.Logger
}

// LeaderHandler owns the leader-facing Material handlers
// ("My Materials" list and individual material view/download).
//
// It shares the same underlying DB, storage, and logger pattern as
// AdminHandler but keeps the responsibilities clearly separated.
type LeaderHandler struct {
	DB      *mongo.Database
	Storage storage.Store
	Log     *zap.Logger
	ErrLog  *uierrors.ErrorLogger
}

// NewAdminHandler constructs an AdminHandler bound to the
// given Mongo database, file storage, and logger.
func NewAdminHandler(db *mongo.Database, store storage.Store, errLog *uierrors.ErrorLogger, audit *auditlog.Logger, logger *zap.Logger) *AdminHandler {
	return &AdminHandler{
		DB:       db,
		Storage:  store,
		Log:      logger,
		ErrLog:   errLog,
		AuditLog: audit,
	}
}

// NewLeaderHandler constructs a LeaderHandler bound to the
// given Mongo database, file storage, and logger.
func NewLeaderHandler(db *mongo.Database, store storage.Store, errLog *uierrors.ErrorLogger, logger *zap.Logger) *LeaderHandler {
	return &LeaderHandler{
		DB:      db,
		Storage: store,
		Log:     logger,
		ErrLog:  errLog,
	}
}
