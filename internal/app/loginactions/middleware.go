// internal/app/loginactions/middleware.go
package loginactions

import (
	"context"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
)

type contextKey int

const loginActionsKey contextKey = 1

// Middleware reads login_actions_js from the session on the first page load
// after login, passes it through request context, and clears it from the session.
func Middleware(sm *auth.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, err := sm.GetSession(r)
			if err == nil {
				if js, ok := sess.Values["login_actions_js"].(string); ok && js != "" {
					delete(sess.Values, "login_actions_js")
					sess.Save(r, w)
					ctx := context.WithValue(r.Context(), loginActionsKey, js)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ScriptsFromContext returns the login actions JavaScript from the request context.
// Returns empty string if no login actions are pending.
func ScriptsFromContext(ctx context.Context) string {
	if js, ok := ctx.Value(loginActionsKey).(string); ok {
		return js
	}
	return ""
}
