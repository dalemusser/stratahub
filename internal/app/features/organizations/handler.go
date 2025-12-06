// internal/app/features/organizations/handler.go
package organizations

import (
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

const (
	orgsShortTimeout = 5 * time.Second
	orgsMedTimeout   = 10 * time.Second
	orgsLongTimeout  = 30 * time.Second
)

// Handler is the feature-level entry point for Organizations.
type Handler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

// NewHandler constructs a new Organizations handler bound to a DB and logger.
func NewHandler(db *mongo.Database, logger *zap.Logger) *Handler {
	return &Handler{
		DB:  db,
		Log: logger,
	}
}
