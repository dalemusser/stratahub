// internal/app/system/orgutil/orgs.go
package orgutil

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Common errors returned by org resolution functions.
var (
	ErrUserNotFound   = errors.New("user not found")
	ErrNoOrganization = errors.New("user is not linked to an organization")
	ErrBadOrgID       = errors.New("bad organization id")
	ErrOrgNotFound    = errors.New("organization not found")
)

// OrgRow is a view model for displaying an organization in a list with a count.
// Used by members, leaders, groups, and reports list pages.
type OrgRow struct {
	ID    primitive.ObjectID
	Name  string
	Count int64
}

// ListActiveOrgs returns all active organizations.
func ListActiveOrgs(ctx context.Context, db *mongo.Database) ([]models.Organization, error) {
	cur, err := db.Collection("organizations").Find(ctx, bson.M{"status": "active"})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.Organization
	for cur.Next(ctx) {
		var o models.Organization
		if err := cur.Decode(&o); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// FetchOrgNames fetches organization names for the given org IDs.
// Returns a map of org ID to name. Missing orgs are simply not in the map.
func FetchOrgNames(ctx context.Context, db *mongo.Database, orgIDs []primitive.ObjectID) (map[primitive.ObjectID]string, error) {
	orgNames := make(map[primitive.ObjectID]string)
	if len(orgIDs) == 0 {
		return orgNames, nil
	}

	cur, err := db.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIDs}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var o models.Organization
		if err := cur.Decode(&o); err != nil {
			return nil, err
		}
		orgNames[o.ID] = o.Name
	}

	return orgNames, nil
}

// GetOrgName returns the organization name for the given ID.
// Returns empty string if not found (not an error).
func GetOrgName(ctx context.Context, db *mongo.Database, orgID primitive.ObjectID) (string, error) {
	if orgID.IsZero() {
		return "", nil
	}
	var org models.Organization
	err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": orgID}).Decode(&org)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return org.Name, nil
}

// ResolveLeaderOrg returns the organization ID and name for a leader user.
// Returns ErrUserNotFound if the user doesn't exist, ErrNoOrganization if
// the user has no organization_id set.
func ResolveLeaderOrg(ctx context.Context, db *mongo.Database, userID primitive.ObjectID) (primitive.ObjectID, string, error) {
	var u models.User
	err := db.Collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&u)
	if err == mongo.ErrNoDocuments {
		return primitive.NilObjectID, "", ErrUserNotFound
	}
	if err != nil {
		return primitive.NilObjectID, "", err
	}
	if u.OrganizationID == nil {
		return primitive.NilObjectID, "", ErrNoOrganization
	}

	orgName, err := GetOrgName(ctx, db, *u.OrganizationID)
	if err != nil {
		return primitive.NilObjectID, "", err
	}
	return *u.OrganizationID, orgName, nil
}

// ResolveOrgFromHex parses an org hex string, validates it exists, and returns
// the organization ID and name.
// Returns ErrBadOrgID if the hex is invalid, ErrOrgNotFound if org doesn't exist.
func ResolveOrgFromHex(ctx context.Context, db *mongo.Database, orgHex string) (primitive.ObjectID, string, error) {
	oid, err := primitive.ObjectIDFromHex(orgHex)
	if err != nil {
		return primitive.NilObjectID, "", ErrBadOrgID
	}

	var org models.Organization
	err = db.Collection("organizations").FindOne(ctx, bson.M{"_id": oid}).Decode(&org)
	if err == mongo.ErrNoDocuments {
		return primitive.NilObjectID, "", ErrOrgNotFound
	}
	if err != nil {
		return primitive.NilObjectID, "", err
	}
	return oid, org.Name, nil
}

// ErrOrgNotActive is returned when an organization exists but is not active.
var ErrOrgNotActive = errors.New("organization is not active")

// IsExpectedOrgError returns true if err is one of the expected org resolution
// errors (bad ID, not found, not active) rather than an unexpected DB error.
// Use this to distinguish user-fixable problems from server errors.
func IsExpectedOrgError(err error) bool {
	return errors.Is(err, ErrBadOrgID) ||
		errors.Is(err, ErrOrgNotFound) ||
		errors.Is(err, ErrOrgNotActive)
}

