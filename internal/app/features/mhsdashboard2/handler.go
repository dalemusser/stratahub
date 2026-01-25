// internal/app/features/mhsdashboard2/handler.go
package mhsdashboard2

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the shared dependency container for the MHS dashboard 2 feature.
type Handler struct {
	DB     *mongo.Database
	Log    *zap.Logger
	ErrLog *uierrors.ErrorLogger
}

// NewHandler constructs a new Handler.
func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:     db,
		Log:    logger,
		ErrLog: errLog,
	}
}
