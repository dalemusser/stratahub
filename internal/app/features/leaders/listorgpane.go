// internal/app/features/leaders/listorgpane.go
package leaders

import (
	"context"

	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"go.mongodb.org/mongo-driver/mongo"
)

// fetchOrgPane fetches the org pane data including paginated orgs with leader counts.
func (h *Handler) fetchOrgPane(
	ctx context.Context,
	db *mongo.Database,
	orgQ, orgAfter, orgBefore string,
) (orgPaneData, error) {
	data, err := orgutil.FetchOrgPane(ctx, db, h.Log, "leader", orgQ, orgAfter, orgBefore)
	if err != nil {
		return orgPaneData{}, err
	}

	return orgPaneData{
		Rows:       data.Rows,
		Total:      data.Total,
		HasPrev:    data.HasPrev,
		HasNext:    data.HasNext,
		PrevCursor: data.PrevCursor,
		NextCursor: data.NextCursor,
		RangeStart: data.RangeStart,
		RangeEnd:   data.RangeEnd,
		AllCount:   data.AllCount,
	}, nil
}
