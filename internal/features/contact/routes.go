package contact

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

/*──────────────────────── embedded slice templates ──────────────*/

//go:embed templates/*.html
var views embed.FS

var (
	tplOnce sync.Once
	tpl     *template.Template
)

// parseTemplates merges the shared layout/menu with this slice’s templates
// and registers a simple {{ now }} helper.
func parseTemplates() *template.Template {
	funcs := template.FuncMap{"now": time.Now}

	// 1) shared layout & menu
	t, err := template.New("").
		Funcs(funcs).
		ParseFS(layout.Views, "templates/*.html")
	if err != nil {
		logger.Error("contact: layout parse failed", "err", err)
		panic(err)
	}

	// 2) slice-specific templates
	if _, err := t.ParseFS(views, "templates/*.html"); err != nil {
		logger.Error("contact: slice parse failed", "err", err)
		panic(err)
	}
	return t
}

/*────────────────────────── route registration ───────────────────*/

// MountRoutes attaches GET /contact.
func MountRoutes(r chi.Router, h *handler.Handler) {
	r.Get("/contact", func(w http.ResponseWriter, r *http.Request) {
		// lazy, thread-safe template parse
		tplOnce.Do(func() { tpl = parseTemplates() })

		// minimal view-model; add role / user if menu logic needs it
		data := struct {
			Title      string
			Role       string
			IsLoggedIn bool
			UserName   string
		}{
			Title:      "Contact",
			Role:       h.Session.Role(r),
			IsLoggedIn: h.Session.IsAuth(r),
			UserName:   h.Session.UserName(r),
		}

		if err := tpl.ExecuteTemplate(w, "base", data); err != nil {
			logger.Error("contact: render failed", "err", err)
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	})
}
