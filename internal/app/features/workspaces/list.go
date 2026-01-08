// internal/app/features/workspaces/list.go
package workspaces

import (
	"context"
	"net/http"

	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// ServeList renders the workspace list page.
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	store := workspacestore.New(h.DB)

	// Parse pagination params
	after := query.Get(r, "after")
	before := query.Get(r, "before")

	// Count total workspaces
	total, err := store.Count(ctx, bson.M{})
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error counting workspaces", err, "A database error occurred.", "/")
		return
	}

	// Build filter with pagination
	filter := bson.M{}
	findOpts := options.Find().
		SetLimit(paging.LimitPlusOne()).
		SetProjection(bson.M{
			"_id":        1,
			"name":       1,
			"name_ci":    1,
			"subdomain":  1,
			"status":     1,
			"created_at": 1,
		})

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)
	cfg.ApplyToFind(findOpts, "name_ci")

	if window := cfg.KeysetWindow("name_ci"); window != nil {
		filter["$or"] = window["$or"]
	}

	// Fetch workspaces
	workspaces, err := store.Find(ctx, filter, findOpts)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error finding workspaces", err, "A database error occurred.", "/")
		return
	}

	// Apply pagination trimming
	page := paging.TrimPage(&workspaces, before, after)

	// Reverse if paging backwards
	if before != "" {
		paging.Reverse(workspaces)
	}

	// Collect workspace IDs for stats lookup
	wsIDs := make([]primitive.ObjectID, len(workspaces))
	for i, ws := range workspaces {
		wsIDs[i] = ws.ID
	}

	// Fetch user and org counts per workspace
	userCounts := h.fetchUserCounts(ctx, wsIDs)
	orgCounts := h.fetchOrgCounts(ctx, wsIDs)

	// Build rows
	rows := make([]workspaceRow, len(workspaces))
	for i, ws := range workspaces {
		rows[i] = workspaceRow{
			ID:          ws.ID.Hex(),
			Name:        ws.Name,
			Subdomain:   ws.Subdomain,
			Status:      ws.Status,
			UserCount:   userCounts[ws.ID],
			OrgCount:    orgCounts[ws.ID],
			CreatedAt:   ws.CreatedAt,
			StatusBadge: statusBadge(ws.Status),
		}
	}

	// Build cursors
	var prevCursor, nextCursor string
	if len(workspaces) > 0 {
		prevCursor = wafflemongo.EncodeCursor(text.Fold(workspaces[0].Name), workspaces[0].ID)
		nextCursor = wafflemongo.EncodeCursor(text.Fold(workspaces[len(workspaces)-1].Name), workspaces[len(workspaces)-1].ID)
	}

	data := listData{
		BaseVM:     viewdata.NewBaseVM(r, h.DB, "Workspaces", "/"),
		Rows:       rows,
		Total:      total,
		Domain:     h.PrimaryDomain,
		HasPrev:    page.HasPrev,
		HasNext:    page.HasNext,
		PrevCursor: prevCursor,
		NextCursor: nextCursor,
	}

	templates.Render(w, r, "workspace_list", data)
}

// fetchUserCounts returns user counts per workspace.
func (h *Handler) fetchUserCounts(ctx context.Context, wsIDs []primitive.ObjectID) map[primitive.ObjectID]int64 {
	counts := make(map[primitive.ObjectID]int64)
	if len(wsIDs) == 0 {
		return counts
	}

	pipeline := []bson.M{
		{"$match": bson.M{"workspace_id": bson.M{"$in": wsIDs}}},
		{"$group": bson.M{"_id": "$workspace_id", "count": bson.M{"$sum": 1}}},
	}

	cur, err := h.DB.Collection("users").Aggregate(ctx, pipeline)
	if err != nil {
		h.Log.Warn("failed to aggregate user counts", zap.Error(err))
		return counts
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var result struct {
			ID    primitive.ObjectID `bson:"_id"`
			Count int64              `bson:"count"`
		}
		if cur.Decode(&result) == nil {
			counts[result.ID] = result.Count
		}
	}
	return counts
}

// fetchOrgCounts returns organization counts per workspace.
func (h *Handler) fetchOrgCounts(ctx context.Context, wsIDs []primitive.ObjectID) map[primitive.ObjectID]int64 {
	counts := make(map[primitive.ObjectID]int64)
	if len(wsIDs) == 0 {
		return counts
	}

	pipeline := []bson.M{
		{"$match": bson.M{"workspace_id": bson.M{"$in": wsIDs}}},
		{"$group": bson.M{"_id": "$workspace_id", "count": bson.M{"$sum": 1}}},
	}

	cur, err := h.DB.Collection("organizations").Aggregate(ctx, pipeline)
	if err != nil {
		h.Log.Warn("failed to aggregate org counts", zap.Error(err))
		return counts
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var result struct {
			ID    primitive.ObjectID `bson:"_id"`
			Count int64              `bson:"count"`
		}
		if cur.Decode(&result) == nil {
			counts[result.ID] = result.Count
		}
	}
	return counts
}

// statusBadge returns a CSS class suffix for the status badge.
func statusBadge(status string) string {
	switch status {
	case "active":
		return "success"
	case "suspended":
		return "warning"
	case "archived":
		return "secondary"
	default:
		return "secondary"
	}
}

// ServeManageModal handles GET /workspaces/{id}/manage_modal and returns
// the snippet for the Manage Workspace modal.
func (h *Handler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		http.NotFound(w, r)
		return
	}

	wsID, err := primitive.ObjectIDFromHex(idParam)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	var ws struct {
		ID        primitive.ObjectID `bson:"_id"`
		Name      string             `bson:"name"`
		Subdomain string             `bson:"subdomain"`
		Status    string             `bson:"status"`
	}
	if err := h.DB.Collection("workspaces").FindOne(ctx, bson.M{"_id": wsID}).Decode(&ws); err != nil {
		http.NotFound(w, r)
		return
	}

	data := manageModalData{
		ID:        ws.ID.Hex(),
		Name:      ws.Name,
		Subdomain: ws.Subdomain,
		Status:    ws.Status,
		Domain:    h.PrimaryDomain,
	}

	templates.RenderSnippet(w, "workspace_manage_modal", data)
}
