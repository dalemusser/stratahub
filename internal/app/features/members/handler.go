// internal/app/features/members/handler.go
package members

import (
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler is the feature-level handler for Members.
// It holds the DB handle and logger provided by WAFFLE DBDeps / Startup.
type Handler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

func NewHandler(db *mongo.Database, logger *zap.Logger) *Handler {
	return &Handler{
		DB:  db,
		Log: logger,
	}
}
