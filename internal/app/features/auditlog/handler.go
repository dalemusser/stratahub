// internal/app/features/auditlog/handler.go
package auditlog

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

type Handler struct {
	DB     *mongo.Database
	Log    *zap.Logger
	ErrLog *uierrors.ErrorLogger
}

// NewHandler constructs an Audit Log feature handler bound to
// the given Mongo database and logger.
func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:     db,
		Log:    logger,
		ErrLog: errLog,
	}
}
