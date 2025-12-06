// internal/app/features/reports/handler.go
package reports

import (
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Standard timeouts used by the Reports feature (Members Report + CSV).
const (
	reportsShortTimeout = 5 * time.Second
	reportsMedTimeout   = 10 * time.Second
	reportsLongTimeout  = 30 * time.Second
)

// Handler owns all Members Report handlers (HTML page + CSV export).
//
// It follows the same pattern as other features in stratahub:
// a thin struct wrapping the shared Mongo database handle and logger,
// constructed once at startup in bootstrap and passed into Routes().
type Handler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

// NewHandler constructs a reports Handler bound to the given Mongo
// database and logger.
func NewHandler(db *mongo.Database, logger *zap.Logger) *Handler {
	return &Handler{
		DB:  db,
		Log: logger,
	}
}
