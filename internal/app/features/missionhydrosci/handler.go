// internal/app/features/missionhydrosci/handler.go
package missionhydrosci

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicestatus"
	"github.com/dalemusser/stratahub/internal/app/store/mhsuserprogress"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// GameServiceConfig holds the URL and auth header for a game service (log or save).
type GameServiceConfig struct {
	URL  string
	Auth string
}

// Handler is the dependency container for the Mission HydroSci (experimental) feature.
type Handler struct {
	DB                *mongo.Database
	Log               *zap.Logger
	ErrLog            *uierrors.ErrorLogger
	CDNBaseURL        string // e.g., "https://cdn.adroit.games/mhs"
	LogService        GameServiceConfig
	SaveService       GameServiceConfig
	ProgressStore     *mhsuserprogress.Store
	DeviceStatusStore *mhsdevicestatus.Store
}

// NewHandler constructs a new Handler.
func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, cdnBaseURL string, logSvc, saveSvc GameServiceConfig, logger *zap.Logger) *Handler {
	return &Handler{
		DB:                db,
		Log:               logger,
		ErrLog:            errLog,
		CDNBaseURL:        cdnBaseURL,
		LogService:        logSvc,
		SaveService:       saveSvc,
		ProgressStore:     mhsuserprogress.New(db),
		DeviceStatusStore: mhsdevicestatus.New(db),
	}
}
