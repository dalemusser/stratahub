// internal/app/features/dashboard/leader.go
package dashboard

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.uber.org/zap"
)

func (h *Handler) ServeLeader(w http.ResponseWriter, r *http.Request) {
	base := viewdata.NewBaseVM(r, h.DB, "Leader Dashboard", "/")
	data := baseDashboardData{
		BaseVM: base,
	}

	h.Log.Debug("leader dashboard served", zap.String("user", base.UserName))

	templates.Render(w, r, "leader_dashboard", data)
}
