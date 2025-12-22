// internal/app/store/users/upsert_member.go
package userstore

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/status"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// itoa is a shorthand for strconv.Itoa
func itoa(i int) string { return strconv.Itoa(i) }

// UpsertMemberInOrg creates or updates a *member* inside the given orgID only.
// Returns (updated=true) if an existing user in the same org was updated.
// Returns (conflictErr!=nil) if an email exists in a different org.
// Returns err on database errors.
//
// NOTE: This does not move users between organizations.
func (s *Store) UpsertMembersInOrg(
	ctx context.Context,
	orgID primitive.ObjectID,
	fullName, email, authMethod string,
) (updated bool, conflictErr error, err error) {

	email = normalize.Email(email)
	fullName = normalize.Name(fullName)
	authMethod = normalize.AuthMethod(authMethod)
	if email == "" || fullName == "" {
		return false, nil, nil
	}

	// Capture timestamp once for consistent created_at/updated_at across both branches.
	now := time.Now()

	// Lookup by email once to decide create/update/conflict.
	var existing struct {
		ID   primitive.ObjectID `bson:"_id"`
		Org  primitive.ObjectID `bson:"organization_id"`
		Role string             `bson:"role"`
	}
	findErr := s.c.FindOne(ctx, bson.M{"email": email}).Decode(&existing)
	switch findErr {
	case mongo.ErrNoDocuments:
		// Insert new member in this org
		doc := bson.M{
			"full_name":       fullName,
			"full_name_ci":    text.Fold(fullName),
			"email":           email,
			"role":            "member",
			"organization_id": orgID,
			"status":          status.Active,
			"auth_method":     authMethod,
			"created_at":      now,
			"updated_at":      now,
		}
		_, err := s.c.InsertOne(ctx, doc)
		return false, nil, err

	default:
		if findErr != nil {
			return false, nil, findErr
		}
		// Exists: only update if already in this org; otherwise conflict.
		if existing.Org != orgID {
			return false, ErrDifferentOrg, nil
		}
		_, err := s.c.UpdateByID(ctx, existing.ID, bson.M{
			"$set": bson.M{
				"full_name":    fullName,
				"full_name_ci": text.Fold(fullName),
				"auth_method":  authMethod,
				"updated_at":   now,
			},
		})
		return true, nil, err
	}
}

// ErrDifferentOrg is returned when an email already exists in a different organization.
var ErrDifferentOrg = errors.New("email exists in a different organization")

// MemberEntry represents a member to upsert.
type MemberEntry struct {
	FullName   string
	Email      string
	AuthMethod string
}

// ItemError represents a per-item error during batch processing.
type ItemError struct {
	Email  string // The email that failed (normalized, or original if normalization failed)
	Row    int    // Original row number (1-indexed for user display)
	Reason string // Human-readable error reason
}

// UpsertBatchResult holds the result of a batch upsert operation with per-item tracking.
type UpsertBatchResult struct {
	Created int
	Updated int
	Skipped int

	// SkippedEmails lists emails skipped due to org conflicts (for backward compatibility)
	SkippedEmails []string

	// ItemErrors provides granular per-item error details
	ItemErrors []ItemError
}

// HasErrors returns true if any per-item errors occurred.
func (r UpsertBatchResult) HasErrors() bool {
	return len(r.ItemErrors) > 0
}

