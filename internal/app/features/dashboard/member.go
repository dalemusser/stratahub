// internal/app/features/dashboard/member.go
package dashboard

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
)

func (h *Handler) ServeMember(w http.ResponseWriter, r *http.Request) {
	base := viewdata.NewBaseVM(r, h.DB, "Member Dashboard", "/")
	data := baseDashboardData{
		BaseVM: base,
	}

	templates.Render(w, r, "member_dashboard", data)
}
