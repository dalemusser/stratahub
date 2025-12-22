// internal/app/features/terms/handler.go
package terms

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.uber.org/zap"
)

type pageData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string
}

type Handler struct {
	Log *zap.Logger
}

func NewHandler(logger *zap.Logger) *Handler {
	return &Handler{
		Log: logger,
	}
}

func (h *Handler) ServeTerms(w http.ResponseWriter, r *http.Request) {
	role, name, _, signedIn := authz.UserCtx(r)

	data := pageData{
		Title:      "Terms of Service",
		IsLoggedIn: signedIn,
		Role:       role,
		UserName:   name,
	}

	templates.Render(w, r, "terms", data)
}
