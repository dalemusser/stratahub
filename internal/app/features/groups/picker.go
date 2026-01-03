// internal/app/features/groups/picker.go
package groups

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// groupPickerData is the view model for the group picker modal.
type groupPickerData struct {
	Query      string
	SelectedID string // Currently selected group ID (for highlighting)
	OrgID      string // Filter by organization (optional)
	OrgName    string // Organization name for display

	Groups []groupPickerItem

	Total      int64
	RangeStart int
	RangeEnd   int
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	PrevStart  int
	NextStart  int
}

// groupPickerItem represents a group in the picker list.
type groupPickerItem struct {
	ID       string
	Name     string
	OrgName  string // Organization name (shown when not filtering by org)
	Selected bool
}

// ServeGroupPicker serves the group picker modal or just the list portion (for HTMX updates).
func (h *Handler) ServeGroupPicker(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if role != "admin" && role != "coordinator" && role != "leader" {
		uierrors.RenderForbidden(w, r, "Access denied.", "/dashboard")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	// Parse parameters
	q := normalize.QueryParam(query.Get(r, "q"))
	after := normalize.QueryParam(query.Get(r, "after"))
	before := normalize.QueryParam(query.Get(r, "before"))
	selectedID := normalize.QueryParam(query.Get(r, "selected"))
	orgIDHex := normalize.QueryParam(query.Get(r, "org"))
	start := paging.ParseStart(r)

	// Build filter for active groups
	filter := bson.M{
		"status": "active",
	}

	// Filter by organization if provided
	var orgOID primitive.ObjectID
	var orgName string
	if orgIDHex != "" {
		var err error
		orgOID, err = primitive.ObjectIDFromHex(orgIDHex)
		if err == nil {
			filter["organization_id"] = orgOID
			// Look up org name for display
			var orgDoc struct {
				Name string `bson:"name"`
			}
			if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": orgOID}).Decode(&orgDoc); err == nil {
				orgName = orgDoc.Name
			}
		}
	}

	// Leaders can only see groups in their organization
	if role == "leader" {
		var leaderDoc struct {
			OrgID *primitive.ObjectID `bson:"organization_id"`
		}
		if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&leaderDoc); err != nil || leaderDoc.OrgID == nil {
			// No org found for leader - show nothing
			filter["_id"] = primitive.NilObjectID
		} else {
			filter["organization_id"] = *leaderDoc.OrgID
			orgOID = *leaderDoc.OrgID
			// Look up org name
			var orgDoc struct {
				Name string `bson:"name"`
			}
			if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": orgOID}).Decode(&orgDoc); err == nil {
				orgName = orgDoc.Name
			}
		}
	}

	// Coordinators can only see groups in their assigned organizations
	if role == "coordinator" {
		orgIDs := authz.UserOrgIDs(r)
		if len(orgIDs) == 0 {
			filter["_id"] = primitive.NilObjectID // Will match nothing
		} else {
			// If org filter is provided, verify coordinator has access
			if orgIDHex != "" {
				hasAccess := false
				for _, oid := range orgIDs {
					if oid == orgOID {
						hasAccess = true
						break
					}
				}
				if !hasAccess {
					filter["_id"] = primitive.NilObjectID // Will match nothing
				}
			} else {
				filter["organization_id"] = bson.M{"$in": orgIDs}
			}
		}
	}

	// Add search filter if query provided (match from start of name)
	if q != "" {
		qFold := text.Fold(q)
		filter["name_ci"] = bson.M{"$regex": "^" + qFold, "$options": "i"}
	}

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)
	if window := cfg.KeysetWindow("name_ci"); window != nil {
		for k, v := range window {
			filter[k] = v
		}
	}

	// Count total (without pagination) - rebuild filter without keyset window
	countFilter := bson.M{
		"status": "active",
	}
	if orgIDHex != "" && orgOID != primitive.NilObjectID {
		countFilter["organization_id"] = orgOID
	}
	if role == "leader" {
		var leaderDoc struct {
			OrgID *primitive.ObjectID `bson:"organization_id"`
		}
		if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&leaderDoc); err == nil && leaderDoc.OrgID != nil {
			countFilter["organization_id"] = *leaderDoc.OrgID
		} else {
			countFilter["_id"] = primitive.NilObjectID
		}
	}
	if role == "coordinator" {
		orgIDs := authz.UserOrgIDs(r)
		if len(orgIDs) == 0 {
			countFilter["_id"] = primitive.NilObjectID
		} else if orgIDHex != "" {
			hasAccess := false
			for _, oid := range orgIDs {
				if oid == orgOID {
					hasAccess = true
					break
				}
			}
			if !hasAccess {
				countFilter["_id"] = primitive.NilObjectID
			}
		} else {
			countFilter["organization_id"] = bson.M{"$in": orgIDs}
		}
	}
	if q != "" {
		qFold := text.Fold(q)
		countFilter["name_ci"] = bson.M{"$regex": "^" + qFold, "$options": "i"}
	}
	total, err := db.Collection("groups").CountDocuments(ctx, countFilter)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error counting groups", err, "A database error occurred.", "/dashboard")
		return
	}

	// Fetch groups (use modal page size for picker)
	findOpts := options.Find().
		SetProjection(bson.M{"_id": 1, "name": 1, "name_ci": 1, "organization_id": 1})
	cfg.ApplyToFindModal(findOpts, "name_ci")

	cur, err := db.Collection("groups").Find(ctx, filter, findOpts)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error finding groups", err, "A database error occurred.", "/dashboard")
		return
	}
	defer cur.Close(ctx)

	type groupDoc struct {
		ID     primitive.ObjectID  `bson:"_id"`
		Name   string              `bson:"name"`
		NameCI string              `bson:"name_ci"`
		OrgID  *primitive.ObjectID `bson:"organization_id"`
	}
	var docs []groupDoc
	if err := cur.All(ctx, &docs); err != nil {
		h.ErrLog.LogServerError(w, r, "database error decoding groups", err, "A database error occurred.", "/dashboard")
		return
	}

	// Collect unique org IDs to fetch names (if not filtering by single org)
	orgNames := make(map[primitive.ObjectID]string)
	if orgIDHex == "" && len(docs) > 0 {
		var orgIDs []primitive.ObjectID
		for _, d := range docs {
			if d.OrgID != nil {
				orgIDs = append(orgIDs, *d.OrgID)
			}
		}
		if len(orgIDs) > 0 {
			orgCur, err := db.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIDs}},
				options.Find().SetProjection(bson.M{"_id": 1, "name": 1}))
			if err == nil {
				defer orgCur.Close(ctx)
				for orgCur.Next(ctx) {
					var org struct {
						ID   primitive.ObjectID `bson:"_id"`
						Name string             `bson:"name"`
					}
					if orgCur.Decode(&org) == nil {
						orgNames[org.ID] = org.Name
					}
				}
			}
		}
	}

	// Apply pagination trimming (use modal page size)
	result := paging.TrimPageModal(&docs, before, after)
	if cfg.Direction == paging.Backward {
		paging.Reverse(docs)
	}

	// Build cursors
	var prevCursor, nextCursor string
	if len(docs) > 0 {
		prevCursor, nextCursor = paging.BuildCursors(docs,
			func(d groupDoc) string { return d.NameCI },
			func(d groupDoc) primitive.ObjectID { return d.ID },
		)
	}

	// Compute range (use modal page size)
	rng := paging.ComputeRangeModal(start, len(docs))

	// Convert to view model
	groups := make([]groupPickerItem, len(docs))
	for i, d := range docs {
		idHex := d.ID.Hex()
		item := groupPickerItem{
			ID:       idHex,
			Name:     d.Name,
			Selected: idHex == selectedID,
		}
		// Add org name if not filtering by org
		if orgIDHex == "" && d.OrgID != nil {
			item.OrgName = orgNames[*d.OrgID]
		}
		groups[i] = item
	}

	data := groupPickerData{
		Query:      q,
		SelectedID: selectedID,
		OrgID:      orgIDHex,
		OrgName:    orgName,
		Groups:     groups,
		Total:      total,
		RangeStart: rng.Start,
		RangeEnd:   rng.End,
		HasPrev:    result.HasPrev,
		HasNext:    result.HasNext,
		PrevCursor: prevCursor,
		NextCursor: nextCursor,
		PrevStart:  rng.PrevStart,
		NextStart:  rng.NextStart,
	}

	// Check if this is an HTMX request for just the list
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "group-picker-list" {
		templates.Render(w, r, "group_picker_list", data)
		return
	}

	// Full modal
	templates.Render(w, r, "group_picker_modal", data)
}
