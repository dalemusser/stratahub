// internal/app/features/groups/resourceassignquery.go
package groups

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	resourcestore "github.com/dalemusser/stratahub/internal/app/store/resources"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// buildAssignments composes the assigned + available resources lists, including
// availability summaries, paging state, and filters.
func (h *Handler) buildAssignments(ctx context.Context, r *http.Request, g models.Group, q, typeFilter, after, before string) (
	assigned []assignedResourceItem,
	avail []availableResourceItem,
	shown int,
	total int64,
	nextCursor, prevCursor string,
	hasNext, hasPrev bool,
	err error,
) {
	db := h.DB

	// assigned resource ids
	assns, e := resourceassignstore.New(db).ListByGroup(ctx, g.ID)
	if e != nil {
		err = e
		return
	}

	assignedIDs := make([]primitive.ObjectID, 0, len(assns))
	for _, a := range assns {
		assignedIDs = append(assignedIDs, a.ResourceID)
	}

	// Use organization-specific timezone for assignment availability
	loc, _ := resolveGroupLocation(ctx, db, g)
	now := time.Now().In(loc)

	// load assigned resources
	if len(assignedIDs) > 0 {
		resStore := resourcestore.New(db)
		resList, findErr := resStore.Find(ctx, bson.M{"_id": bson.M{"$in": assignedIDs}})
		if findErr != nil {
			h.Log.Error("database error finding assigned resources", zap.Error(findErr), zap.String("group_id", g.ID.Hex()))
			err = findErr
			return
		}

		resByID := make(map[primitive.ObjectID]models.Resource)
		for _, rdoc := range resList {
			resByID[rdoc.ID] = rdoc
		}

		for _, a := range assns {
			res, ok := resByID[a.ResourceID]
			if !ok {
				continue
			}

			availStr, availTitle := summarizeAssignmentAvailability(now, a.VisibleFrom, a.VisibleUntil)

			assigned = append(assigned, assignedResourceItem{
				AssignmentID:      a.ID.Hex(),
				ResourceID:        res.ID.Hex(),
				Title:             res.Title,
				Subject:           res.Subject,
				Type:              res.Type,
				Status:            res.Status,
				Availability:      availStr,
				AvailabilityTitle: availTitle,
			})
		}
	}

	// sort assigned by Title (case-insensitive)
	sort.SliceStable(assigned, func(i, j int) bool {
		ti := strings.ToLower(assigned[i].Title)
		tj := strings.ToLower(assigned[j].Title)
		if ti == tj {
			return assigned[i].Title < assigned[j].Title
		}
		return ti < tj
	})

	// load available resources with paging/search (status:active only; do not exclude assigned)
	avail, shown, total, nextCursor, prevCursor, hasNext, hasPrev, err = h.fetchAvailableResourcesPaged(ctx, r, q, typeFilter, after, before)
	return
}

