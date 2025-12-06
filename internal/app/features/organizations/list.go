// internal/app/system/handlers/organizations/list.go
package organizations

import (
	"context"
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.uber.org/zap"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServeList handles GET /organizations (with optional ?q= search).
// It supports HTMX partial refresh of the table when HX-Target="orgs-table-wrap".
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		// In practice, this should be guarded by auth middleware, but we fail safe.
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))

	ctx, cancel := context.WithTimeout(r.Context(), orgsMedTimeout)
	defer cancel()

	db := h.DB

	filter := bson.M{}
	if q != "" {
		fq := textfold.Fold(q)
		if fq != "" {
			hi := fq + "\uffff"
			filter["$or"] = []bson.M{
				{"name_ci": bson.M{"$gte": fq, "$lt": hi}},
				{"city_ci": bson.M{"$gte": fq, "$lt": hi}},
				{"state_ci": bson.M{"$gte": fq, "$lt": hi}},
			}
		}
	}

	find := options.Find().
		SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}})

	cur, err := db.Collection("organizations").Find(ctx, filter, find)
	if err != nil {
		h.Log.Error("find organizations failed", zap.Error(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	var orgs []models.Organization
	if err := cur.All(ctx, &orgs); err != nil {
		h.Log.Error("decode organizations failed", zap.Error(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Build ID list for aggregates
	orgIDs := make([]primitive.ObjectID, 0, len(orgs))
	for _, o := range orgs {
		orgIDs = append(orgIDs, o.ID)
	}

	leadersMap, _ := aggregateCountByOrg(ctx, db, "users", bson.M{
		"role":            "leader",
		"organization_id": bson.M{"$in": orgIDs},
	}, "organization_id")

	groupsMap, _ := aggregateCountByOrg(ctx, db, "groups", bson.M{
		"organization_id": bson.M{"$in": orgIDs},
	}, "organization_id")

	items := make([]listItem, 0, len(orgs))
	for _, o := range orgs {
		items = append(items, listItem{
			ID:           o.ID,
			Name:         o.Name,
			City:         o.City,
			State:        o.State,
			LeadersCount: leadersMap[o.ID.Hex()],
			GroupsCount:  groupsMap[o.ID.Hex()],
		})
	}

	data := listData{
		Title:       "Organizations",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		Q:           q,
		Items:       items,
		CurrentPath: nav.CurrentPath(r),
	}

	// HTMX partial: just the table
	if r.Header.Get("HX-Request") != "" && r.Header.Get("HX-Target") == "orgs-table-wrap" {
		templates.RenderSnippet(w, "organizations_table", data)
		return
	}

	templates.Render(w, r, "organizations_list", data)
}
