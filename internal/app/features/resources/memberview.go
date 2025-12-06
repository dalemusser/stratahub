package resources

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	webutil "github.com/dalemusser/waffle/toolkit/http/webutil"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.uber.org/zap"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ServeViewResource handles GET /member/resources/{resourceID} for members.
// It enforces that the current user is a member, checks that the resource is
// currently available based on the group assignment visibility window, and
// then renders the member_resource_view template.
func (h *MemberHandler) ServeViewResource(w http.ResponseWriter, r *http.Request) {
	role, uname, memberOID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	role = strings.ToLower(role)

	resourceID := chi.URLParam(r, "resourceID")
	oid, err := primitive.ObjectIDFromHex(resourceID)
	if err != nil {
		http.Error(w, "invalid resource id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), resourcesMedTimeout)
	defer cancel()
	db := h.DB

	// Fetch the member: need login email, org, and groups
	var member struct {
		ID             primitive.ObjectID  `bson:"_id"`
		Email          string              `bson:"email"`
		OrganizationID *primitive.ObjectID `bson:"organization_id,omitempty"`
	}
	if err := db.Collection("users").FindOne(ctx,
		bson.M{"_id": memberOID, "role": "member"}).Decode(&member); err != nil {
		http.Error(w, "member not found", http.StatusNotFound)
		return
	}

	orgName, loc, tzID := resolveMemberOrgLocation(ctx, db, member.OrganizationID)
	tzLabel := ""
	if tzID != "" {
		tzLabel = timezones.Label(tzID)
	}

	// "Now" in org-local time
	nowLocal := time.Now().In(loc)

	// Fetch the resource
	var res models.Resource
	switch err := db.Collection("resources").FindOne(ctx, bson.M{"_id": oid}).Decode(&res); err {
	case nil:
		// ok
	case mongo.ErrNoDocuments:
		http.NotFound(w, r)
		return
	default:
		h.Log.Error("find resource failed", zapError(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Find ONE of the member's groups that assigns this resource (if any),
	// along with the assignment visibility window.
	var asnRow struct {
		GroupName    string     `bson:"group_name"`
		VisibleFrom  *time.Time `bson:"visible_from"`
		VisibleUntil *time.Time `bson:"visible_until"`
	}

	assignPipe := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"user_id": memberOID,
			"role":    "member",
		}}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "group_resource_assignments",
			"localField":   "group_id",
			"foreignField": "group_id",
			"as":           "asg",
		}}},
		bson.D{{Key: "$unwind", Value: "$asg"}},
		bson.D{{Key: "$match", Value: bson.M{"asg.resource_id": oid}}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "groups",
			"localField":   "group_id",
			"foreignField": "_id",
			"as":           "g",
		}}},
		bson.D{{Key: "$unwind", Value: "$g"}},
		bson.D{{Key: "$project", Value: bson.M{
			"group_name":    "$g.name",
			"visible_from":  "$asg.visible_from",
			"visible_until": "$asg.visible_until",
		}}},
		bson.D{{Key: "$limit", Value: 1}},
	}

	assignCur, err := db.Collection("group_memberships").Aggregate(ctx, assignPipe)
	if err != nil {
		h.Log.Error("aggregate assignments failed", zapError(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer assignCur.Close(ctx)

	groupName := ""
	var visibleFrom, visibleUntil *time.Time
	if assignCur.Next(ctx) {
		if err := assignCur.Decode(&asnRow); err != nil {
			h.Log.Error("decode assignment row failed", zapError(err))
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		groupName = asnRow.GroupName
		visibleFrom = asnRow.VisibleFrom
		visibleUntil = asnRow.VisibleUntil
	}

	// Build launch URL with id (email), group (name), org (name)
	launch := webutil.AddOrSetQueryParams(res.LaunchURL, map[string]string{
		"id":    member.Email,
		"group": groupName,
		"org":   orgName,
	})

	canOpen := false
	availableUntil := ""

	if visibleFrom != nil && !visibleFrom.IsZero() {
		fromLocal := visibleFrom.In(loc)
		if !nowLocal.Before(fromLocal) {
			// We are at or after the start time.
			if visibleUntil != nil && !visibleUntil.IsZero() {
				untilLocal := visibleUntil.In(loc)
				availableUntil = untilLocal.Format("2006-01-02 15:04")
				if nowLocal.Before(untilLocal) {
					// In window.
					canOpen = (res.Status == "active")
				}
			} else {
				// No end date; started and still active.
				availableUntil = "No end date"
				canOpen = (res.Status == "active")
			}
		} else {
			// Not yet started.
			availableUntil = "No end date"
		}
	} else {
		// No visible_from: treat as not currently available.
		availableUntil = "No end date"
	}

	typeLabel := res.Type
	if typeLabel == "" {
		typeLabel = "Resource"
	} else {
		// capitalize first letter for display (e.g., "game" -> "Game")
		if len(typeLabel) > 1 {
			typeLabel = strings.ToUpper(typeLabel[:1]) + typeLabel[1:]
		} else {
			typeLabel = strings.ToUpper(typeLabel)
		}
	}

	data := viewResourceData{
		common: common{
			Title:      "View Resource",
			IsLoggedIn: true,
			Role:       role,
			UserName:   uname,
			UserID:     member.Email,
		},
		ResourceID:          res.ID.Hex(),
		ResourceTitle:       res.Title,
		Subject:             res.Subject,
		Type:                res.Type,
		TypeDisplay:         typeLabel,
		Description:         res.Description,
		DefaultInstructions: res.DefaultInstructions,
		LaunchURL:           launch,
		Status:              res.Status,
		AvailableUntil:      availableUntil,
		BackURL:             nav.ResolveBackURL(r, "/member/resources"),
		CanOpen:             canOpen,
		TimeZone:            tzLabel,
	}

	templates.Render(w, r, "member_resource_view", data)
}

// zapError is a tiny helper to keep logging calls concise.
func zapError(err error) zap.Field {
	return zap.Error(err)
}
