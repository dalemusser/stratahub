// internal/app/features/resources/admindelete.go
package resources

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	resourcestore "github.com/dalemusser/stratahub/internal/app/store/resources"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// HandleDelete deletes a resource and its assignments.
func (h *AdminHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	// Check if user can manage resources (admin or coordinator with permission)
	if !authz.CanManageResources(r) {
		http.Redirect(w, r, "/resources", http.StatusSeeOther)
		return
	}

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/resources")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	rasStore := resourceassignstore.New(db)
	resStore := resourcestore.New(db)

	// Get resource to check for file
	res, err := resStore.GetByID(ctx, oid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Resource not found.", "/resources")
		return
	}

	// Use transaction for atomic deletion of resource and assignments.
	if err := txn.Run(ctx, db, h.Log, func(ctx context.Context) error {
		// 1) Clean up assignments first
		if _, err := rasStore.DeleteByResource(ctx, oid); err != nil {
			return err
		}
		// 2) Delete resource
		if _, err := resStore.Delete(ctx, oid); err != nil {
			return err
		}
		return nil
	}); err != nil {
		h.ErrLog.LogServerError(w, r, "delete resource failed", err, "Unable to delete resource.", "")
		return
	}

	// Delete file from storage if exists (after successful DB deletion)
	if res.HasFile() {
		if err := h.Storage.Delete(ctx, res.FilePath); err != nil {
			h.Log.Warn("failed to delete file after resource deletion",
				zap.String("path", res.FilePath),
				zap.Error(err))
		}
	}

	// HTMX flow: redirect via HX-Redirect
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/resources")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Normal redirect
	ret := urlutil.SafeReturn(r.FormValue("return"), idHex, "/resources")
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
