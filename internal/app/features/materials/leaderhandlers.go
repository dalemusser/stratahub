// internal/app/features/materials/leaderhandlers.go
package materials

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/queries/leadermaterials"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// LeaderMaterialsBackURL provides options for leader materials pages.
var LeaderMaterialsBackURL = navigation.BackURLOptions{
	AllowedPrefix:    "/leader/materials",
	ExcludedSubpaths: []string{"/download"},
	Fallback:         "/leader/materials",
}

// ServeListMaterials renders the "My Materials" list for the current leader.
func (h *LeaderHandler) ServeListMaterials(w http.ResponseWriter, r *http.Request) {
	_, _, userID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderForbidden(w, r, "You must be logged in.", "/login")
		return
	}

	// Get session user for organization ID
	sessionUser, ok := auth.CurrentUser(r)
	if !ok || sessionUser.OrganizationID == "" {
		uierrors.RenderServerError(w, r, "Unable to determine your organization.", "/dashboard")
		return
	}

	orgID, err := primitive.ObjectIDFromHex(sessionUser.OrganizationID)
	if err != nil {
		uierrors.RenderServerError(w, r, "Invalid organization.", "/dashboard")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Fetch materials for this leader
	materials, err := leadermaterials.ListActiveMaterialsForLeader(ctx, h.DB, userID, orgID)
	if err != nil {
		h.Log.Error("error fetching leader materials", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load materials.", "/dashboard")
		return
	}

	// Build list items
	items := make([]leaderMaterialItem, 0, len(materials))
	now := time.Now()
	for _, m := range materials {
		// Check visibility window
		isVisible := true
		if m.VisibleFrom != nil && now.Before(*m.VisibleFrom) {
			isVisible = false
		}
		if m.VisibleUntil != nil && now.After(*m.VisibleUntil) {
			isVisible = false
		}

		// Only show currently visible materials
		if !isVisible {
			continue
		}

		// Build launch URL with id parameter (leader's login ID)
		launchURL := m.Material.LaunchURL
		if launchURL != "" {
			launchURL = urlutil.AddOrSetQueryParams(launchURL, map[string]string{
				"id": sessionUser.LoginID,
			})
		}

		item := leaderMaterialItem{
			ID:            m.Material.ID.Hex(),
			Title:         m.Material.Title,
			Subject:       m.Material.Subject,
			Type:          m.Material.Type,
			HasFile:       m.Material.FilePath != "",
			LaunchURL:     launchURL,
			Description:   m.Material.Description,
			Directions:    template.HTML(m.Directions),
			HasDirections: strings.TrimSpace(m.Directions) != "",
			IsOrgWide:     m.IsOrgWide,
			OrgName:       m.OrgName,
		}

		if m.VisibleUntil != nil {
			item.AvailableUntil = m.VisibleUntil.Format("Jan 2, 2006")
		}

		items = append(items, item)
	}

	data := leaderMaterialsListData{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "My Materials", "/dashboard"),
		Materials: items,
	}

	templates.RenderAutoMap(w, r, "leader_materials_list", nil, data)
}

// ServeViewMaterial renders a single material detail page for a leader.
func (h *LeaderHandler) ServeViewMaterial(w http.ResponseWriter, r *http.Request) {
	_, _, userID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderForbidden(w, r, "You must be logged in.", "/login")
		return
	}

	// Get session user for organization ID
	sessionUser, ok := auth.CurrentUser(r)
	if !ok || sessionUser.OrganizationID == "" {
		uierrors.RenderServerError(w, r, "Unable to determine your organization.", "/dashboard")
		return
	}

	orgID, err := primitive.ObjectIDFromHex(sessionUser.OrganizationID)
	if err != nil {
		uierrors.RenderServerError(w, r, "Invalid organization.", "/dashboard")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get material ID from URL
	idStr := chi.URLParam(r, "materialID")
	matID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/leader/materials")
		return
	}

	// Fetch the material with access check
	result, err := leadermaterials.GetMaterialForLeader(ctx, h.DB, userID, orgID, matID)
	if err != nil {
		h.Log.Error("error fetching material for leader", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load material.", "/leader/materials")
		return
	}
	if result == nil {
		uierrors.RenderNotFound(w, r, "Material not found or you don't have access.", "/leader/materials")
		return
	}

	// Check visibility window
	now := time.Now()
	canOpen := true
	if result.VisibleFrom != nil && now.Before(*result.VisibleFrom) {
		canOpen = false
	}
	if result.VisibleUntil != nil && now.After(*result.VisibleUntil) {
		canOpen = false
	}

	// Format available until
	availableUntil := ""
	if result.VisibleUntil != nil {
		availableUntil = result.VisibleUntil.Format("Jan 2, 2006")
	}

	// Get type display name
	typeDisplay := result.Material.Type
	for _, opt := range materialTypeOptions() {
		if opt.ID == result.Material.Type {
			typeDisplay = opt.Label
			break
		}
	}

	// Build launch URL with id parameter (leader's login ID)
	launchURL := result.Material.LaunchURL
	if launchURL != "" {
		launchURL = urlutil.AddOrSetQueryParams(launchURL, map[string]string{
			"id": sessionUser.LoginID,
		})
	}

	data := leaderMaterialViewData{
		BaseVM:         viewdata.NewBaseVM(r, h.DB, "View Material", navigation.SafeBackURL(r, LeaderMaterialsBackURL)),
		MaterialID:     result.Material.ID.Hex(),
		MaterialTitle:  result.Material.Title,
		Subject:        result.Material.Subject,
		Type:           result.Material.Type,
		TypeDisplay:    typeDisplay,
		Description:    result.Material.Description,
		Directions:     template.HTML(result.Directions),
		DisplayURL:     result.Material.LaunchURL,
		LaunchURL:      launchURL,
		HasFile:        result.Material.FilePath != "",
		FileName:       result.Material.FileName,
		Status:         result.Material.Status,
		AvailableUntil: availableUntil,
		CanOpen:        canOpen,
	}

	templates.RenderAutoMap(w, r, "leader_materials_view", nil, data)
}

