// internal/domain/models/loginhistory.go
package models

import "time"

// LoginRecord captures a single successful login event.
// CreatedAt is indexed for recent-activity views.
type LoginRecord struct {
	UserID    string    `bson:"user_id"`
	CreatedAt time.Time `bson:"created_at"`
	IP        string    `bson:"ip"`
	Provider  string    `bson:"provider"`
}
