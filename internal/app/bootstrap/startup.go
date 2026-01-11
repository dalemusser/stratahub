// internal/app/bootstrap/startup.go
package bootstrap

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"github.com/dalemusser/stratahub/internal/app/store/emailverify"
	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/workers"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/config"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Startup runs once after DB connections and schema/index setup are complete,
// but before the HTTP handler is built and requests are served.
//
// This is the place for one-time initialization that depends on having live
// database connections and fully loaded configuration. Unlike ConnectDB and
// EnsureSchema which focus on infrastructure, Startup is for application-level
// initialization.
//
// Common uses for Startup:
//   - Load shared templates from the resources directory
//   - Warm caches with frequently accessed data
//   - Initialize in-memory lookup tables
//   - Validate external service connectivity
//   - Set up background workers or scheduled tasks
//   - Perform health checks on dependencies
//
// Returning a non-nil error will abort startup and prevent the server from
// starting. Returning nil signals that initialization succeeded.
//
// The context will be cancelled if the process is asked to shut down while
// Startup is running; honor it in any long-running work.
//
// StrataHub uses Startup to load shared templates (layouts, navigation, etc.)
// that are used across all features.
func Startup(ctx context.Context, coreCfg *config.CoreConfig, appCfg AppConfig, deps DBDeps, logger *zap.Logger) error {
	resources.LoadSharedTemplates()

	// Bootstrap default workspace if none exists
	// Note: Workspace indexes are created in EnsureSchema via indexes.EnsureAll()
	wsStore := workspacestore.New(deps.StrataHubMongoDatabase)
	ws, err := wsStore.EnsureDefault(ctx, appCfg.DefaultWorkspaceName, appCfg.DefaultWorkspaceSubdomain)
	if err != nil {
		logger.Error("failed to bootstrap default workspace", zap.Error(err))
		return err
	}
	logger.Info("workspace ready",
		zap.String("workspace_id", ws.ID.Hex()),
		zap.String("name", ws.Name),
		zap.String("subdomain", ws.Subdomain))

	// Migrate existing data to default workspace if needed
	if err := migrateDataToWorkspace(ctx, deps, ws.ID, logger); err != nil {
		logger.Error("failed to migrate data to workspace", zap.Error(err))
		return err
	}

	// Bootstrap superadmin user if configured
	if appCfg.SuperAdminEmail != "" {
		if err := ensureSuperAdmin(ctx, deps, appCfg.SuperAdminEmail, logger); err != nil {
			logger.Error("failed to bootstrap superadmin", zap.Error(err))
			return err
		}
	}

	// Ensure indexes for email verification store
	emailVerifyStore := emailverify.New(deps.StrataHubMongoDatabase, appCfg.EmailVerifyExpiry)
	if err := emailVerifyStore.EnsureIndexes(ctx); err != nil {
		logger.Error("failed to ensure email verify indexes", zap.Error(err))
		return err
	}

	// Ensure indexes for audit store
	auditStore := audit.New(deps.StrataHubMongoDatabase)
	if err := auditStore.EnsureIndexes(ctx); err != nil {
		logger.Error("failed to ensure audit indexes", zap.Error(err))
		return err
	}

	// Ensure indexes for sessions store (activity tracking)
	sessionsStore := sessions.New(deps.StrataHubMongoDatabase)
	if err := sessionsStore.EnsureIndexes(ctx); err != nil {
		logger.Error("failed to ensure sessions indexes", zap.Error(err))
		return err
	}

	// Ensure indexes for activity events store
	activityStore := activity.New(deps.StrataHubMongoDatabase)
	if err := activityStore.EnsureIndexes(ctx); err != nil {
		logger.Error("failed to ensure activity indexes", zap.Error(err))
		return err
	}

	// Start session cleanup background worker
	// Runs every minute, closes sessions inactive for more than 10 minutes
	deps.SessionCleanupWorker = workers.NewSessionCleanup(
		sessionsStore,
		logger,
		1*time.Minute,  // check every minute
		10*time.Minute, // close sessions inactive for 10+ minutes
	)
	deps.SessionCleanupWorker.Start()

	return nil
}

