package userstore

import (
	"context"
	"errors"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/status"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Store struct {
	c *mongo.Collection
}

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("users")}
}

// GetByID loads a user by ObjectID.
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetMemberByID loads a user by ObjectID, returning an error if the user
// does not exist or is not a member role.
func (s *Store) GetMemberByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{"_id": id, "role": "member"}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetLeaderByID loads a user by ObjectID, returning an error if the user
// does not exist or is not a leader role.
func (s *Store) GetLeaderByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{"_id": id, "role": "leader"}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByLoginID looks up a user by case/diacritic-insensitive login_id. Returns mongo.ErrNoDocuments if not found.
func (s *Store) GetByLoginID(ctx context.Context, loginID string) (*models.User, error) {
	var u models.User
	folded := text.Fold(loginID)
	if err := s.c.FindOne(ctx, bson.M{"login_id_ci": folded}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

var (
	// ErrDuplicateLoginID is returned when attempting to create a user with a login_id that already exists.
	ErrDuplicateLoginID = errors.New("a user with this login ID already exists")
	// ErrDuplicateEmail is kept for backwards compatibility but maps to ErrDuplicateLoginID
	ErrDuplicateEmail = ErrDuplicateLoginID
	errBadRole        = errors.New(`role must be "admin"|"analyst"|"coordinator"|"leader"|"member"`)
	errBadStatus      = errors.New(`status must be "active"|"disabled"`)
	errOrgNeeded      = errors.New("leader/member must have organization_id")
)

// Create inserts a new user after normalizing & validating fields.
// It does not write any group membership arrays.
func (s *Store) Create(ctx context.Context, u models.User) (models.User, error) {
	// Normalize core fields
	u.ID = primitive.NewObjectID()
	u.FullName = normalize.Name(u.FullName)
	u.FullNameCI = text.Fold(u.FullName)

	// Normalize login_id fields
	if u.LoginID != nil && *u.LoginID != "" {
		loginID := normalize.Email(*u.LoginID) // lowercase
		loginIDCI := text.Fold(loginID)        // folded for case/diacritic-insensitive matching
		u.LoginID = &loginID
		u.LoginIDCI = &loginIDCI
	}

	// Normalize email if provided
	if u.Email != nil && *u.Email != "" {
		email := normalize.Email(*u.Email) // lowercase
		u.Email = &email
	}

	if u.Status == "" {
		u.Status = status.Active
	}

	// Validate role
	switch u.Role {
	case "admin", "analyst", "coordinator", "leader", "member":
		// ok
	default:
		return models.User{}, errBadRole
	}

	// Validate status
	if !status.IsValid(u.Status) {
		return models.User{}, errBadStatus
	}

	// Leaders/members must be scoped to an org
	if (u.Role == "leader" || u.Role == "member") && u.OrganizationID == nil {
		return models.User{}, errOrgNeeded
	}

	// Timestamps
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now

	// Insert
	if _, err := s.c.InsertOne(ctx, u); err != nil {
		if wafflemongo.IsDup(err) {
			return models.User{}, ErrDuplicateEmail
		}
		return models.User{}, err
	}
	return u, nil
}

// MemberUpdate holds the fields that can be updated for a member.
type MemberUpdate struct {
	FullName       string
	LoginID        string
	Email          *string
	AuthReturnID   *string
	AuthMethod     string
	Status         string
	OrganizationID primitive.ObjectID
	PasswordHash   *string
	PasswordTemp   *bool
}

// UpdateMember updates a member's fields. Only updates users with role="member".
// Returns ErrDuplicateLoginID if the login_id already exists for another user.
func (s *Store) UpdateMember(ctx context.Context, id primitive.ObjectID, upd MemberUpdate) error {
	loginID := normalize.Email(upd.LoginID)
	loginIDCI := text.Fold(loginID)

	set := bson.M{
		"full_name":       upd.FullName,
		"full_name_ci":    text.Fold(upd.FullName),
		"login_id":        loginID,
		"login_id_ci":     loginIDCI,
		"auth_method":     upd.AuthMethod,
		"status":          upd.Status,
		"organization_id": upd.OrganizationID,
		"updated_at":      time.Now(),
	}

	// Optional email field
	if upd.Email != nil {
		set["email"] = *upd.Email
	}

	// Optional auth_return_id field
	if upd.AuthReturnID != nil {
		set["auth_return_id"] = *upd.AuthReturnID
	}

	// Password fields (for password auth method)
	if upd.PasswordHash != nil {
		set["password_hash"] = *upd.PasswordHash
	}
	if upd.PasswordTemp != nil {
		set["password_temp"] = *upd.PasswordTemp
	}

	_, err := s.c.UpdateOne(ctx, bson.M{"_id": id, "role": "member"}, bson.M{"$set": set})
	if err != nil {
		if wafflemongo.IsDup(err) {
			return ErrDuplicateLoginID
		}
		return err
	}
	return nil
}

// DeleteMember deletes a user by ID, but only if they have role="member".
// Returns the number of documents deleted (0 or 1).
func (s *Store) DeleteMember(ctx context.Context, id primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteOne(ctx, bson.M{"_id": id, "role": "member"})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// LoginIDExistsForOther checks if a login_id already exists for a user other than the given ID.
func (s *Store) LoginIDExistsForOther(ctx context.Context, loginID string, excludeID primitive.ObjectID) (bool, error) {
	err := s.c.FindOne(ctx, bson.M{
		"login_id_ci": text.Fold(loginID),
		"_id":         bson.M{"$ne": excludeID},
	}).Err()
	if err == nil {
		return true, nil // found another user with this login_id
	}
	if err == mongo.ErrNoDocuments {
		return false, nil // no duplicate
	}
	return false, err // actual error
}

// DeleteByOrg deletes all users belonging to an organization.
// This only deletes users with organization_id set (leaders and members).
// Returns the number of documents deleted.
func (s *Store) DeleteByOrg(ctx context.Context, orgID primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteMany(ctx, bson.M{"organization_id": orgID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// CountByOrg returns the count of users in an organization.
func (s *Store) CountByOrg(ctx context.Context, orgID primitive.ObjectID) (int64, error) {
	return s.c.CountDocuments(ctx, bson.M{"organization_id": orgID})
}

// SystemUserUpdate holds the fields that can be updated for a system user (admin/analyst/coordinator).
type SystemUserUpdate struct {
	FullName   string
	LoginID    string
	AuthMethod string
	Role       string
	Status     string

	// Optional auth fields (pointer = optional)
	Email        *string
	AuthReturnID *string
	PasswordHash *string
	PasswordTemp *bool

	// Coordinator-specific permissions (only relevant when Role == "coordinator")
	CanManageMaterials bool
	CanManageResources bool
}

// UpdateSystemUser updates a system user's fields. Only updates users with role="admin", "analyst", or "coordinator".
// Returns ErrDuplicateLoginID if the login_id already exists for another user.
func (s *Store) UpdateSystemUser(ctx context.Context, id primitive.ObjectID, upd SystemUserUpdate) error {
	loginID := normalize.Email(upd.LoginID)
	loginIDCI := text.Fold(loginID)

	set := bson.M{
		"full_name":    upd.FullName,
		"full_name_ci": text.Fold(upd.FullName),
		"login_id":     loginID,
		"login_id_ci":  loginIDCI,
		"auth_method":  upd.AuthMethod,
		"role":         upd.Role,
		"status":       upd.Status,
		"updated_at":   time.Now(),
	}

	// Handle optional email
	if upd.Email != nil {
		set["email"] = *upd.Email
	}

	// Handle optional auth_return_id
	if upd.AuthReturnID != nil {
		set["auth_return_id"] = *upd.AuthReturnID
	}

	// Handle optional password reset
	if upd.PasswordHash != nil {
		set["password_hash"] = *upd.PasswordHash
		if upd.PasswordTemp != nil {
			set["password_temp"] = *upd.PasswordTemp
		}
	}

	update := bson.M{"$set": set}

	// For coordinators, set the permission fields; for others, unset them
	if upd.Role == "coordinator" {
		set["can_manage_materials"] = upd.CanManageMaterials
		set["can_manage_resources"] = upd.CanManageResources
	} else {
		// Clear coordinator-specific permissions when role is not coordinator
		update["$unset"] = bson.M{
			"can_manage_materials": "",
			"can_manage_resources": "",
		}
	}

	_, err := s.c.UpdateOne(ctx, bson.M{"_id": id, "role": bson.M{"$in": []string{"admin", "analyst", "coordinator"}}}, update)
	if err != nil {
		if wafflemongo.IsDup(err) {
			return ErrDuplicateLoginID
		}
		return err
	}
	return nil
}

// DeleteSystemUser deletes a user by ID, but only if they have role="admin", "analyst", or "coordinator".
// Returns the number of documents deleted (0 or 1).
func (s *Store) DeleteSystemUser(ctx context.Context, id primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteOne(ctx, bson.M{"_id": id, "role": bson.M{"$in": []string{"admin", "analyst", "coordinator"}}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// GetActiveMemberInOrg loads a user by ObjectID, returning an error if the user
// does not exist, is not a member, is not active, or does not belong to the given organization.
func (s *Store) GetActiveMemberInOrg(ctx context.Context, id, orgID primitive.ObjectID) (*models.User, error) {
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{
		"_id":             id,
		"role":            "member",
		"status":          "active",
		"organization_id": orgID,
	}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetLeaderInOrg loads a user by ObjectID, returning an error if the user
// does not exist, is not a leader, or does not belong to the given organization.
func (s *Store) GetLeaderInOrg(ctx context.Context, id, orgID primitive.ObjectID) (*models.User, error) {
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{
		"_id":             id,
		"role":            "leader",
		"organization_id": orgID,
	}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// CountActiveAdmins returns the number of users with role=admin and status=active.
func (s *Store) CountActiveAdmins(ctx context.Context) (int64, error) {
	return s.c.CountDocuments(ctx, bson.M{
		"role":   "admin",
		"status": "active",
	})
}

// Find returns users matching the given filter with optional find options.
// The caller is responsible for building the filter and options (pagination, sorting, projection).
func (s *Store) Find(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]models.User, error) {
	cur, err := s.c.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var users []models.User
	if err := cur.All(ctx, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// Count returns the number of users matching the given filter.
func (s *Store) Count(ctx context.Context, filter bson.M) (int64, error) {
	return s.c.CountDocuments(ctx, filter)
}

// UpdateThemePreference updates a user's theme preference.
// Valid values: "light", "dark", "system", or "" (empty = system default).
func (s *Store) UpdateThemePreference(ctx context.Context, id primitive.ObjectID, theme string) error {
	set := bson.M{
		"theme_preference": theme,
		"updated_at":       time.Now(),
	}
	_, err := s.c.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}

// UpdatePassword updates a user's password hash and clears the temporary flag.
// This is used when a user changes their own password (not a temp password reset).
func (s *Store) UpdatePassword(ctx context.Context, id primitive.ObjectID, passwordHash string) error {
	set := bson.M{
		"password_hash": passwordHash,
		"password_temp": false,
		"updated_at":    time.Now(),
	}
	_, err := s.c.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}
