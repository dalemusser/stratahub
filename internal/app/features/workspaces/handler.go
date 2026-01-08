// internal/app/features/workspaces/handler.go
package workspaces

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/auditlog"
	"github.com/dalemusser/waffle/pantry/storage"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler provides HTTP handlers for workspace management.
// This feature is superadmin-only and accessible at the apex domain.
type Handler struct {
	DB            *mongo.Database
	Storage       storage.Store
	Log           *zap.Logger
	ErrLog        *uierrors.ErrorLogger
	AuditLog      *auditlog.Logger
	PrimaryDomain string // For subdomain display (e.g., "adroit.games")
}

// NewHandler creates a new workspaces Handler.
func NewHandler(db *mongo.Database, store storage.Store, errLog *uierrors.ErrorLogger, audit *auditlog.Logger, primaryDomain string, logger *zap.Logger) *Handler {
	return &Handler{
		DB:            db,
		Storage:       store,
		Log:           logger,
		ErrLog:        errLog,
		AuditLog:      audit,
		PrimaryDomain: primaryDomain,
	}
}
