// internal/app/system/validators/validators.go
package validators

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"errors"
	"strings"

	"github.com/dalemusser/stratahub/internal/domain/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// EnsureAll creates collections (if missing) and tries to attach JSON-Schema
// validators. On servers that don't support collMod/validators (e.g. some
// DocumentDB versions), we log and skip gracefully.
func EnsureAll(ctx context.Context, db *mongo.Database) error {
	var problems []string

	// helper: ensure collection exists (with truthful logging) and then validator (if provided)
	ensure := func(coll string, schema bson.M) {
		if _, err := ensureCollection(ctx, db, coll); err != nil {
			problems = append(problems, coll+": "+err.Error())
			return
		}
		if schema == nil {
			return
		}
		if err := setValidator(ctx, db, coll, schema); err != nil {
			// DocumentDB or other deployments may not support collMod/validators.
			if isNoSuchCommand(err) || isNotImplemented(err) {
				zap.L().Info("validator skipped (unsupported)", zap.String("collection", coll))
				return
			}
			problems = append(problems, coll+": "+err.Error())
		}
	}

	// Core collections this app uses
	ensure("users", usersSchema())
	ensure("organizations", orgsSchema())
	ensure("groups", groupsSchema())
	ensure("resources", resourcesSchema())

	// Membership and assignment collections
	ensure("group_memberships", groupMembershipsSchema())
	ensure("group_resource_assignments", groupResourceAssignmentsSchema())
	ensure("coordinator_assignments", coordinatorAssignmentsSchema())

	// These don't strictly need validators; we still ensure the collections exist.
	ensure("login_records", nil)

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

/* ---------------------- collection helpers & logging ---------------------- */

// collectionExists returns true when <name> already exists.
// Uses ListCollectionNames to avoid "created collection" log when it didn't.
func collectionExists(ctx context.Context, db *mongo.Database, name string) (bool, error) {
	names, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}

// ensureCollection idempotently makes sure <name> exists.
// Returns created==true only if we actually created it.
func ensureCollection(ctx context.Context, db *mongo.Database, name string) (created bool, err error) {
	exists, listErr := collectionExists(ctx, db, name)
	if listErr == nil && exists {
		zap.L().Info("collection exists", zap.String("collection", name))
		return false, nil
	}
	// If listing failed, fall back to create-and-handle-race.
	if err := db.CreateCollection(ctx, name); err != nil {
		// NamespaceExists / already exists is fine (race or prior run).
		if isNamespaceExistsErr(err) {
			zap.L().Info("collection exists", zap.String("collection", name))
			return false, nil
		}
		zap.L().Warn("createCollection failed", zap.String("collection", name), zap.Error(err))
		return false, err
	}
	zap.L().Info("created collection", zap.String("collection", name))
	return true, nil
}

/* ------------------------------ validators ------------------------------- */

func setValidator(ctx context.Context, db *mongo.Database, name string, validator bson.M) error {
	cmd := bson.D{
		{Key: "collMod", Value: name},
		{Key: "validator", Value: validator},
		{Key: "validationLevel", Value: "moderate"},
		{Key: "validationAction", Value: "error"},
	}
	var out bson.M
	if err := db.RunCommand(ctx, cmd).Decode(&out); err != nil {
		return err
	}
	zap.L().Info("validator ensured", zap.String("collection", name))
	return nil
}

/* ------------------------- error helpers ------------------------- */

func isNamespaceExistsErr(err error) bool {
	if err == nil {
		return false
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) && (ce.Code == 48 || strings.Contains(strings.ToLower(ce.Message), "already exists")) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "already exists") || strings.Contains(s, "namespace exists")
}

func isNoSuchCommand(err error) bool {
	if err == nil {
		return false
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) && (ce.Code == 59 || strings.Contains(strings.ToLower(ce.Message), "no such command")) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such command")
}

func isNotImplemented(err error) bool {
	if err == nil {
		return false
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) && (ce.Code == 115 ||
		strings.Contains(strings.ToLower(ce.Message), "not implemented") ||
		strings.Contains(strings.ToLower(ce.Message), "not supported")) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not implemented") || strings.Contains(s, "not supported")
}

/* ------------------------- JSON-Schema docs ---------------------- */