// fetchAvailableResourcesPaged returns a page of available resources with optional
// search and type filters, using keyset pagination on (title_ci, _id).
func (h *Handler) fetchAvailableResourcesPaged(
	ctx context.Context, r *http.Request,
	qRaw, typeFilter, after, before string,
) (resources []availableResourceItem, shown int, total int64, nextCursor, prevCursor string, hasNext, hasPrev bool, err error) {
	db := h.DB
	resStore := resourcestore.New(db)

	filter := bson.M{"status": "active"}
	workspace.Filter(r, filter)
	if strings.TrimSpace(typeFilter) != "" {
		filter["type"] = strings.TrimSpace(typeFilter)
	}

	q := text.Fold(qRaw)
	if q != "" {
		high := q + "\uffff"
		filter["title_ci"] = bson.M{"$gte": q, "$lt": high}
	}

	if cnt, cntErr := resStore.Count(ctx, filter); cntErr != nil {
		h.Log.Error("database error counting available resources", zap.Error(cntErr))
		err = cntErr
		return
	} else {
		total = cnt
	}

	findOpts := options.Find()
	limit := paging.LimitPlusOne()
	if before != "" {
		if c, ok := wafflemongo.DecodeCursor(before); ok {
			filter["$or"] = []bson.M{
				{"title_ci": bson.M{"$lt": c.CI}},
				{"title_ci": c.CI, "_id": bson.M{"$lt": c.ID}},
			}
		}
		findOpts.SetSort(bson.D{{Key: "title_ci", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	} else {
		if after != "" {
			if c, ok := wafflemongo.DecodeCursor(after); ok {
				filter["$or"] = []bson.M{
					{"title_ci": bson.M{"$gt": c.CI}},
					{"title_ci": c.CI, "_id": bson.M{"$gt": c.ID}},
				}
			}
		}
		findOpts.SetSort(bson.D{{Key: "title_ci", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
	}

	rows, e := resStore.Find(ctx, filter, findOpts)
	if e != nil {
		err = e
		return
	}

	if before != "" {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}

	orig := len(rows)
	if before != "" {
		if orig > paging.PageSize {
			rows = rows[1:]
			hasPrev = true
		} else {
			hasPrev = false
		}
		hasNext = true
	} else {
		if orig > paging.PageSize {
			rows = rows[:paging.PageSize]
			hasNext = true
		} else {
			hasNext = false
		}
		hasPrev = after != ""
	}

	resources = make([]availableResourceItem, 0, len(rows))
	for _, r := range rows {
		resources = append(resources, availableResourceItem{
			ResourceID: r.ID.Hex(),
			Title:      r.Title,
			Subject:    r.Subject,
			Type:       r.Type,
			Status:     r.Status,
		})
	}
	shown = len(resources)
	if shown > 0 {
		first := rows[0]
		last := rows[shown-1]
		prevCursor = wafflemongo.EncodeCursor(first.TitleCI, first.ID)
		nextCursor = wafflemongo.EncodeCursor(last.TitleCI, last.ID)
	}

	return
}

// summarizeAssignmentAvailability builds a compact availability summary and
// a tooltip from optional VisibleFrom/VisibleUntil timestamps.
func summarizeAssignmentAvailability(now time.Time, from, until *time.Time) (string, string) {
	// Build the tooltip with full datetimes
	layout := "2006-01-02 15:04"
	startLabel := "No start date"
	endLabel := "No end date"

	var fromVal, untilVal time.Time
	if from != nil {
		fromVal = *from
		if !fromVal.IsZero() {
			startLabel = "Starts " + fromVal.Format(layout)
		}
	}
	if until != nil {
		untilVal = *until
		if !untilVal.IsZero() {
			endLabel = "Ends " + untilVal.Format(layout)
		}
	}

	tooltip := fmt.Sprintf("%s — %s", startLabel, endLabel)

	// Build the compact relative summary
	var startPart string
	if from == nil || fromVal.IsZero() {
		startPart = "Unscheduled"
	} else if !now.Before(fromVal) {
		startPart = "Now"
	} else {
		startPart = "In " + humanizeDuration(fromVal.Sub(now))
	}

	var endPart string
	if until == nil || untilVal.IsZero() {
		endPart = "No end"
	} else if now.After(untilVal) {
		endPart = "Ended"
	} else {
		endPart = "Ends in " + humanizeDuration(untilVal.Sub(now))
	}

	if startPart == "" && endPart == "" {
		return "", tooltip
	}
	if endPart == "" {
		return startPart, tooltip
	}
	return startPart + " → " + endPart, tooltip
}

// humanizeDuration produces short, coarse-grained phrases like "2 days" or
// "5 hours" from a positive duration.
func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	// Round to the nearest hour
	hours := int(d.Hours() + 0.5)
	if hours <= 1 {
		return "1 hour"
	}
	if hours < 24 {
		return fmt.Sprintf("%d hours", hours)
	}

	days := hours / 24
	if days <= 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
