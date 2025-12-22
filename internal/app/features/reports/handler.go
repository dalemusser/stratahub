// internal/app/features/reports/handler.go
package reports

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler owns all Members Report handlers (HTML page + CSV export).
//
// It follows the same pattern as other features in stratahub:
// a thin struct wrapping the shared Mongo database handle and logger,
// constructed once at startup in bootstrap and passed into Routes().
type Handler struct {
	DB     *mongo.Database
	Log    *zap.Logger
	ErrLog *uierrors.ErrorLogger
}

// NewHandler constructs a reports Handler bound to the given Mongo
// database and logger.
func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:     db,
		Log:    logger,
		ErrLog: errLog,
	}
}
