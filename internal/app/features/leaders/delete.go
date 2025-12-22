package leaders

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HandleDelete removes a leader and all of their group memberships.
// It is mounted on POST /leaders/{id}/delete.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid leader ID.", "/leaders")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Use transaction for atomic deletion of memberships and leader.
	if err := txn.Run(ctx, h.DB, h.Log, func(ctx context.Context) error {
		// 1) Remove ALL memberships for this user (defensive: leader/member)
		if _, err := h.DB.Collection("group_memberships").DeleteMany(ctx, bson.M{"user_id": uid}); err != nil {
			return err
		}
		// 2) Delete the user (role: leader)
		if _, err := h.DB.Collection("users").DeleteOne(ctx, bson.M{"_id": uid, "role": "leader"}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		uierrors.RenderServerError(w, r, "Failed to delete leader.", "/leaders")
		return
	}

	// Optional return parameter, otherwise send back to leaders list.
	ret := navigation.SafeBackURL(r, navigation.LeadersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
