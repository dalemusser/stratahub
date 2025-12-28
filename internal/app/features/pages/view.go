// internal/app/features/pages/view.go
package pages

import (
	"context"
	"html/template"
	"net/http"

	pagestore "github.com/dalemusser/stratahub/internal/app/store/pages"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
)

type pageViewVM struct {
	viewdata.BaseVM
	Slug    string
	Content template.HTML
	CanEdit bool
}

// ServePage is a generic handler that serves a page by slug.
func (h *Handler) ServePage(slug, pageTitle string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
		defer cancel()

		store := pagestore.New(h.DB)
		page, err := store.GetBySlug(ctx, slug)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "load page failed", err, "Failed to load page.", "/")
			return
		}

		// Load base view data including site settings
		base := viewdata.NewBaseVM(r, h.DB, pageTitle, "/")

		vm := pageViewVM{
			BaseVM:  base,
			Slug:    slug,
			Content: template.HTML(page.Content),
			CanEdit: base.Role == "admin",
		}

		templates.Render(w, r, "page_view", vm)
	}
}

// ServeAbout displays the About page.
func (h *Handler) ServeAbout(w http.ResponseWriter, r *http.Request) {
	h.ServePage("about", "About")(w, r)
}

// ServeContact displays the Contact page.
func (h *Handler) ServeContact(w http.ResponseWriter, r *http.Request) {
	h.ServePage("contact", "Contact")(w, r)
}

// ServeTerms displays the Terms of Service page.
func (h *Handler) ServeTerms(w http.ResponseWriter, r *http.Request) {
	h.ServePage("terms-of-service", "Terms of Service")(w, r)
}

// ServePrivacy displays the Privacy Policy page.
func (h *Handler) ServePrivacy(w http.ResponseWriter, r *http.Request) {
	h.ServePage("privacy-policy", "Privacy Policy")(w, r)
}
