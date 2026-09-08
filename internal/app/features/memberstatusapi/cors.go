// internal/app/features/memberstatusapi/cors.go
//
// Cross-origin (browser) calls. The provider may call the API not from its
// own server but from JavaScript in the survey page itself, in the student's
// browser. That is a cross-origin request, so the browser first asks this
// server (a preflight OPTIONS) whether the page's origin may POST here, and
// then lets the page read the response only if the answer names that origin.
// The policy is per workspace: the origins listed on Settings → Member Status
// API. Nothing else changes — the key still travels in the body, no cookie is
// ever involved, and a call with no Origin header is served exactly as before.
package memberstatusapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/go-chi/cors"
	"go.uber.org/zap"
)

// corsMaxAge is how long (seconds) a browser may cache a preflight answer.
// Browsers cap it themselves (Chrome at two hours). An origin removed on
// Settings can therefore keep working in an already-open page for up to
// that long; adding one takes effect on the next page load.
const corsMaxAge = 3600

// CORS returns root-router middleware that answers browser cross-origin
// requests to the Member Status API for origins the workspace allows. For
// every other path it is a pass-through.
//
// Policy: POST only; request headers Content-Type and Authorization; no
// credentials (Access-Control-Allow-Credentials is never sent). A preflight
// is answered here and stops the chain, so it never reaches CSRF or
// maintenance handling. An actual request gets Access-Control-Allow-Origin
// on every response, error responses included, so the page can read the
// error body; the request itself is still processed only if the key checks
// out, exactly as for a server-to-server caller.
//
// Placement matters. Install it on the root router AFTER the workspace
// middleware (the allowlist is read from the workspace in the request
// context) and BEFORE the global CORS middleware, which answers every
// preflight itself — with no allow-origin header for origins it does not
// know — and would otherwise block the browser before this policy could
// apply. The global list must not be used for this: it allows credentials
// and covers every route.
func (h *Handler) CORS() func(http.Handler) http.Handler {
	policy := cors.New(cors.Options{
		AllowOriginFunc:  h.originAllowed,
		AllowedMethods:   []string{http.MethodPost},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: false,
		MaxAge:           corsMaxAge,
	})
	return func(next http.Handler) http.Handler {
		api := policy.Handler(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isAPIPath(r.URL.Path) {
				api.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// originAllowed is the per-request check: the workspace the request reached
// must list the origin. It runs for preflights (no body, no key) as well as
// actual requests, so it relies on nothing but the host.
func (h *Handler) originAllowed(r *http.Request, origin string) bool {
	ws := workspace.FromRequest(r)
	if ws == nil || ws.IsApex || ws.ID.IsZero() {
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	settings, err := h.Settings.Get(ctx, ws.ID)
	if err != nil {
		h.Log.Error("member status: settings lookup for CORS failed",
			zap.String("workspace_id", ws.ID.Hex()), zap.Error(err))
		return false
	}
	return OriginAllowed(settings.MemberStatusAPIAllowedOrigins, origin)
}

// isAPIPath reports whether path is the API root or one of its routes.
func isAPIPath(path string) bool {
	return path == PathPrefix || strings.HasPrefix(path, PathPrefix+"/")
}
