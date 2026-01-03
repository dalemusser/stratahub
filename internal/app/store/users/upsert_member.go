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
	"golang.org/x/crypto/bcrypt"
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
	FullName     string
	LoginID      string  // required: effective login identity
	AuthMethod   string  // required: auth method (email, google, microsoft, password, clever, classlink, schoology)
	Email        *string // optional: email address (may be same as LoginID for email-based auth)
	AuthReturnID *string // optional: auth_return_id for clever/classlink/schoology
	TempPassword *string // optional: plain text temp password for password auth (will be hashed)
}

// ItemError represents a per-item error during batch processing.
type ItemError struct {
	LoginID string // The login ID that failed (normalized, or original if normalization failed)
	Row     int    // Original row number (1-indexed for user display)
	Reason  string // Human-readable error reason
}

// SkippedMember represents a member that was skipped due to org conflict.
type SkippedMember struct {
	LoginID string // The login ID that was skipped
	OrgName string // The name of the organization they belong to
}

// MemberSummary represents basic member info for result display.
type MemberSummary struct {
	FullName   string
	LoginID    string
	AuthMethod string
	Email      string
}

// UpsertBatchResult holds the result of a batch upsert operation with per-item tracking.
type UpsertBatchResult struct {
	Created int
	Updated int
	Skipped int

	// CreatedMembers lists members that were newly created
	CreatedMembers []MemberSummary

	// UpdatedMembers lists members that were updated
	UpdatedMembers []MemberSummary

	// SkippedMembers lists members skipped due to org conflicts, with their org info
	SkippedMembers []SkippedMember

	// ItemErrors provides granular per-item error details
	ItemErrors []ItemError
}

// HasErrors returns true if any per-item errors occurred.
func (r UpsertBatchResult) HasErrors() bool {
	return len(r.ItemErrors) > 0
}

