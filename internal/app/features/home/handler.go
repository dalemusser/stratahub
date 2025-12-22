package home

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.uber.org/zap"
)

// Handler holds dependencies needed to serve the home page.
type Handler struct {
	Log *zap.Logger
}

func NewHandler(logger *zap.Logger) *Handler {
	return &Handler{
		Log: logger,
	}
}

/*─────────────────────────────────────────────────────────────────────────────*
| GET / – landing                                                             |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) ServeRoot(w http.ResponseWriter, r *http.Request) {
	role, userName, _, logged := authz.UserCtx(r)

	data := struct {
		Title      string
		IsLoggedIn bool
		Role       string
		UserName   string
	}{
		Title:      "Welcome to Adroit Games",
		IsLoggedIn: logged,
		Role:       role,
		UserName:   userName,
	}

	templates.Render(w, r, "home", data)
}