// ResolveActiveOrgFromHex parses an org hex string, validates it exists and is active,
// and returns the organization ID and name.
// Returns ErrBadOrgID if the hex is invalid, ErrOrgNotFound if org doesn't exist,
// ErrOrgNotActive if the org exists but is not active.
func ResolveActiveOrgFromHex(ctx context.Context, db *mongo.Database, orgHex string) (primitive.ObjectID, string, error) {
	oid, err := primitive.ObjectIDFromHex(orgHex)
	if err != nil {
		return primitive.NilObjectID, "", ErrBadOrgID
	}

	var org models.Organization
	err = db.Collection("organizations").FindOne(ctx, bson.M{"_id": oid}).Decode(&org)
	if err == mongo.ErrNoDocuments {
		return primitive.NilObjectID, "", ErrOrgNotFound
	}
	if err != nil {
		return primitive.NilObjectID, "", err
	}
	if org.Status != "active" {
		return primitive.NilObjectID, "", ErrOrgNotActive
	}
	return oid, org.Name, nil
}

// OrgOption represents an organization choice for form dropdowns.
type OrgOption struct {
	ID   primitive.ObjectID
	Name string
}

// LeaderOption represents a leader choice for form dropdowns.
type LeaderOption struct {
	ID       primitive.ObjectID
	FullName string
	OrgID    primitive.ObjectID
	OrgHex   string // populated for template filtering
}

// LoadActiveOrgOptions returns organization options for dropdowns plus the list of IDs.
// Results are sorted alphabetically by name (case-insensitive).
func LoadActiveOrgOptions(ctx context.Context, db *mongo.Database) ([]OrgOption, []primitive.ObjectID, error) {
	cur, err := db.Collection("organizations").Find(ctx, bson.M{"status": "active"})
	if err != nil {
		return nil, nil, err
	}
	defer cur.Close(ctx)

	var opts []OrgOption
	var ids []primitive.ObjectID
	for cur.Next(ctx) {
		var o models.Organization
		if err := cur.Decode(&o); err != nil {
			return nil, nil, err
		}
		opts = append(opts, OrgOption{ID: o.ID, Name: o.Name})
		ids = append(ids, o.ID)
	}

	// Sort A→Z by name (case-insensitive, stable on original)
	sort.SliceStable(opts, func(i, j int) bool {
		ni := strings.ToLower(opts[i].Name)
		nj := strings.ToLower(opts[j].Name)
		if ni == nj {
			return opts[i].Name < opts[j].Name
		}
		return ni < nj
	})

	return opts, ids, nil
}

// LoadActiveLeaders returns leader options for a set of org IDs.
// Pass nil or empty orgIDs to load leaders from all organizations.
// Results are sorted alphabetically by full name (case-insensitive).
func LoadActiveLeaders(ctx context.Context, db *mongo.Database, orgIDs []primitive.ObjectID) ([]LeaderOption, error) {
	filter := bson.M{"role": "leader", "status": "active"}
	if len(orgIDs) > 0 {
		filter["organization_id"] = bson.M{"$in": orgIDs}
	}

	cur, err := db.Collection("users").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []LeaderOption
	for cur.Next(ctx) {
		var u models.User
		if err := cur.Decode(&u); err != nil {
			return nil, err
		}
		if u.OrganizationID != nil {
			out = append(out, LeaderOption{
				ID:       u.ID,
				FullName: u.FullName,
				OrgID:    *u.OrganizationID,
				OrgHex:   u.OrganizationID.Hex(),
			})
		}
	}

	// Sort A→Z by full name (case-insensitive, stable)
	sort.SliceStable(out, func(i, j int) bool {
		ni := strings.ToLower(out[i].FullName)
		nj := strings.ToLower(out[j].FullName)
		if ni == nj {
			return out[i].FullName < out[j].FullName
		}
		return ni < nj
	})

	return out, nil
}

// ResolveOrgLocation resolves the time.Location and timezone label for an
// organization by ID.
//
// It looks up the organization, reads org.TimeZone, and:
//
//   - returns loc = time.Local and label = "" if the org has no timezone set
//   - otherwise, tries time.LoadLocation(tzID) and returns that loc + friendly label
//
// Callers are expected to:
//
//   - use loc for all time calculations (parsing, formatting)
//   - use label to display the timezone to the user
func ResolveOrgLocation(ctx context.Context, db *mongo.Database, orgID primitive.ObjectID) (*time.Location, string) {
	loc := time.Local
	label := ""

	if orgID.IsZero() {
		return loc, label
	}

	var org models.Organization
	err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": orgID}).Decode(&org)
	if err != nil {
		if err != mongo.ErrNoDocuments {
			zap.L().Warn("ResolveOrgLocation: org lookup failed", zap.Error(err))
		}
		return loc, label
	}

	tzID := strings.TrimSpace(org.TimeZone)
	if tzID == "" {
		return loc, label
	}

	// Get friendly label from timezones module
	label = timezones.Label(tzID)

	// Load the location
	if l, err := time.LoadLocation(tzID); err == nil {
		loc = l
	} else {
		zap.L().Warn("ResolveOrgLocation: LoadLocation failed", zap.Error(err), zap.String("tz", tzID))
	}

	return loc, label
}
