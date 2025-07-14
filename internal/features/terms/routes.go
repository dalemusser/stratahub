package terms

import (
	"embed"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dalemusser/stratahub/internal/layout" // base + menu
	"github.com/dalemusser/stratahub/internal/platform/handler"
)

/*─────────────────── embedded slice templates ───────────────────*/

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
		panic("terms: layout parse failed: " + err.Error())
	}

	// 2) this slice’s templates
	if _, err := t.ParseFS(views, "templates/*.html"); err != nil {
		panic("terms: slice parse failed: " + err.Error())
	}
	return t
}

/*────────────────────── route registration ─────────────────────*/

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
			http.Error(w, "template error: "+err.Error(),
				http.StatusInternalServerError)
		}
	})
}
