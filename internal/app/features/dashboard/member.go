// internal/app/features/dashboard/member.go
package dashboard

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.uber.org/zap"
)

func (h *Handler) ServeMember(w http.ResponseWriter, r *http.Request) {
	base := viewdata.NewBaseVM(r, h.DB, "Member Dashboard", "/")
	data := baseDashboardData{
		BaseVM: base,
	}

	h.Log.Debug("member dashboard served", zap.String("user", base.UserName))

	templates.Render(w, r, "member_dashboard", data)
}
