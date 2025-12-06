// internal/app/features/groups/handler.go
package groups

import (
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the shared dependency container for the groups feature.
// It holds references to the Mongo database and the logger so that
// the various handlers (list, meta, manage, upload CSV, assignments)
// can all share the same core dependencies.
type Handler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

// NewHandler constructs a new groups Handler. It is typically called
// from the bootstrap BuildHandler function, where the application's
// DB and logger are already initialized.
func NewHandler(db *mongo.Database, logger *zap.Logger) *Handler {
	return &Handler{
		DB:  db,
		Log: logger,
	}
}
