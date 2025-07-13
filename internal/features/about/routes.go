package about

import (
	"embed"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	// shared layout (base.html, footer partials, …)
	"github.com/dalemusser/stratahub/internal/layout"
	"github.com/dalemusser/stratahub/internal/platform/handler"
)

// --------------------------------------------------------------------
// 1.  Slice-local templates
// --------------------------------------------------------------------

//go:embed templates/*.html
var views embed.FS

// lazy-parsed template set
var (
	tplOnce sync.Once
	tpl     *template.Template
)

func parseTemplates() *template.Template {
	funcs := template.FuncMap{"now": time.Now}

	// 1) Parse the shared layout first.
	t, err := template.New("").
		Funcs(funcs).
		ParseFS(layout.Views, "templates/*.html")
	if err != nil {
		panic("about: layout parse failed: " + err.Error())
	}

	// 2) Add this slice’s templates.
	if _, err := t.ParseFS(views, "templates/*.html"); err != nil {
		panic("about: slice parse failed: " + err.Error())
	}
	return t
}

// --------------------------------------------------------------------
// 2.  Route registration
// --------------------------------------------------------------------

// MountRoutes attaches GET /about.
func MountRoutes(r chi.Router, h *handler.Handler) {
	r.Get("/about", func(w http.ResponseWriter, r *http.Request) {
		// Parse & cache once.
		tplOnce.Do(func() { tpl = parseTemplates() })

		data := map[string]any{
			"title": "About StrataHub",
			// you can add h.Session or user data here later
		}
		// --- Render the shared layout ("base") ---
		if err := tpl.ExecuteTemplate(w, "base", data); err != nil {
			http.Error(w, "template error: "+err.Error(),
				http.StatusInternalServerError)
		}
	})
}
