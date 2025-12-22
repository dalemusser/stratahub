package resources

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/resourcepolicy"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/urlutil"

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
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	resourceID := chi.URLParam(r, "resourceID")
	oid, err := primitive.ObjectIDFromHex(resourceID)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/member/resources")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Verify member access using policy layer
	member, err := resourcepolicy.VerifyMemberAccess(ctx, db, r)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error verifying member access", err, "A database error occurred.", "/member/resources")
		return
	}
	if member == nil {
		uierrors.RenderNotFound(w, r, "Member not found.", "/login")
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
		uierrors.RenderNotFound(w, r, "Resource not found.", "/member/resources")
		return
	default:
		h.ErrLog.LogServerError(w, r, "find resource failed", err, "A database error occurred.", "/member/resources")
		return
	}

	// Check if member has access to this resource via group assignment
	assignment, err := resourcepolicy.CanViewResource(ctx, db, member.ID, oid)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "policy check failed", err, "A database error occurred.", "/member/resources")
		return
	}

	groupName := ""
	var visibleFrom, visibleUntil *time.Time
	if assignment != nil {
		groupName = assignment.GroupName
		visibleFrom = assignment.VisibleFrom
		visibleUntil = assignment.VisibleUntil
	}

	// Build launch URL with id (email), group (name), org (name)
	launch := urlutil.AddOrSetQueryParams(res.LaunchURL, map[string]string{
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
		BackURL:             httpnav.ResolveBackURL(r, "/member/resources"),
		CanOpen:             canOpen,
		TimeZone:            tzLabel,
	}

	templates.Render(w, r, "member_resource_view", data)
}
