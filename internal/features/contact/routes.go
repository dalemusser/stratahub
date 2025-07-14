package contact

import (
	"embed"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dalemusser/stratahub/internal/layout" // shared base + menu
	"github.com/dalemusser/stratahub/internal/platform/handler"
)

// ────────────────────────────── Embedded slice templates ──────────────────────
//

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
		panic("contact: layout parse failed: " + err.Error())
	}

	// 2) slice-specific templates
	if _, err := t.ParseFS(views, "templates/*.html"); err != nil {
		panic("contact: slice parse failed: " + err.Error())
	}
	return t
}

// ────────────────────────────── Route registration ────────────────────────────

// MountRoutes attaches GET /contact.
func MountRoutes(r chi.Router, h *handler.Handler) {
	r.Get("/contact", func(w http.ResponseWriter, r *http.Request) {

		// lazy, thread-safe template parse
		tplOnce.Do(func() { tpl = parseTemplates() })

		// minimal view-model; add Role / UserName if you need the menu logic
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
			http.Error(w, "template error: "+err.Error(),
				http.StatusInternalServerError)
		}
	})
}
