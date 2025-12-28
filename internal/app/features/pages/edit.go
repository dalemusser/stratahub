// internal/app/features/pages/edit.go
package pages

import (
	"context"
	"net/http"
	"strings"

	pagestore "github.com/dalemusser/stratahub/internal/app/store/pages"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/htmlsanitize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type pageEditVM struct {
	viewdata.BaseVM
	Slug      string
	PageTitle string
	Content   string
	Error     string
}

// pageDisplayName returns a human-friendly name for a page slug.
func pageDisplayName(slug string) string {
	switch slug {
	case "about":
		return "About"
	case "contact":
		return "Contact"
	case "terms-of-service":
		return "Terms of Service"
	case "privacy-policy":
		return "Privacy Policy"
	default:
		return strings.Title(strings.ReplaceAll(slug, "-", " "))
	}
}

// ServeEdit displays the page edit form.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		h.ErrLog.LogBadRequest(w, r, "missing slug", nil, "Page not specified.", "/dashboard")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	store := pagestore.New(h.DB)
	page, err := store.GetBySlug(ctx, slug)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "load page failed", err, "Failed to load page.", "/dashboard")
		return
	}

	vm := pageEditVM{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Edit "+pageDisplayName(slug), "/dashboard"),
		Slug:      slug,
		PageTitle: page.Title,
		Content:   page.Content,
	}

	templates.Render(w, r, "page_edit", vm)
}

// HandleEdit processes the page edit form submission.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		h.ErrLog.LogBadRequest(w, r, "missing slug", nil, "Page not specified.", "/dashboard")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/pages/"+slug+"/edit")
		return
	}

	pageTitle := strings.TrimSpace(r.FormValue("page_title"))
	content := htmlsanitize.Sanitize(strings.TrimSpace(r.FormValue("content")))

	// Validation
	if pageTitle == "" {
		h.renderEditWithError(w, r, slug, content, "Page title is required.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()

	store := pagestore.New(h.DB)

	// Get user info for audit
	_, uname, memberID, _ := authz.UserCtx(r)

	// Update the page
	page := models.Page{
		Slug:          slug,
		Title:         pageTitle,
		Content:       content,
		UpdatedByID:   &memberID,
		UpdatedByName: uname,
	}

	if err := store.Upsert(ctx, page); err != nil {
		h.Log.Error("failed to save page", zap.String("slug", slug), zap.Error(err))
		h.renderEditWithError(w, r, slug, content, "Failed to save page.")
		return
	}

	// Redirect back to the page view
	redirectURL := slugToViewURL(slug)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// slugToViewURL returns the public URL for a page slug.
func slugToViewURL(slug string) string {
	switch slug {
	case "about":
		return "/about"
	case "contact":
		return "/contact"
	case "terms-of-service":
		return "/terms"
	case "privacy-policy":
		return "/privacy"
	default:
		return "/"
	}
}

func (h *Handler) renderEditWithError(w http.ResponseWriter, r *http.Request, slug, content, errMsg string) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	store := pagestore.New(h.DB)
	page, _ := store.GetBySlug(ctx, slug)

	// Use the submitted content if provided, otherwise fall back to stored content
	if content == "" {
		content = page.Content
	}

	vm := pageEditVM{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Edit "+pageDisplayName(slug), "/dashboard"),
		Slug:      slug,
		PageTitle: page.Title,
		Content:   content,
		Error:     errMsg,
	}

	templates.Render(w, r, "page_edit", vm)
}
