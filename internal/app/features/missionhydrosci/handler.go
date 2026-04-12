// internal/app/features/missionhydrosci/handler.go
package missionhydrosci

import (
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/mhsbuilds"
	"github.com/dalemusser/stratahub/internal/app/store/mhscollections"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicestatus"
	"github.com/dalemusser/stratahub/internal/app/store/mhsuserprogress"
	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/staffauth"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// GameServices holds all game service endpoint URLs and auth headers.
// Each URL is a full endpoint (e.g., "https://log.adroit.games/api/log/submit").
type GameServices struct {
	LogSubmitURL    string
	LogAuth         string
	StateSaveURL    string
	StateLoadURL    string
	SettingsSaveURL string
	SettingsLoadURL string
	SaveAuth        string
}

// Handler is the dependency container for the Mission HydroSci (experimental) feature.
type Handler struct {
	DB                *mongo.Database
	Log               *zap.Logger
	ErrLog            *uierrors.ErrorLogger
	CDNBaseURL        string // e.g., "https://cdn.adroit.games/mhs"
	Services          GameServices
	ProgressStore     *mhsuserprogress.Store
	DeviceStatusStore *mhsdevicestatus.Store
	SettingsStore     *settingsstore.Store
	CollectionStore   *mhscollections.Store
	BuildStore        *mhsbuilds.Store
	SessionMgr        *auth.SessionManager
	StaffAuthVerifier *staffauth.Verifier
}

// NewHandler constructs a new Handler.
func NewHandler(db *mongo.Database, errLog *uierrors.ErrorLogger, cdnBaseURL string, services GameServices, sm *auth.SessionManager, logger *zap.Logger) *Handler {
	return &Handler{
		DB:                db,
		Log:               logger,
		ErrLog:            errLog,
		CDNBaseURL:        cdnBaseURL,
		Services:          services,
		ProgressStore:     mhsuserprogress.New(db),
		DeviceStatusStore: mhsdevicestatus.New(db),
		SettingsStore:     settingsstore.New(db),
		CollectionStore:   mhscollections.New(db),
		BuildStore:        mhsbuilds.New(db),
		SessionMgr:        sm,
	}
}
