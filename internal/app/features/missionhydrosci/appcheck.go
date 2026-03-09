package missionhydrosci

import (
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
)

// RequireApp returns middleware that ensures the given app is enabled for members.
// Non-member roles (admin, coordinator, leader, superadmin) pass unconditionally.
// Members are checked against their SessionUser.EnabledApps.
// If denied, a forbidden page is rendered.
func RequireApp(appID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := auth.CurrentUser(r)
			if !ok {
				// Not signed in — let downstream RequireSignedIn handle it
				next.ServeHTTP(w, r)
				return
			}

			// Non-member roles always have access
			if user.Role != "member" {
				next.ServeHTTP(w, r)
				return
			}

			// Members: check EnabledApps
			for _, id := range user.EnabledApps {
				if id == appID {
					next.ServeHTTP(w, r)
					return
				}
			}

			// App not enabled for this member
			uierrors.RenderForbidden(w, r,
				"This feature is not available for your group.",
				"/dashboard")
		})
	}
}
