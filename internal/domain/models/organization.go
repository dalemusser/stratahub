// internal/domain/models/organizations.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Organization includes case/diacritic-insensitive fields for search/sort.
type Organization struct {
	ID          primitive.ObjectID `bson:"_id"`
	Name        string             `bson:"name"`
	NameCI      string             `bson:"name_ci"` // ← always stored
	City        string             `bson:"city"`
	CityCI      string             `bson:"city_ci"` // ← always stored
	State       string             `bson:"state"`
	StateCI     string             `bson:"state_ci"` // ← always stored
	TimeZone    string             `bson:"time_zone"`
	ContactInfo string             `bson:"contact_info"`
	Status      string             `bson:"status"`
	CreatedAt   time.Time          `bson:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"`
}
