package resources

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// resolveMemberOrgLocation resolves the organization name, time.Location, and
// timezone ID for a member's organization.
//
// Return values:
//   - orgName: empty if there is no organization or lookup fails
//   - loc: time.Local by default, or the organization's timezone if loadable
//   - tzID: the raw timezone ID string from the organization (may be empty)
func resolveMemberOrgLocation(ctx context.Context, db *mongo.Database, orgID *primitive.ObjectID) (orgName string, loc *time.Location, tzID string) {
	loc = time.Local
	tzID = ""

	if orgID == nil || orgID.IsZero() {
		return orgName, loc, tzID
	}

	var org struct {
		Name     string `bson:"name"`
		TimeZone string `bson:"time_zone"`
	}

	err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": orgID}).Decode(&org)
	if err != nil {
		if err != mongo.ErrNoDocuments {
			zap.L().Warn("org FindOne(resolveMemberOrgLocation)", zap.Error(err), zap.Any("org_id", orgID))
		}
		return orgName, loc, tzID
	}

	orgName = org.Name
	tzID = strings.TrimSpace(org.TimeZone)
	if tzID == "" {
		return orgName, loc, tzID
	}

	if l, err := time.LoadLocation(tzID); err == nil {
		loc = l
	} else {
		zap.L().Warn("time.LoadLocation(resolveMemberOrgLocation)", zap.Error(err), zap.String("tz", tzID))
	}

	return orgName, loc, tzID
}