// UpsertMembersInOrgBatch creates or updates members inside the given orgID.
// This is a batch-optimized version of UpsertMembersInOrg.
//
// For each entry:
//   - If email not found: creates new member in the org
//   - If email found in same org: updates full_name and auth_method
//   - If email found in different org: skips (adds to SkippedEmails and ItemErrors)
//
// ItemErrors provides per-item error tracking for validation failures, duplicates,
// and org conflicts. The Row field is 1-indexed for user display.
func (s *Store) UpsertMembersInOrgBatch(
	ctx context.Context,
	orgID primitive.ObjectID,
	entries []MemberEntry,
) (UpsertBatchResult, error) {
	var result UpsertBatchResult
	if len(entries) == 0 {
		return result, nil
	}

	// Normalize entries and collect unique emails, tracking per-item issues
	type normalizedEntry struct {
		fullName   string
		email      string
		authMethod string
		row        int // original 1-indexed row number
	}
	normalized := make(map[string]normalizedEntry, len(entries))
	emails := make([]string, 0, len(entries))

	for i, e := range entries {
		row := i + 1 // 1-indexed for user display
		email := normalize.Email(e.Email)
		fullName := normalize.Name(e.FullName)
		authMethod := normalize.AuthMethod(e.AuthMethod)

		// Track validation errors
		if email == "" && fullName == "" {
			result.ItemErrors = append(result.ItemErrors, ItemError{
				Email:  e.Email,
				Row:    row,
				Reason: "missing email and name",
			})
			continue
		}
		if email == "" {
			result.ItemErrors = append(result.ItemErrors, ItemError{
				Email:  e.Email,
				Row:    row,
				Reason: "missing or invalid email",
			})
			continue
		}
		if fullName == "" {
			result.ItemErrors = append(result.ItemErrors, ItemError{
				Email:  email,
				Row:    row,
				Reason: "missing name",
			})
			continue
		}

		// Track duplicates within the batch
		if existing, seen := normalized[email]; seen {
			result.ItemErrors = append(result.ItemErrors, ItemError{
				Email:  email,
				Row:    row,
				Reason: "duplicate of row " + itoa(existing.row),
			})
			continue
		}

		normalized[email] = normalizedEntry{
			fullName:   fullName,
			email:      email,
			authMethod: authMethod,
			row:        row,
		}
		emails = append(emails, email)
	}

	if len(emails) == 0 {
		return result, nil
	}

	// Batch fetch all existing users by email
	cur, err := s.c.Find(ctx, bson.M{"email": bson.M{"$in": emails}})
	if err != nil {
		return result, err
	}
	defer cur.Close(ctx)

	type existingUser struct {
		ID    primitive.ObjectID  `bson:"_id"`
		Email string              `bson:"email"`
		OrgID *primitive.ObjectID `bson:"organization_id"`
	}
	existing := make(map[string]existingUser, len(emails))
	for cur.Next(ctx) {
		var u existingUser
		if err := cur.Decode(&u); err != nil {
			return result, err
		}
		existing[strings.ToLower(u.Email)] = u
	}
	if err := cur.Err(); err != nil {
		return result, err
	}

	now := time.Now()

	// Categorize entries: to_insert vs to_update vs skipped
	type insertDoc struct {
		doc bson.M
		row int
	}
	var toInsert []insertDoc
	var toUpdate []existingUser

	for _, email := range emails {
		entry := normalized[email]
		if ex, found := existing[email]; found {
			// User exists
			if ex.OrgID == nil || *ex.OrgID != orgID {
				// Different org - skip with per-item error
				result.Skipped++
				result.SkippedEmails = append(result.SkippedEmails, email)
				result.ItemErrors = append(result.ItemErrors, ItemError{
					Email:  email,
					Row:    entry.row,
					Reason: "email exists in a different organization",
				})
				continue
			}
			// Same org - update
			toUpdate = append(toUpdate, ex)
		} else {
			// New user - insert
			toInsert = append(toInsert, insertDoc{
				doc: bson.M{
					"full_name":       entry.fullName,
					"full_name_ci":    text.Fold(entry.fullName),
					"email":           entry.email,
					"role":            "member",
					"organization_id": orgID,
					"status":          status.Active,
					"auth_method":     entry.authMethod,
					"created_at":      now,
					"updated_at":      now,
				},
				row: entry.row,
			})
		}
	}

	// Batch insert new users
	if len(toInsert) > 0 {
		// Convert to []interface{} for InsertMany
		docs := make([]interface{}, len(toInsert))
		for i, d := range toInsert {
			docs[i] = d.doc
		}

		opts := options.InsertMany().SetOrdered(false)
		_, err := s.c.InsertMany(ctx, docs, opts)
		if err != nil {
			// Handle partial success with duplicate key errors (race conditions)
			var bulkErr mongo.BulkWriteException
			if errors.As(err, &bulkErr) {
				// Collect emails that hit duplicate key errors for retry as updates
				var raceConditionEmails []string
				for _, we := range bulkErr.WriteErrors {
					if we.Code == 11000 {
						// Extract email and row from the failed document using the index
						if we.Index >= 0 && we.Index < len(toInsert) {
							insertItem := toInsert[we.Index]
							if email, ok := insertItem.doc["email"].(string); ok {
								raceConditionEmails = append(raceConditionEmails, email)
							}
						}
					} else {
						// Non-duplicate error - record per-item error and continue
						if we.Index >= 0 && we.Index < len(toInsert) {
							insertItem := toInsert[we.Index]
							email, _ := insertItem.doc["email"].(string)
							result.ItemErrors = append(result.ItemErrors, ItemError{
								Email:  email,
								Row:    insertItem.row,
								Reason: "database error: " + we.Message,
							})
						}
					}
				}
				// Count successful inserts (total minus failures)
				result.Created = len(toInsert) - len(bulkErr.WriteErrors)

				// Actually update the records that hit race conditions
				if len(raceConditionEmails) > 0 {
					var raceModels []mongo.WriteModel
					for _, email := range raceConditionEmails {
						entry := normalized[email]
						raceModels = append(raceModels, mongo.NewUpdateOneModel().
							SetFilter(bson.M{"email": email, "organization_id": orgID}).
							SetUpdate(bson.M{
								"$set": bson.M{
									"full_name":    entry.fullName,
									"full_name_ci": text.Fold(entry.fullName),
									"auth_method":  entry.authMethod,
									"updated_at":   now,
								},
							}))
					}
					bulkOpts := options.BulkWrite().SetOrdered(false)
					res, bulkErr := s.c.BulkWrite(ctx, raceModels, bulkOpts)
					if bulkErr != nil {
						// Track per-item errors for race condition updates that failed
						var updateBulkErr mongo.BulkWriteException
						if errors.As(bulkErr, &updateBulkErr) {
							for _, we := range updateBulkErr.WriteErrors {
								if we.Index >= 0 && we.Index < len(raceConditionEmails) {
									email := raceConditionEmails[we.Index]
									entry := normalized[email]
									result.ItemErrors = append(result.ItemErrors, ItemError{
										Email:  email,
										Row:    entry.row,
										Reason: "update failed: " + we.Message,
									})
								}
							}
						} else {
							return result, bulkErr
						}
					}
					result.Updated += int(res.ModifiedCount)
					// Records that weren't modified were in a different org - count as skipped
					notModified := len(raceConditionEmails) - int(res.ModifiedCount)
					if notModified > 0 {
						result.Skipped += notModified
						// Track which ones were skipped due to org mismatch
						// We can't easily determine which ones, so we skip detailed tracking here
					}
				}
			} else {
				return result, err
			}
		} else {
			result.Created = len(toInsert)
		}
	}

	// Batch update existing users in the same org
	if len(toUpdate) > 0 {
		var models []mongo.WriteModel
		for _, ex := range toUpdate {
			entry := normalized[ex.Email]
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": ex.ID}).
				SetUpdate(bson.M{
					"$set": bson.M{
						"full_name":    entry.fullName,
						"full_name_ci": text.Fold(entry.fullName),
						"auth_method":  entry.authMethod,
						"updated_at":   now,
					},
				}))
		}
		opts := options.BulkWrite().SetOrdered(false)
		res, err := s.c.BulkWrite(ctx, models, opts)
		if err != nil {
			// Handle partial success - track per-item errors
			var bulkErr mongo.BulkWriteException
			if errors.As(err, &bulkErr) {
				for _, we := range bulkErr.WriteErrors {
					if we.Index >= 0 && we.Index < len(toUpdate) {
						ex := toUpdate[we.Index]
						entry := normalized[ex.Email]
						result.ItemErrors = append(result.ItemErrors, ItemError{
							Email:  ex.Email,
							Row:    entry.row,
							Reason: "update failed: " + we.Message,
						})
					}
				}
				// Count successful updates (total minus failures)
				result.Updated += len(toUpdate) - len(bulkErr.WriteErrors)
			} else {
				return result, err
			}
		} else {
			result.Updated += int(res.ModifiedCount + res.UpsertedCount)
		}
	}

	return result, nil
}
