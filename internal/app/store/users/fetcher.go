package userstore

import (
	"context"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Fetcher implements auth.UserFetcher to load fresh user data on each request.
// It fetches user and organization data from MongoDB.
type Fetcher struct {
	users            *mongo.Collection
	orgs             *mongo.Collection
	coordAssignments *mongo.Collection
}

// NewFetcher creates a UserFetcher that queries the given database.
func NewFetcher(db *mongo.Database) *Fetcher {
	return &Fetcher{
		users:            db.Collection("users"),
		orgs:             db.Collection("organizations"),
		coordAssignments: db.Collection("coordinator_assignments"),
	}
}

// FetchUser retrieves a user by ID and returns nil if the user is not found,
// disabled, or if any error occurs. This implements auth.UserFetcher.
func (f *Fetcher) FetchUser(ctx context.Context, userID string) *auth.SessionUser {
	// Parse the user ID
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil
	}

	// Use a short timeout for the DB query
	ctx, cancel := context.WithTimeout(ctx, timeouts.Short())
	defer cancel()

	// Fetch the user with projection for only needed fields
	var u models.User
	proj := options.FindOne().SetProjection(bson.M{
		"_id":                  1,
		"full_name":            1,
		"login_id":             1,
		"role":                 1,
		"status":               1,
		"organization_id":      1,
		"can_manage_materials": 1,
		"can_manage_resources": 1,
	})

	if err := f.users.FindOne(ctx, bson.M{"_id": oid}, proj).Decode(&u); err != nil {
		// User not found or DB error
		return nil
	}

	// Check if user is disabled
	if normalize.Status(u.Status) == "disabled" {
		return nil
	}

	// Build the session user
	loginID := ""
	if u.LoginID != nil {
		loginID = *u.LoginID
	}
	su := &auth.SessionUser{
		ID:      u.ID.Hex(),
		Name:    u.FullName,
		LoginID: loginID,
		Role:    normalize.Role(u.Role),
	}

	// If user has a single organization (leaders/members), fetch the org name
	if u.OrganizationID != nil {
		su.OrganizationID = u.OrganizationID.Hex()

		var org models.Organization
		orgProj := options.FindOne().SetProjection(bson.M{"name": 1})
		if err := f.orgs.FindOne(ctx, bson.M{"_id": u.OrganizationID}, orgProj).Decode(&org); err == nil {
			su.OrganizationName = org.Name
		}
		// If org fetch fails, we still return the user with empty org name
	}

	// For coordinators, fetch their organization assignments and set permissions
	if su.Role == "coordinator" {
		// Set coordinator-specific permissions
		su.CanManageMaterials = u.CanManageMaterials
		su.CanManageResources = u.CanManageResources

		cur, err := f.coordAssignments.Find(ctx, bson.M{"user_id": oid})
		if err == nil {
			defer cur.Close(ctx)
			for cur.Next(ctx) {
				var ca struct {
					OrganizationID primitive.ObjectID `bson:"organization_id"`
				}
				if cur.Decode(&ca) == nil {
					su.OrganizationIDs = append(su.OrganizationIDs, ca.OrganizationID.Hex())
				}
			}
		}
		// If query fails, coordinator will have empty org list (safe fallback)
	}

	return su
}
