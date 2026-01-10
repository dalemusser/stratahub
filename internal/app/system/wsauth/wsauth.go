// internal/app/system/wsauth/wsauth.go
// Package wsauth provides workspace-aware authentication method helpers.
package wsauth

import (
	"context"
	"net/http"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// GetEnabledAuthMethods returns the enabled auth methods for the workspace in the request context.
// If no workspace context exists (e.g., apex domain), returns all auth methods.
// If no settings exist for the workspace, returns all auth methods (default).
func GetEnabledAuthMethods(ctx context.Context, r *http.Request, db *mongo.Database) []models.AuthMethod {
	wsID := workspace.IDFromRequest(r)
	if wsID == primitive.NilObjectID {
		return models.AllAuthMethods
	}
	return GetEnabledAuthMethodsForWorkspace(ctx, db, wsID)
}

// GetEnabledAuthMethodsForWorkspace returns the enabled auth methods for a specific workspace.
// If no settings exist for the workspace, returns all auth methods (default).
func GetEnabledAuthMethodsForWorkspace(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID) []models.AuthMethod {
	store := settingsstore.New(db)
	settings, err := store.Get(ctx, wsID)
	if err != nil {
		// On error, default to all methods
		return models.AllAuthMethods
	}
	return settings.GetEnabledAuthMethods()
}

// IsAuthMethodEnabled checks if a specific auth method is enabled for the workspace in the request context.
// If no workspace context exists (e.g., apex domain), checks against all valid auth methods.
// If no settings exist for the workspace, all valid methods are considered enabled (default).
func IsAuthMethodEnabled(ctx context.Context, r *http.Request, db *mongo.Database, method string) bool {
	wsID := workspace.IDFromRequest(r)
	if wsID == primitive.NilObjectID {
		return models.IsValidAuthMethod(method)
	}
	return IsAuthMethodEnabledForWorkspace(ctx, db, wsID, method)
}

// IsAuthMethodEnabledForWorkspace checks if a specific auth method is enabled for a specific workspace.
// If no settings exist for the workspace, all valid methods are considered enabled (default).
func IsAuthMethodEnabledForWorkspace(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID, method string) bool {
	store := settingsstore.New(db)
	settings, err := store.Get(ctx, wsID)
	if err != nil {
		// On error, fall back to global check
		return models.IsValidAuthMethod(method)
	}
	return settings.IsAuthMethodEnabled(method)
}

// GetEnabledAuthMethodMap returns a map of enabled auth method values for quick lookup.
// Useful for template checkbox checked state.
func GetEnabledAuthMethodMap(ctx context.Context, r *http.Request, db *mongo.Database) map[string]bool {
	methods := GetEnabledAuthMethods(ctx, r, db)
	result := make(map[string]bool)
	for _, m := range methods {
		result[m.Value] = true
	}
	return result
}

// GetEnabledAuthMethodMapForWorkspace returns a map of enabled auth method values for a specific workspace.
func GetEnabledAuthMethodMapForWorkspace(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID) map[string]bool {
	methods := GetEnabledAuthMethodsForWorkspace(ctx, db, wsID)
	result := make(map[string]bool)
	for _, m := range methods {
		result[m.Value] = true
	}
	return result
}
