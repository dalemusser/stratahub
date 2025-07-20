package terms

import (
	"embed"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/dalemusser/gowebcore/logger"
	"github.com/go-chi/chi/v5"

	"github.com/dalemusser/stratahub/internal/layout" // shared base + menu
	"github.com/dalemusser/stratahub/internal/platform/handler"
)

/*────────────────── embedded slice templates ──────────────────*/

//go:embed templates/*.html
var views embed.FS

var (
	tplOnce sync.Once
	tpl     *template.Template
)

func parseTemplates() *template.Template {
	funcs := template.FuncMap{"now": time.Now}

	// 1) shared layout / menu
	t, err := template.New("").Funcs(funcs).
		ParseFS(layout.Views, "templates/*.html")
	if err != nil {
		logger.Error("terms: layout parse failed", "err", err)
		panic(err)
	}

	// 2) this slice’s templates
	if _, err := t.ParseFS(views, "templates/*.html"); err != nil {
		logger.Error("terms: slice parse failed", "err", err)
		panic(err)
	}
	return t
}

/*───────────────────── route registration ────────────────────*/

func MountRoutes(r chi.Router, h *handler.Handler) {
	r.Get("/terms", func(w http.ResponseWriter, r *http.Request) {
		tplOnce.Do(func() { tpl = parseTemplates() })

		view := struct {
			Title      string
			Role       string
			IsLoggedIn bool
			UserName   string
		}{
			Title:      "Terms of Service",
			Role:       h.Session.Role(r),
			IsLoggedIn: h.Session.IsAuth(r),
			UserName:   h.Session.UserName(r),
		}

		if err := tpl.ExecuteTemplate(w, "base", view); err != nil {
			logger.Error("terms: render failed", "err", err)
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	})
}
