// internal/app/features/resources/adminlist.go
package resources

import (
	"context"
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/templates"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.uber.org/zap"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServeList displays the admin list of resources.
// Supports live HTMX search and prefix queries on *_ci columns.
func (h *AdminHandler) ServeList(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Extra safety: only admins should be here (routes should already enforce this).
	if !authz.IsAdmin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), resourcesMedTimeout)
	defer cancel()
	db := h.DB

	q := strings.TrimSpace(r.URL.Query().Get("q"))

	filter := bson.M{}
	if lo, hi := textfold.PrefixRange(q); lo != "" {
		filter["$or"] = []bson.M{
			{"title_ci": bson.M{"$gte": lo, "$lt": hi}},
			{"subject_ci": bson.M{"$gte": lo, "$lt": hi}},
		}
	}

	findOpts := options.Find().
		SetSort(bson.D{{Key: "title_ci", Value: 1}, {Key: "_id", Value: 1}}).
		SetProjection(bson.M{
			"_id":             1,
			"title":           1,
			"subject":         1,
			"type":            1,
			"status":          1,
			"show_in_library": 1,
			"description":     1,
		})

	cur, err := db.Collection("resources").Find(ctx, filter, findOpts)
	if err != nil {
		h.Log.Error("find resources failed", zap.Error(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	var rows []struct {
		ID            primitive.ObjectID `bson:"_id"`
		Title         string             `bson:"title"`
		Subject       string             `bson:"subject"`
		Type          string             `bson:"type"`
		Status        string             `bson:"status"`
		ShowInLibrary bool               `bson:"show_in_library"`
		Description   string             `bson:"description"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		h.Log.Error("decode resources failed", zap.Error(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	items := make([]listItem, 0, len(rows))
	for _, rsrc := range rows {
		items = append(items, listItem{
			ID:            rsrc.ID,
			Title:         rsrc.Title,
			Subject:       rsrc.Subject,
			Type:          rsrc.Type,
			Status:        rsrc.Status,
			ShowInLibrary: rsrc.ShowInLibrary,
			Description:   rsrc.Description,
		})
	}

	data := listData{
		Title:       "Resources",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		Q:           q,
		Items:       items,
		CurrentPath: nav.CurrentPath(r),
	}

	// HTMX partial table refresh
	if r.Header.Get("HX-Request") != "" && r.Header.Get("HX-Target") == "resources-table-wrap" {
		templates.RenderSnippet(w, "resources_table", data)
		return
	}

	templates.Render(w, r, "resources_list", data)
}
