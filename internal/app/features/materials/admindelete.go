// internal/app/features/materials/admindelete.go
package materials

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	materialassignstore "github.com/dalemusser/stratahub/internal/app/store/materialassign"
	materialstore "github.com/dalemusser/stratahub/internal/app/store/materials"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// HandleDelete deletes a material, its assignments, and its file (if any).
func (h *AdminHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	// Check if user can manage materials (admin or coordinator with permission)
	if !authz.CanManageMaterials(r) {
		http.Redirect(w, r, "/materials", http.StatusSeeOther)
		return
	}

	actorRole, _, actorID, _ := authz.UserCtx(r)

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid material ID.", "/materials")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	matStore := materialstore.New(db)
	assignStore := materialassignstore.New(db)

	// Get material first to check for file
	mat, err := matStore.GetByID(ctx, oid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
		return
	}

	// Use transaction for atomic deletion of material and assignments.
	if err := txn.Run(ctx, db, h.Log, func(ctx context.Context) error {
		// 1) Clean up assignments first
		if _, err := assignStore.DeleteByMaterial(ctx, oid); err != nil {
			return err
		}
		// 2) Delete material
		if _, err := matStore.Delete(ctx, oid); err != nil {
			return err
		}
		return nil
	}); err != nil {
		h.ErrLog.LogServerError(w, r, "delete material failed", err, "Unable to delete material.", "")
		return
	}

	// Audit log: material deleted (before file cleanup, after DB success)
	h.AuditLog.MaterialDeleted(ctx, r, actorID, oid, actorRole, mat.Title)

	// 3) Delete file from storage (outside transaction, best effort)
	if mat.HasFile() {
		if err := h.Storage.Delete(ctx, mat.FilePath); err != nil {
			h.Log.Warn("failed to delete material file",
				zap.String("material_id", oid.Hex()),
				zap.String("file_path", mat.FilePath),
				zap.Error(err))
		}
	}

	// HTMX flow: redirect via HX-Redirect
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/materials")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Normal redirect
	ret := urlutil.SafeReturn(r.FormValue("return"), idHex, "/materials")
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
