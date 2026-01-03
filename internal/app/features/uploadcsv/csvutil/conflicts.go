// Package csvutil provides CSV parsing and formatting utilities.
package csvutil

import "go.mongodb.org/mongo-driver/bson/primitive"

// LoginConflict represents a member login ID that conflicts with
// an existing member in a different organization.
type LoginConflict struct {
	LoginID string
	OrgID   primitive.ObjectID
	OrgName string
}
