// internal/app/features/dashboard/leader.go
package dashboard

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"go.uber.org/zap"
)

func (h *Handler) ServeLeader(w http.ResponseWriter, r *http.Request) {
	role, uname, _, signedIn := authz.UserCtx(r)

	data := baseDashboardData{
		Title:       "Leader Dashboard",
		IsLoggedIn:  signedIn,
		Role:        role,
		UserName:    uname,
		CurrentPath: httpnav.CurrentPath(r),
	}

	h.Log.Debug("leader dashboard served", zap.String("user", uname))

	templates.Render(w, r, "leader_dashboard", data)
}
