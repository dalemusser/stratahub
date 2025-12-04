package leaders

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const deleteTimeout = 10 * time.Second

// HandleDelete removes a leader and all of their group memberships.
// It is mounted on POST /leaders/{id}/delete.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), deleteTimeout)
	defer cancel()

	// Remove ALL memberships for this user (defensive: leader/member)
	if _, err := h.DB.Collection("group_memberships").DeleteMany(ctx, bson.M{"user_id": uid}); err != nil {
		http.Error(w, "delete memberships error", http.StatusInternalServerError)
		return
	}

	// Finally delete the user (role: leader)
	if _, err := h.DB.Collection("users").DeleteOne(ctx, bson.M{"_id": uid, "role": "leader"}); err != nil {
		http.Error(w, "delete user error", http.StatusInternalServerError)
		return
	}

	// Optional return parameter, otherwise send back to leaders list.
	if ret := strings.TrimSpace(r.FormValue("return")); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/leaders", http.StatusSeeOther)
}
