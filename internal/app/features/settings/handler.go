// internal/app/features/settings/handler.go
package settings

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/waffle/pantry/storage"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler owns all admin-facing Settings handlers.
type Handler struct {
	DB      *mongo.Database
	Storage storage.Store
	Log     *zap.Logger
	ErrLog  *uierrors.ErrorLogger
}

// NewHandler constructs a Handler bound to the given Mongo database, file storage, and logger.
func NewHandler(db *mongo.Database, store storage.Store, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:      db,
		Storage: store,
		Log:     logger,
		ErrLog:  errLog,
	}
}
