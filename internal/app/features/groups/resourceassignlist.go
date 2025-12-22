// internal/app/features/groups/resourceassignlist.go
package groups

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// ServeAssignResources renders the full Assign Resources page for a group.
func (h *Handler) ServeAssignResources(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(assign)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	q := r.URL.Query().Get("q")
	typeFilter := r.URL.Query().Get("type")
	after := r.URL.Query().Get("after")
	before := r.URL.Query().Get("before")

	assigned, avail, shown, total, nextCur, prevCur, hasNext, hasPrev, err := h.buildAssignments(ctx, group, q, typeFilter, after, before)
	if err != nil {
		h.Log.Warn("buildAssignments", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"))
		return
	}

	back := urlutil.SafeReturn(r.FormValue("return"), "", "")
	if back == "" {
		back = urlutil.SafeReturn(r.URL.Query().Get("return"), "", "/groups")
	}

	templates.RenderAutoMap(w, r, "group_manage_resources", nil, assignmentListData{
		Title:          "Assign Resources",
		IsLoggedIn:     true,
		Role:           role,
		UserName:       uname,
		GroupID:        group.ID.Hex(),
		GroupName:      group.Name,
		Assigned:       assigned,
		Available:      avail,
		AvailableShown: shown,
		AvailableTotal: total,
		Query:          q,
		TypeFilter:     typeFilter,
		TypeOptions:    models.ResourceTypes,
		CurrentAfter:   after,
		CurrentBefore:  before,
		NextCursor:     nextCur,
		PrevCursor:     prevCur,
		HasNext:        hasNext,
		HasPrev:        hasPrev,
		BackURL:        back,
		CurrentPath:    httpnav.CurrentPath(r),
	})
}

// ServeSearchResources serves only the Available Resources block (for HTMX search/paging).
func (h *Handler) ServeSearchResources(w http.ResponseWriter, r *http.Request) {
	// If this is a normal (non-HTMX) request, render the full page instead of a bare snippet.
	if r.Header.Get("HX-Request") != "true" {
		h.ServeAssignResources(w, r)
		return
	}

	gid := chi.URLParam(r, "id")
	q := r.URL.Query().Get("q")
	typeFilter := r.URL.Query().Get("type")
	after := r.URL.Query().Get("after")
	before := r.URL.Query().Get("before")

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	assigned, avail, shown, total, nextCur, prevCur, hasNext, hasPrev, err := h.buildAssignments(ctx, group, q, typeFilter, after, before)
	if err != nil {
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"))
		return
	}

	back := urlutil.SafeReturn(r.FormValue("return"), "", "")
	if back == "" {
		back = urlutil.SafeReturn(r.URL.Query().Get("return"), "", "/groups")
	}

	data := assignmentListData{
		Title:          "Assign Resources",
		IsLoggedIn:     true,
		GroupID:        group.ID.Hex(),
		GroupName:      group.Name,
		Assigned:       assigned,
		Available:      avail,
		AvailableShown: shown,
		AvailableTotal: total,
		Query:          q,
		TypeFilter:     typeFilter,
		TypeOptions:    models.ResourceTypes,
		CurrentAfter:   after,
		CurrentBefore:  before,
		NextCursor:     nextCur,
		PrevCursor:     prevCur,
		HasNext:        hasNext,
		HasPrev:        hasPrev,
		BackURL:        back,
	}

	templates.RenderSnippet(w, "group_available_resources_block", data)
}

// redirectAssignResources redirects (or HTMX-navigates) back to the
// /assign_resources page for the given group, preserving any ?return=...
// parameter when present.
func (h *Handler) redirectAssignResources(w http.ResponseWriter, r *http.Request, gid string) {
	dest := "/groups/" + gid + "/assign_resources"

	// preserve a ?return=... if present (validated to prevent open redirects)
	if ret := urlutil.SafeReturn(r.FormValue("return"), "", ""); ret != "" {
		dest = dest + "?return=" + url.QueryEscape(ret)
	} else if ret := urlutil.SafeReturn(r.URL.Query().Get("return"), "", ""); ret != "" {
		dest = dest + "?return=" + url.QueryEscape(ret)
	}

	if r.Header.Get("HX-Request") == "true" {
		// For HTMX, prefer client-side navigation instead of plain 303.
		// Use json.Marshal to safely escape the path value.
		hxLoc := struct {
			Path   string `json:"path"`
			Target string `json:"target"`
			Swap   string `json:"swap"`
		}{
			Path:   dest,
			Target: "#content",
			Swap:   "innerHTML",
		}
		if hxJSON, err := json.Marshal(hxLoc); err == nil {
			w.Header().Set("HX-Location", string(hxJSON))
		}
		w.Header().Set("HX-Push-Url", "true")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, dest, http.StatusSeeOther)
}
