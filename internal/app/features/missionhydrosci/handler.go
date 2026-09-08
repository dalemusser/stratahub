// internal/app/features/missionhydrosci/handler.go
package missionhydrosci

import (
	"net/url"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/mhsbuilds"
	"github.com/dalemusser/stratahub/internal/app/store/mhscollections"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicestatus"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicetests"
	"github.com/dalemusser/stratahub/internal/app/store/mhsuserprogress"
	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/ratelimit"
	"github.com/dalemusser/stratahub/internal/app/system/staffauth"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// GameServices holds all game service endpoint URLs and auth headers.
// Each URL is a full endpoint (e.g., "https://log.adroit.games/api/log/submit").
type GameServices struct {
	LogSubmitURL      string
	LogAuth           string
	StateSaveURL      string
	StateLoadURL      string
	StateDeleteURL    string
	SettingsSaveURL   string
	SettingsLoadURL   string
	SettingsDeleteURL string
	SaveAuth          string
}

// Handler is the dependency container for the Mission HydroSci (experimental) feature.
type Handler struct {
	DB                *mongo.Database
	Log               *zap.Logger
	ErrLog            *uierrors.ErrorLogger
	CDNBaseURL        string // e.g., "https://cdn.adroit.games/mhs"
	Services          GameServices
	Tuning            ManifestTuning  // client download timing, served with the manifest
	Probes            []ManifestProbe // game-service reachability probes, served with the manifest
	ProgressStore     *mhsuserprogress.Store
	DeviceStatusStore *mhsdevicestatus.Store
	DeviceTestStore   *mhsdevicetests.Store
	SettingsStore     *settingsstore.Store
	CollectionStore   *mhscollections.Store
	BuildStore        *mhsbuilds.Store
	SessionMgr        *auth.SessionManager
	StaffAuthVerifier *staffauth.Verifier
	UnlockStore       *staffauth.UnlockStore
	authThrottle      *authThrottle      // per-member backoff on failed member-auth attempts
	startLimiter      *ratelimit.Limiter // per-IP quota on starting device-test runs
	startWindow       time.Duration      // the quota's window (for Retry-After)
}

// SetDeviceTestStartLimit replaces the per-IP quota on starting device-test
// runs (config keys mhs_device_test_start_limit / _window). Non-positive
// values keep the defaults.
func (h *Handler) SetDeviceTestStartLimit(limit int, window time.Duration) {
	if limit <= 0 {
		limit = deviceTestStartLimit
	}
	if window <= 0 {
		window = deviceTestStartWindow
	}
	h.startLimiter = ratelimit.New(limit, window)
	h.startWindow = window
}

// ProbesFromServices derives reachability probes (the services' /health
// routes) from the configured game-service endpoints, so a client can tell a
// blocked log or save host from a working one before a download or launch.
// Endpoints that are unset or malformed produce no probe.
func ProbesFromServices(s GameServices) []ManifestProbe {
	var probes []ManifestProbe
	add := func(name, endpoint string) {
		u, err := url.Parse(strings.TrimSpace(endpoint))
		if err != nil || u.Scheme == "" || u.Host == "" {
			return
		}
		probes = append(probes, ManifestProbe{Name: name, URL: u.Scheme + "://" + u.Host + "/health"})
	}
	add("log", s.LogSubmitURL)
	add("save", s.StateSaveURL)
	return probes
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
		DeviceTestStore:   mhsdevicetests.New(db),
		SettingsStore:     settingsstore.New(db),
		CollectionStore:   mhscollections.New(db),
		BuildStore:        mhsbuilds.New(db),
		SessionMgr:        sm,
		UnlockStore:       staffauth.NewUnlockStore(db),
		authThrottle:      newAuthThrottle(),
		startLimiter:      ratelimit.New(deviceTestStartLimit, deviceTestStartWindow),
		startWindow:       deviceTestStartWindow,
	}
}
