// internal/app/features/auditlog/list.go
package auditlog

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"github.com/dalemusser/stratahub/internal/app/store/coordinatorassign"
	orgstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

const pageSize = 50

// ServeList handles GET /audit - displays the audit log list with filtering.
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	role, _, userID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "audit log list")
	defer cancel()

	// Get filter parameters
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	eventType := strings.TrimSpace(r.URL.Query().Get("event_type"))
	startDate := strings.TrimSpace(r.URL.Query().Get("start_date"))
	endDate := strings.TrimSpace(r.URL.Query().Get("end_date"))
	pageStr := r.URL.Query().Get("page")

	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	// Build query filter
	filter := audit.QueryFilter{
		Category:  category,
		EventType: eventType,
		Limit:     pageSize,
		Offset:    int64((page - 1) * pageSize),
	}

	// Parse dates
	if startDate != "" {
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			filter.StartTime = &t
		}
	}
	if endDate != "" {
		if t, err := time.Parse("2006-01-02", endDate); err == nil {
			// End of day
			endOfDay := t.Add(24*time.Hour - time.Second)
			filter.EndTime = &endOfDay
		}
	}

	// Authorization: coordinators can only see events from their assigned orgs
	if role == "coordinator" {
		coordStore := coordinatorassign.New(h.DB)
		orgIDs, err := coordStore.OrgIDsByUser(ctx, userID)
		if err != nil {
			h.Log.Error("failed to get coordinator org IDs", zap.Error(err))
			h.ErrLog.LogServerError(w, r, "database error", err, "A database error occurred.", "/dashboard")
			return
		}
		if len(orgIDs) == 0 {
			// No orgs assigned - show empty result
			tzGroups, _ := timezones.Groups()
			templates.RenderAutoMap(w, r, "audit_list", nil, listData{
				BaseVM:         viewdata.NewBaseVM(r, h.DB, "Audit Log", "/dashboard"),
				Categories:     allCategories(),
				TimezoneGroups: tzGroups,
				Page:           1,
				TotalPages:     1,
			})
			return
		}
		filter.OrganizationIDs = orgIDs
	}

	// Query audit store
	auditStore := audit.New(h.DB)
	events, err := auditStore.Query(ctx, filter)
	if err != nil {
		h.Log.Error("failed to query audit events", zap.Error(err))
		h.ErrLog.LogServerError(w, r, "database error", err, "A database error occurred.", "/dashboard")
		return
	}

	total, err := auditStore.CountByFilter(ctx, filter)
	if err != nil {
		h.Log.Error("failed to count audit events", zap.Error(err))
		h.ErrLog.LogServerError(w, r, "database error", err, "A database error occurred.", "/dashboard")
		return
	}

	// Collect unique user IDs and org IDs for name resolution
	userIDs := make(map[primitive.ObjectID]struct{})
	orgIDs := make(map[primitive.ObjectID]struct{})
	for _, e := range events {
		if e.ActorID != nil {
			userIDs[*e.ActorID] = struct{}{}
		}
		if e.UserID != nil {
			userIDs[*e.UserID] = struct{}{}
		}
		if e.OrganizationID != nil {
			orgIDs[*e.OrganizationID] = struct{}{}
		}
	}

	// Batch fetch user names
	userNames := make(map[primitive.ObjectID]string)
	if len(userIDs) > 0 {
		usrStore := userstore.New(h.DB)
		ids := make([]primitive.ObjectID, 0, len(userIDs))
		for id := range userIDs {
			ids = append(ids, id)
		}
		users, err := usrStore.GetByIDs(ctx, ids)
		if err != nil {
			h.Log.Warn("failed to fetch user names for audit log", zap.Error(err))
		} else {
			for _, u := range users {
				userNames[u.ID] = u.FullName
			}
		}
	}

	// Batch fetch org names
	orgNames := make(map[primitive.ObjectID]string)
	if len(orgIDs) > 0 {
		oStore := orgstore.New(h.DB)
		ids := make([]primitive.ObjectID, 0, len(orgIDs))
		for id := range orgIDs {
			ids = append(ids, id)
		}
		orgs, err := oStore.GetByIDs(ctx, ids)
		if err != nil {
			h.Log.Warn("failed to fetch org names for audit log", zap.Error(err))
		} else {
			for _, o := range orgs {
				orgNames[o.ID] = o.Name
			}
		}
	}

	// Build list items
	items := make([]listItem, 0, len(events))
	for _, e := range events {
		item := listItem{
			ID:        e.ID.Hex(),
			Timestamp: e.Timestamp,
			Category:  e.Category,
			EventType: e.EventType,
			IP:        e.IP,
			Success:   e.Success,
			Details:   e.Details,
		}
		if e.ActorID != nil {
			if name, ok := userNames[*e.ActorID]; ok {
				item.ActorName = name
			} else {
				item.ActorName = e.ActorID.Hex()
			}
		}
		if e.UserID != nil {
			if name, ok := userNames[*e.UserID]; ok {
				item.TargetName = name
			} else {
				item.TargetName = e.UserID.Hex()
			}
		}
		if e.OrganizationID != nil {
			if name, ok := orgNames[*e.OrganizationID]; ok {
				item.OrgName = name
			} else {
				item.OrgName = e.OrganizationID.Hex()
			}
		}
		items = append(items, item)
	}

	// Calculate pagination
	totalPages := int((total + pageSize - 1) / pageSize)
	if totalPages < 1 {
		totalPages = 1
	}

	prevPage := page - 1
	if prevPage < 1 {
		prevPage = 1
	}
	nextPage := page + 1
	if nextPage > totalPages {
		nextPage = totalPages
	}

	// Get event types for selected category (or all if no category selected)
	eventTypes := eventTypesForCategory(category)

	// Get timezone groups for selector
	tzGroups, _ := timezones.Groups()

	templates.RenderAutoMap(w, r, "audit_list", nil, listData{
		BaseVM:         viewdata.NewBaseVM(r, h.DB, "Audit Log", "/dashboard"),
		Items:          items,
		Category:       category,
		EventType:      eventType,
		StartDate:      startDate,
		EndDate:        endDate,
		Categories:     allCategories(),
		EventTypes:     eventTypes,
		TimezoneGroups: tzGroups,
		Page:           page,
		TotalPages:     totalPages,
		Total:          total,
		Shown:          len(items),
		HasPrev:        page > 1,
		HasNext:        page < totalPages,
		PrevPage:       prevPage,
		NextPage:       nextPage,
	})
}