// ensureSuperAdmin ensures a superadmin user exists with the given login_id.
// If a user exists with this login_id, promote them to superadmin.
// If no user exists, create a new superadmin user.
func ensureSuperAdmin(ctx context.Context, deps DBDeps, loginID string, logger *zap.Logger) error {
	db := deps.StrataHubMongoDatabase
	coll := db.Collection("users")

	loginID = strings.ToLower(strings.TrimSpace(loginID))

	// Check if user exists with this login_id
	var existingUser models.User
	err := coll.FindOne(ctx, bson.M{"login_id": loginID}).Decode(&existingUser)

	if err == nil {
		// User exists
		if existingUser.Role == "superadmin" {
			logger.Debug("superadmin user already configured", zap.String("login_id", loginID))
			return nil
		}

		// Promote to superadmin
		_, err = coll.UpdateByID(ctx, existingUser.ID, bson.M{
			"$set": bson.M{
				"role":         "superadmin",
				"workspace_id": nil,
				"updated_at":   time.Now().UTC(),
			},
		})
		if err != nil {
			return err
		}
		logger.Info("promoted existing user to superadmin",
			zap.String("login_id", loginID),
			zap.String("user_id", existingUser.ID.Hex()),
			zap.String("previous_role", existingUser.Role))
		return nil
	}

	if err != mongo.ErrNoDocuments {
		return err
	}

	// Create new superadmin user
	now := time.Now().UTC()
	newUser := models.User{
		ID:           primitive.NewObjectID(),
		WorkspaceID:  nil, // Superadmins have no workspace_id
		FullName:     "SuperAdmin",
		FullNameCI:   text.Fold("SuperAdmin"),
		Email:        nil,
		LoginID:      &loginID,
		LoginIDCI:    ptrString(text.Fold(loginID)),
		AuthMethod:   "email", // Default to email auth for new superadmin
		Role:         "superadmin",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err = coll.InsertOne(ctx, newUser)
	if err != nil {
		return err
	}

	logger.Info("created superadmin user",
		zap.String("login_id", loginID),
		zap.String("user_id", newUser.ID.Hex()))
	return nil
}

func ptrString(s string) *string {
	return &s
}

// migrateDataToWorkspace assigns existing documents without workspace_id to the default workspace.
// This is a one-time migration for existing data when multi-workspace support is added.
func migrateDataToWorkspace(ctx context.Context, deps DBDeps, workspaceID primitive.ObjectID, logger *zap.Logger) error {
	db := deps.StrataHubMongoDatabase

	// Collections that need workspace_id
	collections := []string{
		"users",
		"organizations",
		"groups",
		"resources",
		"materials",
		"site_settings",
	}

	// Filter: documents without workspace_id field, with null value, or with NilObjectID
	filter := bson.M{
		"$or": []bson.M{
			{"workspace_id": bson.M{"$exists": false}},
			{"workspace_id": nil},
			{"workspace_id": primitive.NilObjectID},
		},
	}

	// Update: set workspace_id to the default workspace
	update := bson.M{
		"$set": bson.M{"workspace_id": workspaceID},
	}

	for _, collName := range collections {
		coll := db.Collection(collName)
		result, err := coll.UpdateMany(ctx, filter, update)
		if err != nil {
			logger.Error("failed to migrate collection to workspace",
				zap.String("collection", collName),
				zap.Error(err))
			return err
		}

		if result.ModifiedCount > 0 {
			logger.Info("migrated documents to workspace",
				zap.String("collection", collName),
				zap.Int64("count", result.ModifiedCount),
				zap.String("workspace_id", workspaceID.Hex()))
		}
	}

	return nil
}
