// internal/app/features/dashboard/routes.go
package dashboard

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

const (
	AdminDashboard   = "/admin/dashboard"
	AnalystDashboard = "/analyst/dashboard"
	LeaderDashboard  = "/leader/dashboard"
	MemberDashboard  = "/member/dashboard"
)

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		pr.Use(auth.RequireRole("admin"))
		pr.Get(AdminDashboard, h.ServeAdmin)
	})

	r.Group(func(pr chi.Router) {
		pr.Use(auth.RequireRole("analyst"))
		pr.Get(AnalystDashboard, h.ServeAnalyst)
	})

	r.Group(func(pr chi.Router) {
		pr.Use(auth.RequireRole("leader"))
		pr.Get(LeaderDashboard, h.ServeLeader)
	})

	r.Group(func(pr chi.Router) {
		pr.Use(auth.RequireRole("member"))
		pr.Get(MemberDashboard, h.ServeMember)
	})

	return r
}
