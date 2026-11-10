// internal/app/features/missionhydrosci/handler.go
package missionhydrosci

import (
	"net/url"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/logdata"
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
	// SaveAdminAuth is the auth header for the save service's delete
	// endpoints. LogAuth and SaveAuth are rendered into the play page for
	// the game, so every browser holds them; this one is used only for
	// calls this server makes and must never reach a template. Empty means
	// the save service has no separate delete key and SaveAuth is sent.
	SaveAdminAuth string
}

// deleteAuth returns the auth header for the save service's delete endpoints.
func (s GameServices) deleteAuth() string {
	if s.SaveAdminAuth != "" {
		return s.SaveAdminAuth
	}
	return s.SaveAuth
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
	Logs              *logdata.Store  // read-only view of the log service's entries (logging-health checks); nil when its database is not configured
	GradesDB          *mongo.Database // mhsgrader's database: EA checkpoint scores for the ceremony (progress_point_grades); nil when not configured
	SettingsStore     *settingsstore.Store
	CollectionStore   *mhscollections.Store
	BuildStore        *mhsbuilds.Store
	SessionMgr        *auth.SessionManager
	StaffAuthVerifier *staffauth.Verifier
	UnlockStore       *staffauth.UnlockStore
	authThrottle      *authThrottle      // per-member backoff on failed member-auth attempts
	startLimiter      *ratelimit.Limiter // per-IP quota on starting device-test runs
	startWindow       time.Duration      // the quota's window (for Retry-After)
	svcHealth         servicesHealth     // the server's view of the game services (services_health.go)
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

// ProbesFromServices derives reachability probes from the configured
// game-service endpoints, so a client can tell a blocked log or save host
// from a working one: the services' /health routes (URL) and the /probe
// route beside the endpoint the game posts to (Post: ".../api/log/submit"
// gives ".../api/log/probe"). Endpoints that are unset or malformed produce
// no probe; an endpoint without a path gets no Post.
func ProbesFromServices(s GameServices) []ManifestProbe {
	var probes []ManifestProbe
	add := func(name, endpoint string) {
		u, err := url.Parse(strings.TrimSpace(endpoint))
		if err != nil || u.Scheme == "" || u.Host == "" {
			return
		}
		p := ManifestProbe{Name: name, URL: u.Scheme + "://" + u.Host + "/health"}
		// "/api/log/submit" -> "/api/log/probe"; a one-segment path such as
		// the legacy "/logs" has no sibling route, so no Post.
		if p2 := strings.TrimSuffix(u.Path, "/"); strings.LastIndex(p2, "/") > 0 {
			p.Post = u.Scheme + "://" + u.Host + p2[:strings.LastIndex(p2, "/")] + "/probe"
		}
		probes = append(probes, p)
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
