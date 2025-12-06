// internal/app/features/resources/handler.go
package resources

import (
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Standard timeouts used by the Resources feature.
const (
	resourcesShortTimeout = 5 * time.Second
	resourcesMedTimeout   = 10 * time.Second
	resourcesLongTimeout  = 30 * time.Second
)

// AdminHandler owns all admin/library-facing Resource handlers
// (list, new, edit, view, delete, manage modal).
//
// It is constructed once at startup in bootstrap, using the
// shared Mongo database handle and logger.
type AdminHandler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

// MemberHandler owns the member-facing Resource handlers
// ("My Resources" list and individual resource view).
//
// It shares the same underlying DB and logger pattern as
// AdminHandler but keeps the responsibilities clearly separated.
type MemberHandler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

// NewAdminHandler constructs an AdminHandler bound to the
// given Mongo database and logger.
func NewAdminHandler(db *mongo.Database, logger *zap.Logger) *AdminHandler {
	return &AdminHandler{
		DB:  db,
		Log: logger,
	}
}

// NewMemberHandler constructs a MemberHandler bound to the
// given Mongo database and logger.
func NewMemberHandler(db *mongo.Database, logger *zap.Logger) *MemberHandler {
	return &MemberHandler{
		DB:  db,
		Log: logger,
	}
}
