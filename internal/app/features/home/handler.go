package home

import (
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/auth" // you'll port this soon
	"github.com/dalemusser/waffle/templates"
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
	user, logged := auth.CurrentUser(r)

	role, userName := "visitor", ""
	if logged {
		role = strings.ToLower(user.Role)
		userName = user.Name
	}

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
