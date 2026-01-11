package userstore

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

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
	"go.uber.org/zap"
)

// Fetcher implements auth.UserFetcher to load fresh user data on each request.
// It fetches user and organization data from MongoDB.
type Fetcher struct {
	users            *mongo.Collection
	orgs             *mongo.Collection
	coordAssignments *mongo.Collection
	workspaces       *mongo.Collection
	logger           *zap.Logger
}

// NewFetcher creates a UserFetcher that queries the given database.
func NewFetcher(db *mongo.Database, logger *zap.Logger) *Fetcher {
	return &Fetcher{
		users:            db.Collection("users"),
		orgs:             db.Collection("organizations"),
		coordAssignments: db.Collection("coordinator_assignments"),
		workspaces:       db.Collection("workspaces"),
		logger:           logger,
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
		"login_id_ci":          1,
		"auth_method":          1,
		"role":                 1,
		"status":               1,
		"organization_id":      1,
		"workspace_id":         1,
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

	// Set workspace fields
	if u.WorkspaceID != nil {
		su.WorkspaceID = u.WorkspaceID.Hex()
	}

	// Check for superadmin role
	if su.Role == "superadmin" {
		su.IsSuperAdmin = true
		// Superadmins can access all active workspaces
		su.WorkspaceIDs = f.fetchAllActiveWorkspaceIDs(ctx)
	} else {
		// Regular users - find all workspaces where they have an account
		// with the same login_id_ci and auth_method
		su.WorkspaceIDs = f.fetchUserWorkspaceIDs(ctx, u.LoginIDCI, u.AuthMethod)
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

// fetchAllActiveWorkspaceIDs returns all active workspace IDs for superadmins.
func (f *Fetcher) fetchAllActiveWorkspaceIDs(ctx context.Context) []string {
	var ids []string
	cur, err := f.workspaces.Find(ctx, bson.M{"status": "active"}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		f.logger.Warn("failed to fetch workspaces for superadmin", zap.Error(err))
		return ids
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var ws struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if cur.Decode(&ws) == nil {
			ids = append(ids, ws.ID.Hex())
		}
	}
	return ids
}

// fetchUserWorkspaceIDs returns workspace IDs where the user has an account.
// This allows users to have accounts in multiple workspaces with the same login credentials.
func (f *Fetcher) fetchUserWorkspaceIDs(ctx context.Context, loginIDCI *string, authMethod string) []string {
	var ids []string

	if loginIDCI == nil || *loginIDCI == "" {
		return ids
	}

	// Find all users with the same login_id_ci and auth_method (across workspaces)
	cur, err := f.users.Find(ctx, bson.M{
		"login_id_ci": *loginIDCI,
		"auth_method": authMethod,
		"status":      "active",
	}, options.Find().SetProjection(bson.M{"workspace_id": 1}))
	if err != nil {
		f.logger.Warn("failed to fetch user workspaces", zap.Error(err), zap.Stringp("login_id_ci", loginIDCI))
		return ids
	}
	defer cur.Close(ctx)

	seen := make(map[string]bool)
	for cur.Next(ctx) {
		var u struct {
			WorkspaceID *primitive.ObjectID `bson:"workspace_id"`
		}
		if cur.Decode(&u) == nil && u.WorkspaceID != nil {
			wsID := u.WorkspaceID.Hex()
			if !seen[wsID] {
				seen[wsID] = true
				ids = append(ids, wsID)
			}
		}
	}
	return ids
}
