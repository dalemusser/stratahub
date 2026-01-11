// Package csvutil provides CSV parsing and formatting utilities.
package csvutil

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import "go.mongodb.org/mongo-driver/bson/primitive"

// LoginConflict represents a member login ID that conflicts with
// an existing member in a different organization.
type LoginConflict struct {
	LoginID string
	OrgID   primitive.ObjectID
	OrgName string
}
