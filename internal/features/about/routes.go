// internal/features/about/routes.go
package about

import (
	"embed"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/dalemusser/gowebcore/logger"
	"github.com/go-chi/chi/v5"

	// shared layout (base.html, menu, footer …)
	"github.com/dalemusser/stratahub/internal/layout"
	"github.com/dalemusser/stratahub/internal/platform/handler"
)

/*─────────────────────────── slice-local templates ──────────────────────────*/

//go:embed templates/*.html
var views embed.FS

var (
	tplOnce sync.Once
	tpl     *template.Template
)

func parseTemplates() *template.Template {
	funcs := template.FuncMap{"now": time.Now}

	// 1) shared layout / partials
	t, err := template.New("").Funcs(funcs).
		ParseFS(layout.Views, "templates/*.html")
	if err != nil {
		logger.Error("about: layout parse failed", "err", err)
		panic(err)
	}

	// 2) this slice’s templates
	if _, err := t.ParseFS(views, "templates/*.html"); err != nil {
		logger.Error("about: slice parse failed", "err", err)
		panic(err)
	}
	return t
}

/*──────────────────────────── route registration ───────────────────────────*/

// MountRoutes attaches GET /about to the router.
func MountRoutes(r chi.Router, h *handler.Handler) {
	r.Get("/about", func(w http.ResponseWriter, r *http.Request) {
		// parse & cache once
		tplOnce.Do(func() { tpl = parseTemplates() })

		data := map[string]any{
			"title": "About StrataHub",
			// add user/session info when needed
		}

		if err := tpl.ExecuteTemplate(w, "base", data); err != nil {
			logger.Error("about: render failed", "err", err)
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	})
}
