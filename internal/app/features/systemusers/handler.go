// internal/app/features/systemusers/handler.go
package systemusers

import (
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

type Handler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

// NewHandler constructs a System Users feature handler bound to
// the given Mongo database and logger.
func NewHandler(db *mongo.Database, logger *zap.Logger) *Handler {
	return &Handler{
		DB:  db,
		Log: logger,
	}
}
