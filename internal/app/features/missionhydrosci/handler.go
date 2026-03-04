// internal/app/features/missionhydrosci/handler.go
package missionhydrosci

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the dependency container for the Mission HydroSci (experimental) feature.
type Handler struct {
	DB         *mongo.Database
	Log        *zap.Logger
	ErrLog     *uierrors.ErrorLogger
	CDNBaseURL string // e.g., "https://cdn.adroit.games/mhs"
}

// NewHandler constructs a new Handler.
func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, cdnBaseURL string, logger *zap.Logger) *Handler {
	return &Handler{
		DB:         db,
		Log:        logger,
		ErrLog:     errLog,
		CDNBaseURL: cdnBaseURL,
	}
}
