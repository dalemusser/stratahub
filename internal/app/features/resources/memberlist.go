package resources

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/resourcepolicy"
	"github.com/dalemusser/stratahub/internal/app/store/queries/memberresources"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/urlutil"
)

func (h *MemberHandler) ServeListResources(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	// Verify member access using policy layer
	member, err := resourcepolicy.VerifyMemberAccess(ctx, db, r)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error verifying member access", err, "A database error occurred.", "/")
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

	results, err := memberresources.ListResourcesForMember(ctx, db, member.ID, memberresources.StatusFilter{Resource: "active", Group: ""})
	if err != nil {
		uierrors.RenderServerError(w, r, "A database error occurred.", "/")
		return
	}

	if len(results) == 0 {
		templates.Render(w, r, "member_resources_list", resourceListData{
			common: common{
				BaseVM: viewdata.NewBaseVM(r, h.DB, "My Resources", "/"),
				UserID: member.Email,
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

		launchURL := urlutil.AddOrSetQueryParams(row.Resource.LaunchURL, map[string]string{
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
			HasFile:        row.Resource.FilePath != "",
			AvailableUntil: availableUntil,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})

	templates.Render(w, r, "member_resources_list", resourceListData{
		common: common{
			BaseVM: viewdata.NewBaseVM(r, h.DB, "My Resources", "/"),
			UserID: member.Email,
		},
		Resources: items,
		TimeZone:  tzLabel,
	})
}
