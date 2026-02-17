package resources

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/resourcepolicy"
	resourcestore "github.com/dalemusser/stratahub/internal/app/store/resources"
	"github.com/dalemusser/stratahub/internal/app/system/htmlsanitize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/urlutil"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// getActivitySessionID extracts the activity session ID from the user's session cookie.
// Uses token-based lookup to get the session's MongoDB ObjectID.
func (h *MemberHandler) getActivitySessionID(r *http.Request) primitive.ObjectID {
	if h.SessionMgr == nil || h.Sessions == nil {
		return primitive.NilObjectID
	}
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		return primitive.NilObjectID
	}
	sessionToken, ok := sess.Values["session_token"].(string)
	if !ok || sessionToken == "" {
		return primitive.NilObjectID
	}
	// Look up session by token to get its ID
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	sessionDoc, err := h.Sessions.GetByToken(ctx, sessionToken)
	if err != nil || sessionDoc == nil {
		return primitive.NilObjectID
	}
	return sessionDoc.ID
}

// ServeViewResource handles GET /member/resources/{resourceID} for members.
// It enforces that the current user is a member, checks that the resource is
// currently available based on the group assignment visibility window, and
// then renders the member_resource_view template.
func (h *MemberHandler) ServeViewResource(w http.ResponseWriter, r *http.Request) {
	resourceID := chi.URLParam(r, "resourceID")
	oid, err := primitive.ObjectIDFromHex(resourceID)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/member/resources")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Verify member access using policy layer
	member, err := resourcepolicy.VerifyMemberAccess(ctx, db, r)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error verifying member access", err, "A database error occurred.", "/member/resources")
		return
	}
	if member == nil {
		uierrors.RenderNotFound(w, r, "Member not found.", "/login")
		return
	}

	orgName, loc, tzID := resolveMemberOrgLocation(ctx, db, member.OrganizationID)
	tzLabel := ""
	if tzID != "" {
		tzLabel = timezones.Label(tzID)
	}

	// "Now" in org-local time
	nowLocal := time.Now().In(loc)

	// Fetch the resource
	resStore := resourcestore.New(db)
	res, err := resStore.GetByID(ctx, oid)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Resource not found.", "/member/resources")
			return
		}
		h.ErrLog.LogServerError(w, r, "find resource failed", err, "A database error occurred.", "/member/resources")
		return
	}

	// Check if member has access to this resource via group assignment
	assignment, err := resourcepolicy.CanViewResource(ctx, db, member.ID, oid)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "policy check failed", err, "A database error occurred.", "/member/resources")
		return
	}

	groupName := ""
	instructions := ""
	var visibleFrom, visibleUntil *time.Time
	if assignment != nil {
		groupName = assignment.GroupName
		visibleFrom = assignment.VisibleFrom
		visibleUntil = assignment.VisibleUntil
		instructions = assignment.Instructions
	}
	// Fall back to resource's default instructions if assignment has none
	if instructions == "" {
		instructions = res.DefaultInstructions
	}

	// Build launch URL with id (login_id), group (name), org (name)
	launch := urlutil.AddOrSetQueryParams(res.LaunchURL, map[string]string{
		"id":    member.LoginID,
		"group": groupName,
		"org":   orgName,
	})

	canOpen := false
	availableUntil := ""

	if visibleFrom != nil && !visibleFrom.IsZero() {
		fromLocal := visibleFrom.In(loc)
		if !nowLocal.Before(fromLocal) {
			// We are at or after the start time.
			if visibleUntil != nil && !visibleUntil.IsZero() {
				untilLocal := visibleUntil.In(loc)
				availableUntil = untilLocal.Format("2006-01-02 15:04")
				if nowLocal.Before(untilLocal) {
					// In window.
					canOpen = (res.Status == "active")
				}
			} else {
				// No end date; started and still active.
				availableUntil = "No end date"
				canOpen = (res.Status == "active")
			}
		} else {
			// Not yet started.
			availableUntil = "No end date"
		}
	} else {
		// No visible_from: treat as not currently available.
		availableUntil = "No end date"
	}

	typeLabel := res.Type
	if typeLabel == "" {
		typeLabel = "Resource"
	} else {
		// capitalize first letter for display (e.g., "game" -> "Game")
		if len(typeLabel) > 1 {
			typeLabel = strings.ToUpper(typeLabel[:1]) + typeLabel[1:]
		} else {
			typeLabel = strings.ToUpper(typeLabel)
		}
	}

	// Record the resource view event
	activitySessionID := h.getActivitySessionID(r)
	if h.Activity != nil && !activitySessionID.IsZero() {
		if err := h.Activity.RecordResourceView(ctx, member.ID, activitySessionID, member.OrganizationID, oid, res.Title); err != nil {
			h.Log.Warn("failed to record resource view",
				zap.Error(err),
				zap.String("resource_id", resourceID),
				zap.String("member_id", member.ID.Hex()))
		}
	}

	data := viewResourceData{
		common: common{
			BaseVM: viewdata.NewBaseVM(r, h.DB, "View Resource", "/member/resources"),
			UserID: member.LoginID,
		},
		ResourceID:          res.ID.Hex(),
		ResourceTitle:       res.Title,
		Subject:             res.Subject,
		Type:                res.Type,
		TypeDisplay:         typeLabel,
		Description:         res.Description,
		DefaultInstructions: htmlsanitize.PrepareForDisplay(instructions),
		DisplayURL:          res.LaunchURL,
		LaunchURL:           launch,
		HasFile:             res.HasFile(),
		FileName:            res.FileName,
		Status:              res.Status,
		AvailableUntil:      availableUntil,
		BackURL:             "/member/resources",
		CanOpen:             canOpen,
		TimeZone:            tzLabel,
	}

	templates.Render(w, r, "member_resource_view", data)
}