// HandleDownload generates a signed URL and redirects to the file.
func (h *LeaderHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	_, _, userID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderForbidden(w, r, "You must be logged in.", "/login")
		return
	}

	// Get session user for organization ID
	sessionUser, ok := auth.CurrentUser(r)
	if !ok || sessionUser.OrganizationID == "" {
		uierrors.RenderServerError(w, r, "Unable to determine your organization.", "/dashboard")
		return
	}

	orgID, err := primitive.ObjectIDFromHex(sessionUser.OrganizationID)
	if err != nil {
		uierrors.RenderServerError(w, r, "Invalid organization.", "/dashboard")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get material ID from URL
	idStr := chi.URLParam(r, "materialID")
	matID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/leader/materials")
		return
	}

	// Fetch the material with access check
	result, err := leadermaterials.GetMaterialForLeader(ctx, h.DB, userID, orgID, matID)
	if err != nil {
		h.Log.Error("error fetching material for download", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load material.", "/leader/materials")
		return
	}
	if result == nil {
		uierrors.RenderNotFound(w, r, "Material not found or you don't have access.", "/leader/materials")
		return
	}

	// Check visibility window
	now := time.Now()
	if result.VisibleFrom != nil && now.Before(*result.VisibleFrom) {
		uierrors.RenderForbidden(w, r, "This material is not yet available.", "/leader/materials")
		return
	}
	if result.VisibleUntil != nil && now.After(*result.VisibleUntil) {
		uierrors.RenderForbidden(w, r, "This material is no longer available.", "/leader/materials")
		return
	}

	// Check if material has a file
	if result.Material.FilePath == "" {
		// If it's a URL, redirect to it with id param
		if result.Material.LaunchURL != "" {
			launch := urlutil.AddOrSetQueryParams(result.Material.LaunchURL, map[string]string{
				"id": sessionUser.LoginID,
			})
			http.Redirect(w, r, launch, http.StatusSeeOther)
			return
		}
		uierrors.RenderNotFound(w, r, "No file available for this material.", "/leader/materials")
		return
	}

	// Build Content-Disposition header for proper filename
	filename := result.Material.FileName
	if filename == "" {
		filename = "download"
	}
	contentDisposition := "attachment; filename=\"" + filename + "\""

	// Prevent browser caching of downloads (important when files are replaced)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Check if local storage - serve file directly
	if localStorage, ok := h.Storage.(*storage.Local); ok {
		fullPath, err := localStorage.GetFullPath(result.Material.FilePath)
		if err != nil {
			h.Log.Error("error getting file path",
				zap.Error(err),
				zap.String("path", result.Material.FilePath))
			uierrors.RenderServerError(w, r, "Failed to locate file.", "/leader/materials")
			return
		}
		w.Header().Set("Content-Disposition", contentDisposition)
		http.ServeFile(w, r, fullPath)
		return
	}

	// For S3/other storage, generate signed URL and redirect
	signedURL, err := h.Storage.PresignedURL(ctx, result.Material.FilePath, &storage.PresignOptions{
		Expires:            15 * time.Minute,
		ContentDisposition: contentDisposition,
	})
	if err != nil {
		h.Log.Error("error generating signed URL",
			zap.Error(err),
			zap.String("path", result.Material.FilePath))
		uierrors.RenderServerError(w, r, "Failed to generate download link.", "/leader/materials")
		return
	}

	// Redirect to the signed URL
	http.Redirect(w, r, signedURL, http.StatusSeeOther)
}

// materialTypeLabel returns the display label for a material type.
func materialTypeLabel(typeID string) string {
	for _, t := range models.MaterialTypes {
		if t == typeID {
			// Convert to title case
			return strings.Title(strings.ToLower(typeID))
		}
	}
	return typeID
}
