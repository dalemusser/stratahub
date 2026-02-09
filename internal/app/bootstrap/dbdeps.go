// internal/app/bootstrap/dbdeps.go
package bootstrap

import (
	"github.com/dalemusser/stratahub/internal/app/system/mailer"
	"github.com/dalemusser/stratahub/internal/app/system/tasks"
	"github.com/dalemusser/waffle/pantry/storage"
	"go.mongodb.org/mongo-driver/mongo"
)

// DBDeps holds database and backend dependencies for this WAFFLE app.
//
// This struct is created in ConnectDB and passed to subsequent lifecycle
// hooks: EnsureSchema, Startup, BuildHandler, and Shutdown. It serves as
// the central place to store all database clients and backend connections
// that your application needs.
//
// Design guidelines:
//   - Add a field for each database or backend service you connect to
//   - Use pointer types for clients that may be nil (optional backends)
//   - Group related dependencies together with comments
//   - Consider adding helper methods for common operations
//
// The Shutdown hook is responsible for closing these connections gracefully
// when the application terminates.
type DBDeps struct {
	// MongoDB client and database for StrataHub data
	StrataHubMongoClient   *mongo.Client
	StrataHubMongoDatabase *mongo.Database

	// MHSGrader database for reading progress grades (same cluster, different database)
	MHSGraderDatabase *mongo.Database

	// FileStorage for material file uploads
	FileStorage storage.Store

	// Mailer for sending emails (verification codes, etc.)
	Mailer *mailer.Mailer

	// Background task runner for cleanup jobs
	TaskRunner *tasks.Runner
}