func usersSchema() bson.M {
	return bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object",
			"required": bson.A{"full_name", "role", "status", "auth_method"},
			"properties": bson.M{
				"full_name":    bson.M{"bsonType": "string", "minLength": 1, "pattern": ".*\\S.*"},
				"full_name_ci": bson.M{"bsonType": "string", "minLength": 1, "pattern": ".*\\S.*"},
				"login_id":       bson.M{"bsonType": "string"},
				"login_id_ci":    bson.M{"bsonType": "string"},
				"auth_return_id": bson.M{"bsonType": bson.A{"string", "null"}},
				"email":          bson.M{"bsonType": bson.A{"string", "null"}},
				"role":           bson.M{"enum": bson.A{"superadmin", "admin", "analyst", "coordinator", "leader", "member", "guest"}},
				"status":       bson.M{"enum": bson.A{"active", "disabled"}},
				"auth_method":  bson.M{"enum": bson.A{"internal", "google", "classlink", "clever", "microsoft", "schoology", "email", "password", "trust"}},
			},
		},
	}
}

func orgsSchema() bson.M {
	return bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object",
			"required": bson.A{"name", "name_ci", "status", "time_zone"},
			"properties": bson.M{
				"name":      bson.M{"bsonType": "string", "minLength": 1, "pattern": ".*\\S.*"},
				"name_ci":   bson.M{"bsonType": "string", "minLength": 1, "pattern": ".*\\S.*"},
				"status":    bson.M{"enum": bson.A{"active"}},
				"time_zone": bson.M{"bsonType": "string", "minLength": 1, "pattern": ".*\\S.*"},
			},
		},
	}
}

func groupsSchema() bson.M {
	return bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object",
			"required": bson.A{"organization_id", "name", "name_ci", "status"},
			"properties": bson.M{
				"organization_id": bson.M{"bsonType": "objectId"},
				"name":            bson.M{"bsonType": "string", "minLength": 1, "pattern": ".*\\S.*"},
				"name_ci":         bson.M{"bsonType": "string", "minLength": 1, "pattern": ".*\\S.*"},
				"status":          bson.M{"enum": bson.A{"active"}},
			},
		},
	}
}

func groupMembershipsSchema() bson.M {
	return bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object",
			"required": bson.A{"user_id", "group_id", "org_id", "role"},
			"properties": bson.M{
				"user_id":    bson.M{"bsonType": "objectId"},
				"group_id":   bson.M{"bsonType": "objectId"},
				"org_id":     bson.M{"bsonType": "objectId"},
				"role":       bson.M{"enum": bson.A{"leader", "member"}},
				"created_at": bson.M{"bsonType": "date"},
			},
		},
	}
}

func groupResourceAssignmentsSchema() bson.M {
	return bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object",
			"required": bson.A{"group_id", "resource_id", "organization_id", "created_at"},
			"properties": bson.M{
				"group_id":        bson.M{"bsonType": "objectId"},
				"resource_id":     bson.M{"bsonType": "objectId"},
				"organization_id": bson.M{"bsonType": "objectId"},
				"visible_from":    bson.M{"bsonType": "date"},
				"visible_until":   bson.M{"bsonType": "date"},
				"instructions":    bson.M{"bsonType": "string"},
				"created_at":      bson.M{"bsonType": "date"},
				"updated_at":      bson.M{"bsonType": "date"},
				"created_by_id":   bson.M{"bsonType": "objectId"},
				"created_by_name": bson.M{"bsonType": "string"},
				"updated_by_id":   bson.M{"bsonType": "objectId"},
				"updated_by_name": bson.M{"bsonType": "string"},
			},
		},
	}
}

func resourcesSchema() bson.M {
	// Build the enum for the resource type field from the canonical list in the domain models.
	typeEnum := bson.A{}
	for _, t := range models.ResourceTypes {
		typeEnum = append(typeEnum, t)
	}

	return bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object",
			"required": bson.A{"title", "title_ci", "status", "type"},
			"properties": bson.M{
				"title":    bson.M{"bsonType": "string", "minLength": 1, "pattern": ".*\\S.*"},
				"title_ci": bson.M{"bsonType": "string", "minLength": 1, "pattern": ".*\\S.*"},

				"subject":    bson.M{"bsonType": "string"},
				"subject_ci": bson.M{"bsonType": "string"},

				"status": bson.M{"enum": bson.A{"active", "disabled"}},

				"type": bson.M{
					"bsonType": "string",
					"enum":     typeEnum,
				},

				"launch_url":           bson.M{"bsonType": "string"},
				"show_in_library":      bson.M{"bsonType": "bool"},
				"description":          bson.M{"bsonType": "string"},
				"default_instructions": bson.M{"bsonType": "string"},
			},
		},
	}
}

func coordinatorAssignmentsSchema() bson.M {
	return bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object",
			"required": bson.A{"user_id", "organization_id", "created_at"},
			"properties": bson.M{
				"user_id":         bson.M{"bsonType": "objectId"},
				"organization_id": bson.M{"bsonType": "objectId"},
				"created_at":      bson.M{"bsonType": "date"},
				"created_by_id":   bson.M{"bsonType": "objectId"},
				"created_by_name": bson.M{"bsonType": "string"},
			},
		},
	}
}
