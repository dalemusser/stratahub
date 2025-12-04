// internal/app/features/dashboard/member.go
package dashboard

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.uber.org/zap"
)

type memberData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	CurrentPath string
}

func (h *Handler) ServeMember(w http.ResponseWriter, r *http.Request) {
	role, uname, _, signedIn := authz.UserCtx(r)

	data := memberData{
		Title:       "Member Dashboard",
		IsLoggedIn:  signedIn,
		Role:        role,
		UserName:    uname,
		CurrentPath: nav.CurrentPath(r),
	}

	h.Log.Debug("member dashboard served", zap.String("user", uname))

	templates.Render(w, r, "member_dashboard", data)
}
