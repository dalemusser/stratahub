// internal/app/features/organizations/delete.go
package organizations

import (
	"context"
	"net/http"
	"strings"

	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"

	"github.com/go-chi/chi/v5"
)

// HandleDelete deletes an organization and redirects back to the list
// (or to a caller-provided return URL if present).
//
// Route: POST /organizations/{id}/delete
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad organization id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), orgsMedTimeout)
	defer cancel()

	db := h.DB

	res, err := db.Collection("organizations").DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		h.Log.Error("delete organization failed", zap.Error(err), zap.String("org_id", idHex))
		http.Error(w, "delete error", http.StatusInternalServerError)
		return
	}

	// Optional: if you want, you can show a flash when nothing was deleted.
	if res.DeletedCount == 0 {
		h.Log.Info("organization delete: no document found (idempotent)", zap.String("org_id", idHex))
	}

	// Honor explicit return if provided; otherwise go back to the list.
	ret := strings.TrimSpace(r.FormValue("return"))
	if ret == "" || !strings.HasPrefix(ret, "/") {
		// Fall back to a safe default using the same pattern as elsewhere.
		ret = nav.ResolveBackURL(r, "/organizations")
	}

	// For HTMX, we can either:
	//   - return a redirect,
	//   - or re-render the list. To keep it simple and consistent with other
	//     features, just use an HTTP redirect here.
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