// HandleDownload generates a signed URL and redirects to the file.
// For members, it verifies access before allowing download.
func (h *MemberHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	resourceID := chi.URLParam(r, "resourceID")
	oid, err := primitive.ObjectIDFromHex(resourceID)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/member/resources")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Verify member access using policy layer
	member, err := resourcepolicy.VerifyMemberAccess(ctx, db, r)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error verifying member access", err, "A database error occurred.", "/member/resources")
		return
	}
	if member == nil {
		uierrors.RenderNotFound(w, r, "Member not found.", "/login")
		return
	}

	_, loc, _ := resolveMemberOrgLocation(ctx, db, member.OrganizationID)
	nowLocal := time.Now().In(loc)

	// Fetch the resource
	resStore := resourcestore.New(db)
	res, err := resStore.GetByID(ctx, oid)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Resource not found.", "/member/resources")
			return
		}
		h.ErrLog.LogServerError(w, r, "find resource failed", err, "A database error occurred.", "/member/resources")
		return
	}

	// Check if member has access to this resource via group assignment
	assignment, err := resourcepolicy.CanViewResource(ctx, db, member.ID, oid)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "policy check failed", err, "A database error occurred.", "/member/resources")
		return
	}
	if assignment == nil {
		uierrors.RenderForbidden(w, r, "You do not have access to this resource.", "/member/resources")
		return
	}

	// Check visibility window
	if assignment.VisibleFrom != nil && !assignment.VisibleFrom.IsZero() {
		fromLocal := assignment.VisibleFrom.In(loc)
		if nowLocal.Before(fromLocal) {
			uierrors.RenderForbidden(w, r, "This resource is not yet available.", "/member/resources")
			return
		}
	} else {
		// No visible_from means not available
		uierrors.RenderForbidden(w, r, "This resource is not available.", "/member/resources")
		return
	}

	if assignment.VisibleUntil != nil && !assignment.VisibleUntil.IsZero() {
		untilLocal := assignment.VisibleUntil.In(loc)
		if nowLocal.After(untilLocal) {
			uierrors.RenderForbidden(w, r, "This resource is no longer available.", "/member/resources")
			return
		}
	}

	// Check if resource is active
	if res.Status != "active" {
		uierrors.RenderForbidden(w, r, "This resource is not currently active.", "/member/resources")
		return
	}

	// Record the resource launch event (for file downloads)
	activitySessionID := h.getActivitySessionID(r)
	if h.Activity != nil && !activitySessionID.IsZero() {
		if err := h.Activity.RecordResourceLaunch(ctx, member.ID, activitySessionID, member.OrganizationID, oid, res.Title); err != nil {
			h.Log.Warn("failed to record resource launch",
				zap.Error(err),
				zap.String("resource_id", resourceID),
				zap.String("member_id", member.ID.Hex()))
		}
	}

	// If resource has a file, serve it
	if res.HasFile() {
		// Build Content-Disposition header for proper filename
		filename := res.FileName
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
			fullPath, err := localStorage.GetFullPath(res.FilePath)
			if err != nil {
				h.Log.Error("error getting file path",
					zap.Error(err),
					zap.String("path", res.FilePath))
				uierrors.RenderServerError(w, r, "Failed to locate file.", "/member/resources")
				return
			}
			w.Header().Set("Content-Disposition", contentDisposition)
			http.ServeFile(w, r, fullPath)
			return
		}

		// For S3/other storage, generate signed URL and redirect
		signedURL, err := h.Storage.PresignedURL(ctx, res.FilePath, &storage.PresignOptions{
			Expires:            15 * time.Minute,
			ContentDisposition: contentDisposition,
		})
		if err != nil {
			h.Log.Error("error generating signed URL",
				zap.Error(err),
				zap.String("path", res.FilePath))
			uierrors.RenderServerError(w, r, "Failed to generate download link.", "/member/resources")
			return
		}
		http.Redirect(w, r, signedURL, http.StatusSeeOther)
		return
	}

	// If resource has a URL, redirect to it (with member info in query params)
	if res.LaunchURL != "" {
		orgName, _, _ := resolveMemberOrgLocation(ctx, db, member.OrganizationID)
		launch := urlutil.AddOrSetQueryParams(res.LaunchURL, map[string]string{
			"id":    member.LoginID,
			"group": assignment.GroupName,
			"org":   orgName,
		})
		http.Redirect(w, r, launch, http.StatusSeeOther)
		return
	}

	uierrors.RenderNotFound(w, r, "No file or URL available for this resource.", "/member/resources")
}

