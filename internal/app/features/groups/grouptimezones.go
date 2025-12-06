// internal/app/features/groups/grouptimezones.go
package groups

import (
	"context"
	"strings"
	"time"

	orgstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// resolveGroupLocation resolves the time.Location and timezone ID to use for a
// given group.
//
// It looks up the group's organization (if any), reads org.TimeZone, and:
//
//   - returns loc = time.Local and tzID = "" if there is no org or timezone
//   - otherwise, tries time.LoadLocation(tzID) and returns that loc + tzID
//
// Callers are expected to:
//
//   - use loc for all time calculations (time.Now().In(loc), formatting, etc.)
//   - use tzID to look up a human-friendly label via the timezones module
//     when presenting the timezone to the user.
func resolveGroupLocation(ctx context.Context, db *mongo.Database, g models.Group) (*time.Location, string) {
	loc := time.Local
	tzID := ""

	// If the group is not attached to an organization, fall back to server local.
	if g.OrganizationID.IsZero() {
		return loc, tzID
	}

	org, err := orgstore.New(db).GetByID(ctx, g.OrganizationID)
	if err != nil {
		if err != mongo.ErrNoDocuments {
			zap.L().Warn("org GetByID(resolveGroupLocation)", zap.Error(err))
		}
		return loc, tzID
	}

	tzID = strings.TrimSpace(org.TimeZone)
	if tzID == "" {
		return loc, tzID
	}

	if l, err := time.LoadLocation(tzID); err == nil {
		loc = l
	} else {
		// If LoadLocation fails, keep loc = time.Local but still return tzID
		// so callers can at least display the raw ID.
		zap.L().Warn("time.LoadLocation(resolveGroupLocation)", zap.Error(err), zap.String("tz", tzID))
	}

	return loc, tzID
}