// UpsertMembersInOrgBatch creates or updates members inside the given orgID.
// This is a batch-optimized version that uses login_id as the unique key.
//
// For each entry:
//   - If login_id not found: creates new member in the org
//   - If login_id found in same org: updates fields (skips silently, counts as Updated)
//   - If login_id found in different org: error (rejects entire batch)
//
// ItemErrors provides per-item error tracking for validation failures and duplicates.
// The Row field is 1-indexed for user display.
func (s *Store) UpsertMembersInOrgBatch(
	ctx context.Context,
	orgID primitive.ObjectID,
	entries []MemberEntry,
) (UpsertBatchResult, error) {
	var result UpsertBatchResult
	if len(entries) == 0 {
		return result, nil
	}

	// Normalize entries and collect unique login IDs, tracking per-item issues
	type normalizedEntry struct {
		fullName     string
		loginID      string
		authMethod   string
		email        *string
		authReturnID *string
		passwordHash *string // hashed password
		row          int     // original 1-indexed row number
	}
	normalized := make(map[string]normalizedEntry, len(entries))
	loginIDs := make([]string, 0, len(entries))

	for i, e := range entries {
		row := i + 1 // 1-indexed for user display
		loginID := strings.TrimSpace(strings.ToLower(e.LoginID))
		fullName := normalize.Name(e.FullName)
		authMethod := normalize.AuthMethod(e.AuthMethod)

		// Track validation errors
		if loginID == "" && fullName == "" {
			result.ItemErrors = append(result.ItemErrors, ItemError{
				LoginID: e.LoginID,
				Row:     row,
				Reason:  "missing login ID and name",
			})
			continue
		}
		if loginID == "" {
			result.ItemErrors = append(result.ItemErrors, ItemError{
				LoginID: e.LoginID,
				Row:     row,
				Reason:  "missing login ID",
			})
			continue
		}
		if fullName == "" {
			result.ItemErrors = append(result.ItemErrors, ItemError{
				LoginID: loginID,
				Row:     row,
				Reason:  "missing name",
			})
			continue
		}

		// Track duplicates within the batch
		if existing, seen := normalized[loginID]; seen {
			result.ItemErrors = append(result.ItemErrors, ItemError{
				LoginID: loginID,
				Row:     row,
				Reason:  "duplicate of row " + itoa(existing.row),
			})
			continue
		}

		// Normalize optional email
		var email *string
		if e.Email != nil {
			emailNorm := normalize.Email(*e.Email)
			if emailNorm != "" {
				email = &emailNorm
			}
		}

		// Hash password if provided
		var passwordHash *string
		if e.TempPassword != nil && *e.TempPassword != "" {
			hash, err := hashPassword(*e.TempPassword)
			if err != nil {
				result.ItemErrors = append(result.ItemErrors, ItemError{
					LoginID: loginID,
					Row:     row,
					Reason:  "failed to hash password",
				})
				continue
			}
			passwordHash = &hash
		}

		normalized[loginID] = normalizedEntry{
			fullName:     fullName,
			loginID:      loginID,
			authMethod:   authMethod,
			email:        email,
			authReturnID: e.AuthReturnID,
			passwordHash: passwordHash,
			row:          row,
		}
		loginIDs = append(loginIDs, loginID)
	}

	if len(loginIDs) == 0 {
		return result, nil
	}

	// Batch fetch all existing users by login_id
	cur, err := s.c.Find(ctx, bson.M{"login_id": bson.M{"$in": loginIDs}})
	if err != nil {
		return result, err
	}
	defer cur.Close(ctx)

	type existingUser struct {
		ID      primitive.ObjectID  `bson:"_id"`
		LoginID string              `bson:"login_id"`
		OrgID   *primitive.ObjectID `bson:"organization_id"`
	}
	existing := make(map[string]existingUser, len(loginIDs))
	for cur.Next(ctx) {
		var u existingUser
		if err := cur.Decode(&u); err != nil {
			return result, err
		}
		existing[strings.ToLower(u.LoginID)] = u
	}
	if err := cur.Err(); err != nil {
		return result, err
	}

	now := time.Now()

	// Categorize entries: to_insert vs to_update vs conflict
	type insertDoc struct {
		doc bson.M
		row int
	}
	type updateItem struct {
		user  existingUser
		entry normalizedEntry
	}
	type skippedItem struct {
		loginID string
		orgID   *primitive.ObjectID
		row     int
	}
	var toInsert []insertDoc
	var toUpdate []updateItem
	var skippedItems []skippedItem

	for _, loginID := range loginIDs {
		entry := normalized[loginID]
		if ex, found := existing[loginID]; found {
			// User exists
			if ex.OrgID == nil || *ex.OrgID != orgID {
				// Different org - track for later org name lookup
				result.Skipped++
				skippedItems = append(skippedItems, skippedItem{
					loginID: loginID,
					orgID:   ex.OrgID,
					row:     entry.row,
				})
				result.ItemErrors = append(result.ItemErrors, ItemError{
					LoginID: loginID,
					Row:     entry.row,
					Reason:  "login ID exists in a different organization",
				})
				continue
			}
			// Same org - update
			toUpdate = append(toUpdate, updateItem{user: ex, entry: entry})
		} else {
			// New user - insert
			doc := bson.M{
				"full_name":       entry.fullName,
				"full_name_ci":    text.Fold(entry.fullName),
				"login_id":        entry.loginID,
				"login_id_ci":     text.Fold(entry.loginID),
				"role":            "member",
				"organization_id": orgID,
				"status":          status.Active,
				"auth_method":     entry.authMethod,
				"created_at":      now,
				"updated_at":      now,
			}
			// Optional fields
			if entry.email != nil {
				doc["email"] = *entry.email
			}
			if entry.authReturnID != nil {
				doc["auth_return_id"] = *entry.authReturnID
			}
			if entry.passwordHash != nil {
				doc["password_hash"] = *entry.passwordHash
				doc["password_temp"] = true
			}

			toInsert = append(toInsert, insertDoc{
				doc: doc,
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
				// Track which indexes failed
				failedIndexes := make(map[int]bool)
				// Collect login IDs that hit duplicate key errors for retry as updates
				var raceConditionLoginIDs []string
				for _, we := range bulkErr.WriteErrors {
					failedIndexes[we.Index] = true
					if we.Code == 11000 {
						// Extract login_id from the failed document using the index
						if we.Index >= 0 && we.Index < len(toInsert) {
							insertItem := toInsert[we.Index]
							if loginID, ok := insertItem.doc["login_id"].(string); ok {
								raceConditionLoginIDs = append(raceConditionLoginIDs, loginID)
							}
						}
					} else {
						// Non-duplicate error - record per-item error and continue
						if we.Index >= 0 && we.Index < len(toInsert) {
							insertItem := toInsert[we.Index]
							loginID, _ := insertItem.doc["login_id"].(string)
							result.ItemErrors = append(result.ItemErrors, ItemError{
								LoginID: loginID,
								Row:     insertItem.row,
								Reason:  "database error: " + we.Message,
							})
						}
					}
				}
				// Count successful inserts and track created members
				result.Created = len(toInsert) - len(bulkErr.WriteErrors)
				for i, item := range toInsert {
					if !failedIndexes[i] {
						result.CreatedMembers = append(result.CreatedMembers, docToMemberSummary(item.doc))
					}
				}

				// Actually update the records that hit race conditions
				if len(raceConditionLoginIDs) > 0 {
					var raceModels []mongo.WriteModel
					for _, loginID := range raceConditionLoginIDs {
						entry := normalized[loginID]
						updateFields := bson.M{
							"full_name":    entry.fullName,
							"full_name_ci": text.Fold(entry.fullName),
							"auth_method":  entry.authMethod,
							"updated_at":   now,
						}
						if entry.email != nil {
							updateFields["email"] = *entry.email
						}
						if entry.authReturnID != nil {
							updateFields["auth_return_id"] = *entry.authReturnID
						}
						if entry.passwordHash != nil {
							updateFields["password_hash"] = *entry.passwordHash
							updateFields["password_temp"] = true
						}
						raceModels = append(raceModels, mongo.NewUpdateOneModel().
							SetFilter(bson.M{"login_id": loginID, "organization_id": orgID}).
							SetUpdate(bson.M{"$set": updateFields}))
					}
					bulkOpts := options.BulkWrite().SetOrdered(false)
					res, bulkErr := s.c.BulkWrite(ctx, raceModels, bulkOpts)
					if bulkErr != nil {
						// Track per-item errors for race condition updates that failed
						var updateBulkErr mongo.BulkWriteException
						if errors.As(bulkErr, &updateBulkErr) {
							raceFailedIndexes := make(map[int]bool)
							for _, we := range updateBulkErr.WriteErrors {
								raceFailedIndexes[we.Index] = true
								if we.Index >= 0 && we.Index < len(raceConditionLoginIDs) {
									loginID := raceConditionLoginIDs[we.Index]
									entry := normalized[loginID]
									result.ItemErrors = append(result.ItemErrors, ItemError{
										LoginID: loginID,
										Row:     entry.row,
										Reason:  "update failed: " + we.Message,
									})
								}
							}
							// Track successful race condition updates
							for i, loginID := range raceConditionLoginIDs {
								if !raceFailedIndexes[i] {
									entry := normalized[loginID]
									result.UpdatedMembers = append(result.UpdatedMembers,
										newMemberSummary(entry.fullName, entry.loginID, entry.authMethod, entry.email))
								}
							}
						} else {
							return result, bulkErr
						}
					} else {
						// All race condition updates succeeded - track them
						for _, loginID := range raceConditionLoginIDs {
							entry := normalized[loginID]
							result.UpdatedMembers = append(result.UpdatedMembers,
								newMemberSummary(entry.fullName, entry.loginID, entry.authMethod, entry.email))
						}
					}
					result.Updated += int(res.ModifiedCount)
					// Records that weren't modified were in a different org - count as skipped
					// We need to identify which ones weren't modified and add them to skippedItems
					if int(res.ModifiedCount) < len(raceConditionLoginIDs) {
						// Re-fetch to find which users are in a different org
						skipCur, skipErr := s.c.Find(ctx, bson.M{
							"login_id": bson.M{"$in": raceConditionLoginIDs},
							"$or": []bson.M{
								{"organization_id": bson.M{"$ne": orgID}},
								{"organization_id": nil},
							},
						})
						if skipErr == nil {
							defer skipCur.Close(ctx)
							for skipCur.Next(ctx) {
								var skipUser struct {
									LoginID string              `bson:"login_id"`
									OrgID   *primitive.ObjectID `bson:"organization_id"`
								}
								if skipCur.Decode(&skipUser) == nil {
									result.Skipped++
									skippedItems = append(skippedItems, skippedItem{
										loginID: skipUser.LoginID,
										orgID:   skipUser.OrgID,
									})
								}
							}
						}
					}
				}
			} else {
				return result, err
			}
		} else {
			// All inserts succeeded
			result.Created = len(toInsert)
			for _, item := range toInsert {
				result.CreatedMembers = append(result.CreatedMembers, docToMemberSummary(item.doc))
			}
		}
	}

	// Batch update existing users in the same org
	if len(toUpdate) > 0 {
		var models []mongo.WriteModel
		for _, item := range toUpdate {
			updateFields := bson.M{
				"full_name":    item.entry.fullName,
				"full_name_ci": text.Fold(item.entry.fullName),
				"auth_method":  item.entry.authMethod,
				"updated_at":   now,
			}
			if item.entry.email != nil {
				updateFields["email"] = *item.entry.email
			}
			if item.entry.authReturnID != nil {
				updateFields["auth_return_id"] = *item.entry.authReturnID
			}
			if item.entry.passwordHash != nil {
				updateFields["password_hash"] = *item.entry.passwordHash
				updateFields["password_temp"] = true
			}
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": item.user.ID}).
				SetUpdate(bson.M{"$set": updateFields}))
		}
		opts := options.BulkWrite().SetOrdered(false)
		res, err := s.c.BulkWrite(ctx, models, opts)
		if err != nil {
			// Handle partial success - track per-item errors
			var bulkErr mongo.BulkWriteException
			if errors.As(err, &bulkErr) {
				failedIndexes := make(map[int]bool)
				for _, we := range bulkErr.WriteErrors {
					failedIndexes[we.Index] = true
					if we.Index >= 0 && we.Index < len(toUpdate) {
						item := toUpdate[we.Index]
						result.ItemErrors = append(result.ItemErrors, ItemError{
							LoginID: item.user.LoginID,
							Row:     item.entry.row,
							Reason:  "update failed: " + we.Message,
						})
					}
				}
				// Count successful updates (total minus failures) and track them
				result.Updated += len(toUpdate) - len(bulkErr.WriteErrors)
				for i, item := range toUpdate {
					if !failedIndexes[i] {
						result.UpdatedMembers = append(result.UpdatedMembers,
							newMemberSummary(item.entry.fullName, item.entry.loginID, item.entry.authMethod, item.entry.email))
					}
				}
			} else {
				return result, err
			}
		} else {
			// All updates succeeded
			result.Updated += int(res.ModifiedCount + res.UpsertedCount)
			for _, item := range toUpdate {
				result.UpdatedMembers = append(result.UpdatedMembers,
					newMemberSummary(item.entry.fullName, item.entry.loginID, item.entry.authMethod, item.entry.email))
			}
		}
	}

	// Batch lookup org names for skipped items
	if len(skippedItems) > 0 {
		// Collect unique org IDs
		orgIDSet := make(map[primitive.ObjectID]bool)
		for _, si := range skippedItems {
			if si.orgID != nil {
				orgIDSet[*si.orgID] = true
			}
		}

		// Batch fetch org names
		orgNames := make(map[primitive.ObjectID]string)
		if len(orgIDSet) > 0 {
			orgIDs := make([]primitive.ObjectID, 0, len(orgIDSet))
			for oid := range orgIDSet {
				orgIDs = append(orgIDs, oid)
			}

			orgCur, orgErr := s.c.Database().Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIDs}})
			if orgErr == nil {
				defer orgCur.Close(ctx)
				for orgCur.Next(ctx) {
					var org struct {
						ID   primitive.ObjectID `bson:"_id"`
						Name string             `bson:"name"`
					}
					if orgCur.Decode(&org) == nil {
						orgNames[org.ID] = org.Name
					}
				}
			}
		}

		// Build SkippedMembers with org names
		for _, si := range skippedItems {
			orgName := "(no organization)"
			if si.orgID != nil {
				if name, ok := orgNames[*si.orgID]; ok {
					orgName = name
				} else {
					orgName = "(unknown organization)"
				}
			}
			result.SkippedMembers = append(result.SkippedMembers, SkippedMember{
				LoginID: si.loginID,
				OrgName: orgName,
			})
		}
	}

	return result, nil
}

// hashPassword hashes a password using bcrypt with a cost of 12.
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// docToMemberSummary extracts member summary info from an insert document.
func docToMemberSummary(doc bson.M) MemberSummary {
	summary := MemberSummary{}
	if v, ok := doc["full_name"].(string); ok {
		summary.FullName = v
	}
	if v, ok := doc["login_id"].(string); ok {
		summary.LoginID = v
	}
	if v, ok := doc["auth_method"].(string); ok {
		summary.AuthMethod = v
	}
	if v, ok := doc["email"].(string); ok {
		summary.Email = v
	}
	return summary
}

// newMemberSummary creates a member summary from the given values.
func newMemberSummary(fullName, loginID, authMethod string, email *string) MemberSummary {
	summary := MemberSummary{
		FullName:   fullName,
		LoginID:    loginID,
		AuthMethod: authMethod,
	}
	if email != nil {
		summary.Email = *email
	}
	return summary
}
