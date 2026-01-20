// internal/app/features/announcements/announcements.go
package announcements

import (
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/announcement"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// announcementRow represents an announcement in the list.
type announcementRow struct {
	ID          string
	Title       string
	Type        announcement.Type
	Active      bool
	Dismissible bool
	StartsAt    string
	EndsAt      string
}

// ListVM is the view model for the announcements list.
type ListVM struct {
	viewdata.BaseVM
	Items   []announcementRow // Named Items to avoid conflict with BaseVM.Announcements
	Success string
	Error   string
}

// List displays all announcements.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	announcements, err := h.Store.List(r.Context())
	if err != nil {
		h.Log.Error("failed to list announcements", zap.Error(err), zap.String("path", r.URL.Path))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	rows := make([]announcementRow, 0, len(announcements))
	for _, ann := range announcements {
		startsAt := ""
		if ann.StartsAt != nil {
			startsAt = ann.StartsAt.Format("Jan 2, 2006 3:04 PM")
		}
		endsAt := ""
		if ann.EndsAt != nil {
			endsAt = ann.EndsAt.Format("Jan 2, 2006 3:04 PM")
		}
		rows = append(rows, announcementRow{
			ID:          ann.ID.Hex(),
			Title:       ann.Title,
			Type:        ann.Type,
			Active:      ann.Active,
			Dismissible: ann.Dismissible,
			StartsAt:    startsAt,
			EndsAt:      endsAt,
		})
	}

	vm := ListVM{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "Announcements", "/"),
		Items:  rows,
	}

	switch r.URL.Query().Get("success") {
	case "created":
		vm.Success = "Announcement created successfully"
	case "updated":
		vm.Success = "Announcement updated successfully"
	case "deleted":
		vm.Success = "Announcement deleted"
	case "toggled":
		vm.Success = "Announcement status updated"
	}

	templates.Render(w, r, "announcements/list", vm)
}

// NewVM is the view model for creating a new announcement.
type NewVM struct {
	viewdata.BaseVM
	AnnTitle    string // renamed to avoid conflict with BaseVM.Title
	Content     string
	Type        string
	Dismissible bool
	Active      bool
	StartsAt    string
	EndsAt      string
	Error       string
}

// ShowNew displays the new announcement form.
func (h *Handler) ShowNew(w http.ResponseWriter, r *http.Request) {
	vm := NewVM{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "New Announcement", "/announcements"),
		Type:        "info",
		Dismissible: true,
		Active:      true,
	}

	templates.Render(w, r, "announcements/new", vm)
}

// Create creates a new announcement.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.Log.Error("failed to parse form", zap.Error(err), zap.String("path", r.URL.Path))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	annType := announcement.Type(r.FormValue("type"))
	dismissible := r.FormValue("dismissible") == "on"
	active := r.FormValue("active") == "on"

	if title == "" {
		vm := NewVM{
			BaseVM:      viewdata.NewBaseVM(r, h.DB, "New Announcement", "/announcements"),
			AnnTitle:    title,
			Content:     content,
			Type:        string(annType),
			Dismissible: dismissible,
			Active:      active,
			Error:       "Title is required",
		}
		templates.Render(w, r, "announcements/new", vm)
		return
	}

	input := announcement.CreateInput{
		Title:       title,
		Content:     content,
		Type:        annType,
		Dismissible: dismissible,
		Active:      active,
	}

	// Parse optional start/end times
	if startsAt := r.FormValue("starts_at"); startsAt != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", startsAt, time.Local); err == nil {
			input.StartsAt = &t
		}
	}
	if endsAt := r.FormValue("ends_at"); endsAt != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", endsAt, time.Local); err == nil {
			input.EndsAt = &t
		}
	}

	if _, err := h.Store.Create(r.Context(), input); err != nil {
		h.Log.Error("failed to create announcement", zap.Error(err), zap.String("path", r.URL.Path))
		vm := NewVM{
			BaseVM:      viewdata.NewBaseVM(r, h.DB, "New Announcement", "/announcements"),
			AnnTitle:    title,
			Content:     content,
			Type:        string(annType),
			Dismissible: dismissible,
			Active:      active,
			Error:       "Failed to create announcement",
		}
		templates.Render(w, r, "announcements/new", vm)
		return
	}

	http.Redirect(w, r, "/announcements?success=created", http.StatusSeeOther)
}

