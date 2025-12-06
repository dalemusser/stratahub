// internal/app/features/resources/admindelete.go
package resources

import (
	"context"
	"net/http"

	webutil "github.com/dalemusser/waffle/toolkit/http/webutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *AdminHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), resourcesMedTimeout)
	defer cancel()
	db := h.DB

	// Best-effort cleanup of assignments
	_, _ = db.Collection("group_resource_assignments").DeleteMany(ctx, bson.M{"resource_id": oid})

	// Delete resource
	if _, err := db.Collection("resources").DeleteOne(ctx, bson.M{"_id": oid}); err != nil {
		http.Error(w, "delete error", http.StatusInternalServerError)
		return
	}

	// HTMX flow: redirect via HX-Redirect
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/resources")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Normal redirect
	ret := webutil.SafeReturn(r.FormValue("return"), idHex, "/resources")
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
