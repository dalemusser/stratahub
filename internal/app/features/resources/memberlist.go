package resources

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/queries/memberresources"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/http/webutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *MemberHandler) ServeListResources(w http.ResponseWriter, r *http.Request) {
	role, uname, memberOID, ok := authz.UserCtx(r)
	role = strings.ToLower(role)
	logged := ok

	ctx, cancel := context.WithTimeout(r.Context(), resourcesShortTimeout)
	defer cancel()
	db := h.DB

	var member struct {
		ID             primitive.ObjectID  `bson:"_id"`
		Email          string              `bson:"email"`
		OrganizationID *primitive.ObjectID `bson:"organization_id"`
	}
	err := db.Collection("users").FindOne(ctx, map[string]interface{}{
		"_id":  memberOID,
		"role": "member",
	}).Decode(&member)
	if err != nil {
		http.Error(w, "member not found", http.StatusInternalServerError)
		return
	}

	orgName, loc, tzID := resolveMemberOrgLocation(ctx, db, member.OrganizationID)
	tzLabel := ""
	if tzID != "" {
		tzLabel = timezones.Label(tzID)
	}

	results, err := memberresources.ListResourcesForMember(ctx, db, memberOID, memberresources.StatusFilter{Resource: "active", Group: ""})
	if err != nil {
		http.Error(w, "error loading resources", http.StatusInternalServerError)
		return
	}

	if len(results) == 0 {
		templates.Render(w, r, "member_resources_list", resourceListData{
			common: common{
				Title:      "My Resources",
				IsLoggedIn: logged,
				Role:       role,
				UserName:   uname,
				UserID:     member.Email,
			},
			TimeZone: tzLabel,
		})
		return
	}

	nowLocal := time.Now().In(loc)
	var items []resourceListItem
	for _, row := range results {
		if row.VisibleFrom == nil || row.VisibleFrom.IsZero() {
			continue
		}
		start := row.VisibleFrom.In(loc)
		if nowLocal.Before(start) {
			continue
		}

		availableUntil := "No end date"
		if row.VisibleUntil != nil && !row.VisibleUntil.IsZero() {
			end := row.VisibleUntil.In(loc)
			if !nowLocal.Before(end) {
				continue
			}
			availableUntil = end.Format("2006-01-02 15:04")
		}

		launchURL := webutil.AddOrSetQueryParams(row.Resource.LaunchURL, map[string]string{
			"id":    member.Email,
			"group": row.GroupName,
			"org":   orgName,
		})

		items = append(items, resourceListItem{
			ID:             row.Resource.ID.Hex(),
			Title:          row.Resource.Title,
			Subject:        row.Resource.Subject,
			Type:           row.Resource.Type,
			LaunchURL:      launchURL,
			AvailableUntil: availableUntil,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})

	templates.Render(w, r, "member_resources_list", resourceListData{
		common: common{
			Title:      "My Resources",
			IsLoggedIn: logged,
			Role:       role,
			UserName:   uname,
			UserID:     member.Email,
		},
		Resources: items,
		TimeZone:  tzLabel,
	})
}
