// internal/app/system/maintenance/maintenance.go
package maintenance

import (
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/store/globalsettings"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// maintenanceData is the view model for the maintenance page.
type maintenanceData struct {
	viewdata.BaseVM
	MaintenanceMessage string
}

// Middleware returns a Chi middleware that blocks non-admin users when
// maintenance mode is enabled in global settings.
//
// Admin and superadmin users pass through normally (a banner is shown via the layout).
// All other users see a maintenance page with a 503 status code.
//
// Certain paths are always exempt (health checks, static assets, auth flows).
func Middleware(store *globalsettings.Store, db *mongo.Database, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always exempt certain paths
			if isExemptPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			gs, err := store.Get(r.Context())
			if err != nil {
				// If we can't read settings, let the request through
				logger.Error("failed to read global settings for maintenance check", zap.Error(err))
				next.ServeHTTP(w, r)
				return
			}

			if !gs.MaintenanceMode {
				next.ServeHTTP(w, r)
				return
			}

			// Maintenance mode is on — check if user is exempt
			if user, ok := auth.CurrentUser(r); ok {
				if user.Role == "superadmin" || user.Role == "admin" {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Block the request — show maintenance page
			data := maintenanceData{
				BaseVM:             viewdata.LoadBase(r, db),
				MaintenanceMessage: gs.MaintenanceMessage,
			}
			data.Title = "Maintenance"

			w.WriteHeader(http.StatusServiceUnavailable)
			templates.Render(w, r, "errors/maintenance", data)
		})
	}
}

// isExemptPath returns true for paths that should never be blocked.
func isExemptPath(path string) bool {
	switch path {
	case "/", "/health", "/login", "/logout", "/clear-session",
		"/sw.js", "/manifest.json":
		return true
	}
	return strings.HasPrefix(path, "/assets/") ||
		strings.HasPrefix(path, "/static/") ||
		strings.HasPrefix(path, "/auth/") ||
		strings.HasPrefix(path, "/login/")
}