// HandleLaunch handles GET /member/resources/{resourceID}/launch for members.
// It records the resource launch event and redirects to the actual launch URL.
func (h *MemberHandler) HandleLaunch(w http.ResponseWriter, r *http.Request) {
	resourceID := chi.URLParam(r, "resourceID")
	oid, err := primitive.ObjectIDFromHex(resourceID)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/member/resources")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Verify member access using policy layer
	member, err := resourcepolicy.VerifyMemberAccess(ctx, db, r)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error verifying member access", err, "A database error occurred.", "/member/resources")
		return
	}
	if member == nil {
		uierrors.RenderNotFound(w, r, "Member not found.", "/login")
		return
	}

	orgName, loc, _ := resolveMemberOrgLocation(ctx, db, member.OrganizationID)
	nowLocal := time.Now().In(loc)

	// Fetch the resource
	resStore := resourcestore.New(db)
	res, err := resStore.GetByID(ctx, oid)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Resource not found.", "/member/resources")
			return
		}
		h.ErrLog.LogServerError(w, r, "find resource failed", err, "A database error occurred.", "/member/resources")
		return
	}

	// Check if member has access to this resource via group assignment
	assignment, err := resourcepolicy.CanViewResource(ctx, db, member.ID, oid)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "policy check failed", err, "A database error occurred.", "/member/resources")
		return
	}
	if assignment == nil {
		uierrors.RenderForbidden(w, r, "You do not have access to this resource.", "/member/resources")
		return
	}

	// Check visibility window
	if assignment.VisibleFrom != nil && !assignment.VisibleFrom.IsZero() {
		fromLocal := assignment.VisibleFrom.In(loc)
		if nowLocal.Before(fromLocal) {
			uierrors.RenderForbidden(w, r, "This resource is not yet available.", "/member/resources")
			return
		}
	} else {
		uierrors.RenderForbidden(w, r, "This resource is not available.", "/member/resources")
		return
	}

	if assignment.VisibleUntil != nil && !assignment.VisibleUntil.IsZero() {
		untilLocal := assignment.VisibleUntil.In(loc)
		if nowLocal.After(untilLocal) {
			uierrors.RenderForbidden(w, r, "This resource is no longer available.", "/member/resources")
			return
		}
	}

	// Check if resource is active
	if res.Status != "active" {
		uierrors.RenderForbidden(w, r, "This resource is not currently active.", "/member/resources")
		return
	}

	// Resource must have a launch URL
	if res.LaunchURL == "" {
		uierrors.RenderBadRequest(w, r, "This resource does not have a launch URL.", "/member/resources")
		return
	}

	// Record the resource launch event
	activitySessionID := h.getActivitySessionID(r)
	if h.Activity != nil && !activitySessionID.IsZero() {
		if err := h.Activity.RecordResourceLaunch(ctx, member.ID, activitySessionID, member.OrganizationID, oid, res.Title); err != nil {
			h.Log.Warn("failed to record resource launch",
				zap.Error(err),
				zap.String("resource_id", resourceID),
				zap.String("member_id", member.ID.Hex()))
		}
	}

	// Build launch URL with id (login_id), group (name), org (name)
	launch := urlutil.AddOrSetQueryParams(res.LaunchURL, map[string]string{
		"id":    member.LoginID,
		"group": assignment.GroupName,
		"org":   orgName,
	})

	h.Log.Info("resource launch redirect",
		zap.String("resource_id", resourceID),
		zap.String("original_url", res.LaunchURL),
		zap.String("redirect_url", launch),
		zap.String("member_login_id", member.LoginID),
		zap.String("group_name", assignment.GroupName),
		zap.String("org_name", orgName))

	http.Redirect(w, r, launch, http.StatusSeeOther)
}