// EditVM is the view model for editing an announcement.
type EditVM struct {
	viewdata.BaseVM
	ID          string
	AnnTitle    string // renamed to avoid conflict with BaseVM.Title
	Content     string
	Type        string
	Dismissible bool
	Active      bool
	StartsAt    string
	EndsAt      string
	Error       string
}

// ManageModalVM is the view model for the manage modal.
type ManageModalVM struct {
	viewdata.BaseVM
	ID       string
	AnnTitle string
	Type     string
	Active   bool
}

// ShowVM is the view model for viewing an announcement.
type ShowVM struct {
	viewdata.BaseVM
	ID          string
	AnnTitle    string
	Content     string
	Type        string
	Dismissible bool
	Active      bool
	StartsAt    string
	EndsAt      string
}

// Show displays a single announcement.
func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ann, err := h.Store.GetByID(r.Context(), objID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	backURL := r.URL.Query().Get("return")
	if backURL == "" {
		backURL = "/announcements"
	}

	startsAt := ""
	if ann.StartsAt != nil {
		startsAt = ann.StartsAt.Format("Jan 2, 2006 3:04 PM")
	}
	endsAt := ""
	if ann.EndsAt != nil {
		endsAt = ann.EndsAt.Format("Jan 2, 2006 3:04 PM")
	}

	vm := ShowVM{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "View Announcement", backURL),
		ID:          id,
		AnnTitle:    ann.Title,
		Content:     ann.Content,
		Type:        string(ann.Type),
		Dismissible: ann.Dismissible,
		Active:      ann.Active,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
	}

	templates.Render(w, r, "announcements/show", vm)
}

// ManageModal displays the manage modal for an announcement.
func (h *Handler) ManageModal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ann, err := h.Store.GetByID(r.Context(), objID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	backURL := r.URL.Query().Get("return")
	if backURL == "" {
		backURL = "/announcements"
	}

	vm := ManageModalVM{
		BaseVM:   viewdata.NewBaseVM(r, h.DB, "", backURL),
		ID:       id,
		AnnTitle: ann.Title,
		Type:     string(ann.Type),
		Active:   ann.Active,
	}

	templates.RenderSnippet(w, "announcements/manage_modal", vm)
}

// ShowEdit displays the edit announcement form.
func (h *Handler) ShowEdit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ann, err := h.Store.GetByID(r.Context(), objID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	startsAt := ""
	if ann.StartsAt != nil {
		startsAt = ann.StartsAt.Format("2006-01-02T15:04")
	}
	endsAt := ""
	if ann.EndsAt != nil {
		endsAt = ann.EndsAt.Format("2006-01-02T15:04")
	}

	vm := EditVM{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "Edit Announcement", "/announcements"),
		ID:          id,
		AnnTitle:    ann.Title,
		Content:     ann.Content,
		Type:        string(ann.Type),
		Dismissible: ann.Dismissible,
		Active:      ann.Active,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
	}

	templates.Render(w, r, "announcements/edit", vm)
}

