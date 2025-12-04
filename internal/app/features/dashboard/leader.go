// internal/app/features/dashboard/leader.go
package dashboard

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.uber.org/zap"
)

type leaderData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	CurrentPath string
}

func (h *Handler) ServeLeader(w http.ResponseWriter, r *http.Request) {
	role, uname, _, signedIn := authz.UserCtx(r)

	data := leaderData{
		Title:       "Leader Dashboard",
		IsLoggedIn:  signedIn,
		Role:        role,
		UserName:    uname,
		CurrentPath: nav.CurrentPath(r),
	}

	h.Log.Debug("leader dashboard served", zap.String("user", uname))

	templates.Render(w, r, "leader_dashboard", data)
}
