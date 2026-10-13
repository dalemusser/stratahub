package mhsactivity

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/app/store/users"
	activity "github.com/dalemusser/stratahub/internal/app/system/mhsactivity"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// OrgInfo names an organization and gives its zone.
type OrgInfo struct {
	Name string
	TZ   string
	Loc  *time.Location
}

// GroupInfo names a group and its organization.
type GroupInfo struct {
	Name  string
	OrgID primitive.ObjectID
}

// Lookups are the names the report needs for the ids in the day documents.
type Lookups struct {
	Orgs   map[primitive.ObjectID]OrgInfo
	Groups map[primitive.ObjectID]GroupInfo
	Users  map[primitive.ObjectID]string
}

// loadLookups resolves the organizations, groups and users the documents
// name. orgOpts (the scope's organizations) seed the names so an
// organization without activity still has one.
func loadLookups(ctx context.Context, db *mongo.Database, docs []models.MHSActivityDay, orgOpts []viewscope.OrgOption, groupOpts []viewscope.GroupOption) (Lookups, error) {
	lk := Lookups{Orgs: map[primitive.ObjectID]OrgInfo{}, Groups: map[primitive.ObjectID]GroupInfo{}, Users: map[primitive.ObjectID]string{}}
	for _, g := range groupOpts {
		lk.Groups[g.ID] = GroupInfo{Name: g.Name, OrgID: g.OrgID}
	}
	orgIDs := map[primitive.ObjectID]bool{}
	for _, o := range orgOpts {
		orgIDs[o.ID] = true
	}
	userIDs := make([]primitive.ObjectID, 0, len(docs))
	var groupIDs []primitive.ObjectID
	for _, d := range docs {
		orgIDs[d.OrganizationID] = true
		userIDs = append(userIDs, d.UserID)
		for _, g := range d.GroupIDs {
			if _, ok := lk.Groups[g]; !ok {
				groupIDs = append(groupIDs, g)
			}
		}
	}
	ids := make([]primitive.ObjectID, 0, len(orgIDs))
	for id := range orgIDs {
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		orgs, err := organizationstore.New(db).GetByIDs(ctx, ids)
		if err != nil {
			return lk, err
		}
		for _, o := range orgs {
			loc, _ := activity.LocationFor(o.TimeZone)
			lk.Orgs[o.ID] = OrgInfo{Name: o.Name, TZ: loc.String(), Loc: loc}
		}
	}
	if len(groupIDs) > 0 {
		cur, err := db.Collection("groups").Find(ctx, bson.M{"_id": bson.M{"$in": groupIDs}})
		if err != nil {
			return lk, err
		}
		for cur.Next(ctx) {
			var g struct {
				ID    primitive.ObjectID `bson:"_id"`
				Name  string             `bson:"name"`
				OrgID primitive.ObjectID `bson:"organization_id"`
			}
			if err := cur.Decode(&g); err == nil {
				lk.Groups[g.ID] = GroupInfo{Name: g.Name, OrgID: g.OrgID}
			}
		}
		cur.Close(ctx)
	}
	if len(userIDs) > 0 {
		us, err := userstore.New(db).GetByIDs(ctx, userIDs)
		if err != nil {
			return lk, err
		}
		for _, u := range us {
			lk.Users[u.ID] = u.FullName
		}
	}
	return lk, nil
}

// OrgName falls back to the id's tail for an organization that is gone.
func (lk Lookups) OrgName(id primitive.ObjectID) string {
	if o, ok := lk.Orgs[id]; ok && o.Name != "" {
		return o.Name
	}
	return "Organization …" + tail(id.Hex())
}

// OrgLoc is the organization's zone, UTC when unknown.
func (lk Lookups) OrgLoc(id primitive.ObjectID) *time.Location {
	if o, ok := lk.Orgs[id]; ok && o.Loc != nil {
		return o.Loc
	}
	return time.UTC
}

// UserName falls back to the id's tail for a user that is gone.
func (lk Lookups) UserName(id primitive.ObjectID) string {
	if n, ok := lk.Users[id]; ok && n != "" {
		return n
	}
	return "Member …" + tail(id.Hex())
}

func (lk Lookups) GroupName(id primitive.ObjectID) string {
	if g, ok := lk.Groups[id]; ok && g.Name != "" {
		return g.Name
	}
	return "Group …" + tail(id.Hex())
}

func tail(hex string) string {
	if len(hex) > 6 {
		return hex[len(hex)-6:]
	}
	return hex
}