// Update updates an announcement.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.Log.Error("failed to parse form", zap.Error(err), zap.String("path", r.URL.Path))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	annType := announcement.Type(r.FormValue("type"))
	dismissible := r.FormValue("dismissible") == "on"
	active := r.FormValue("active") == "on"

	if title == "" {
		vm := EditVM{
			BaseVM:      viewdata.NewBaseVM(r, h.DB, "Edit Announcement", "/announcements"),
			ID:          id,
			AnnTitle:    title,
			Content:     content,
			Type:        string(annType),
			Dismissible: dismissible,
			Active:      active,
			Error:       "Title is required",
		}
		templates.Render(w, r, "announcements/edit", vm)
		return
	}

	input := announcement.UpdateInput{
		Title:       &title,
		Content:     &content,
		Type:        &annType,
		Dismissible: &dismissible,
		Active:      &active,
	}

	// Parse optional start/end times
	if startsAt := r.FormValue("starts_at"); startsAt != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", startsAt, time.Local); err == nil {
			input.StartsAt = &t
		}
	}
	if endsAt := r.FormValue("ends_at"); endsAt != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", endsAt, time.Local); err == nil {
			input.EndsAt = &t
		}
	}

	if err := h.Store.Update(r.Context(), objID, input); err != nil {
		h.Log.Error("failed to update announcement", zap.Error(err), zap.String("path", r.URL.Path))
		vm := EditVM{
			BaseVM:      viewdata.NewBaseVM(r, h.DB, "Edit Announcement", "/announcements"),
			ID:          id,
			AnnTitle:    title,
			Content:     content,
			Type:        string(annType),
			Dismissible: dismissible,
			Active:      active,
			Error:       "Failed to update announcement",
		}
		templates.Render(w, r, "announcements/edit", vm)
		return
	}

	http.Redirect(w, r, "/announcements?success=updated", http.StatusSeeOther)
}

// Toggle toggles the active status of an announcement.
func (h *Handler) Toggle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ann, err := h.Store.GetByID(r.Context(), objID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.Store.SetActive(r.Context(), objID, !ann.Active); err != nil {
		h.Log.Error("failed to toggle announcement", zap.Error(err), zap.String("path", r.URL.Path))
		http.Redirect(w, r, "/announcements?error=toggle_failed", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/announcements?success=toggled", http.StatusSeeOther)
}

// Delete deletes an announcement.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.Store.Delete(r.Context(), objID); err != nil {
		h.Log.Error("failed to delete announcement", zap.Error(err), zap.String("path", r.URL.Path))
		http.Redirect(w, r, "/announcements?error=delete_failed", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/announcements?success=deleted", http.StatusSeeOther)
}

// GetStore returns the underlying announcement store for use by other components.
func (h *Handler) GetStore() *announcement.Store {
	return h.Store
}

// ViewVM is the view model for the user-facing announcements view.
type ViewVM struct {
	viewdata.BaseVM
	Items []viewAnnouncementRow
}

// viewAnnouncementRow represents an announcement in the user view.
type viewAnnouncementRow struct {
	ID          string
	Title       string
	Content     string
	Type        string // info, warning, critical
	Dismissible bool
}

// ViewRoutes returns routes for the user-facing announcements view.
// These routes require authentication but not admin role.
func ViewRoutes(h *Handler, sessionMgr *auth.SessionManager) http.Handler {
	r := chi.NewRouter()
	r.Use(sessionMgr.RequireSignedIn)

	r.Get("/", h.viewAnnouncements)

	return r
}

// viewAnnouncements displays all active announcements for the user.
func (h *Handler) viewAnnouncements(w http.ResponseWriter, r *http.Request) {
	announcements, err := h.Store.GetActive(r.Context())
	if err != nil {
		h.Log.Error("failed to get active announcements", zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	rows := make([]viewAnnouncementRow, 0, len(announcements))
	for _, ann := range announcements {
		rows = append(rows, viewAnnouncementRow{
			ID:          ann.ID.Hex(),
			Title:       ann.Title,
			Content:     ann.Content,
			Type:        string(ann.Type),
			Dismissible: ann.Dismissible,
		})
	}

	vm := ViewVM{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "Announcements", "/dashboard"),
		Items:  rows,
	}

	templates.Render(w, r, "announcements/view", vm)
}
