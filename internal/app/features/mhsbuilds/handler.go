// internal/app/features/mhsbuilds/handler.go
package mhsbuilds

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/mhsbuilds"
	"github.com/dalemusser/stratahub/internal/app/store/mhscollections"
	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/waffle/pantry/storage"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the dependency container for the MHS build management feature.
type Handler struct {
	DB              *mongo.Database
	MHSStorage      storage.Store // S3 client for CDN bucket
	BuildStore      *mhsbuilds.Store
	CollectionStore *mhscollections.Store
	SettingsStore   *settingsstore.Store
	Log             *zap.Logger
	ErrLog          *uierrors.ErrorLogger
	CDNBaseURL      string // e.g., "https://cdn.adroit.games/mhs"
}

// NewHandler constructs a new Handler.
func NewHandler(db *mongo.Database, mhsStorage storage.Store, cdnBaseURL string, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:              db,
		MHSStorage:      mhsStorage,
		BuildStore:      mhsbuilds.New(db),
		CollectionStore: mhscollections.New(db),
		SettingsStore:   settingsstore.New(db),
		Log:             logger,
		ErrLog:          errLog,
		CDNBaseURL:      cdnBaseURL,
	}
}
