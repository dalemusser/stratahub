// internal/app/bootstrap/dbdeps.go
package bootstrap

import (
	"go.mongodb.org/mongo-driver/mongo"
)

// DBDeps holds database/back-end dependencies for the app.
// Extend this struct as your app evolves.
type DBDeps struct {
	StrataHubMongoClient   *mongo.Client
	StrataHubMongoDatabase *mongo.Database
}
