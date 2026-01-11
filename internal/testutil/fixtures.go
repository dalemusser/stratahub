package testutil

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// WithChiURLParam adds a chi URL parameter to the request context.
// Use this in handler tests that need to access chi.URLParam values.
func WithChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// Fixtures provides helper methods for creating test data.
type Fixtures struct {
	db *mongo.Database
	t  *testing.T
}

// NewFixtures creates a new Fixtures instance for the given test database.
func NewFixtures(t *testing.T, db *mongo.Database) *Fixtures {
	t.Helper()
	return &Fixtures{db: db, t: t}
}

// DB returns the underlying database for direct access in tests.
func (f *Fixtures) DB() *mongo.Database {
	return f.db
}

// CreateWorkspace creates a test workspace with the given name and subdomain.
func (f *Fixtures) CreateWorkspace(ctx context.Context, name, subdomain string) models.Workspace {
	f.t.Helper()

	now := time.Now().UTC()
	ws := models.Workspace{
		ID:        primitive.NewObjectID(),
		Name:      name,
		NameCI:    text.Fold(name),
		Subdomain: subdomain,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := f.db.Collection("workspaces").InsertOne(ctx, ws)
	if err != nil {
		f.t.Fatalf("failed to create test workspace: %v", err)
	}

	return ws
}

// CreateOrganization creates a test organization with the given name.
// Returns the created organization with its generated ID.
func (f *Fixtures) CreateOrganization(ctx context.Context, name string) models.Organization {
	f.t.Helper()

	now := time.Now().UTC()
	org := models.Organization{
		ID:        primitive.NewObjectID(),
		Name:      name,
		NameCI:    text.Fold(name),
		City:      "Test City",
		CityCI:    text.Fold("Test City"),
		State:     "TS",
		StateCI:   text.Fold("TS"),
		TimeZone:  "America/New_York",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := f.db.Collection("organizations").InsertOne(ctx, org)
	if err != nil {
		f.t.Fatalf("failed to create test organization: %v", err)
	}

	return org
}

// CreateOrganizationInWorkspace creates a test organization in the given workspace.
func (f *Fixtures) CreateOrganizationInWorkspace(ctx context.Context, name string, workspaceID primitive.ObjectID) models.Organization {
	f.t.Helper()

	now := time.Now().UTC()
	org := models.Organization{
		ID:          primitive.NewObjectID(),
		WorkspaceID: workspaceID,
		Name:        name,
		NameCI:      text.Fold(name),
		City:        "Test City",
		CityCI:      text.Fold("Test City"),
		State:       "TS",
		StateCI:     text.Fold("TS"),
		TimeZone:    "America/New_York",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := f.db.Collection("organizations").InsertOne(ctx, org)
	if err != nil {
		f.t.Fatalf("failed to create test organization: %v", err)
	}

	return org
}

// CreateOrganizationWithDetails creates a test organization with full details.
func (f *Fixtures) CreateOrganizationWithDetails(ctx context.Context, name, city, state, tz string) models.Organization {
	f.t.Helper()

	now := time.Now().UTC()
	org := models.Organization{
		ID:        primitive.NewObjectID(),
		Name:      name,
		NameCI:    text.Fold(name),
		City:      city,
		CityCI:    text.Fold(city),
		State:     state,
		StateCI:   text.Fold(state),
		TimeZone:  tz,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := f.db.Collection("organizations").InsertOne(ctx, org)
	if err != nil {
		f.t.Fatalf("failed to create test organization: %v", err)
	}

	return org
}

// CreateUser creates a test user with the given parameters.
// For members and leaders, orgID must be provided.
func (f *Fixtures) CreateUser(ctx context.Context, fullName, email, role string, orgID *primitive.ObjectID) models.User {
	f.t.Helper()

	now := time.Now().UTC()
	loginIDCI := text.Fold(email)
	user := models.User{
		ID:             primitive.NewObjectID(),
		FullName:       fullName,
		FullNameCI:     text.Fold(fullName),
		LoginID:        &email,
		LoginIDCI:      &loginIDCI,
		AuthMethod:     "trust",
		Role:           role,
		Status:         "active",
		OrganizationID: orgID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err := f.db.Collection("users").InsertOne(ctx, user)
	if err != nil {
		f.t.Fatalf("failed to create test user: %v", err)
	}

	return user
}

// CreateUserInWorkspace creates a test user in the given workspace.
func (f *Fixtures) CreateUserInWorkspace(ctx context.Context, fullName, email, role string, workspaceID primitive.ObjectID, orgID *primitive.ObjectID) models.User {
	f.t.Helper()

	now := time.Now().UTC()
	loginIDCI := text.Fold(email)
	user := models.User{
		ID:             primitive.NewObjectID(),
		WorkspaceID:    &workspaceID,
		FullName:       fullName,
		FullNameCI:     text.Fold(fullName),
		LoginID:        &email,
		LoginIDCI:      &loginIDCI,
		AuthMethod:     "trust",
		Role:           role,
		Status:         "active",
		OrganizationID: orgID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err := f.db.Collection("users").InsertOne(ctx, user)
	if err != nil {
		f.t.Fatalf("failed to create test user: %v", err)
	}

	return user
}

// CreateSuperAdmin creates a test superadmin user (no workspace_id).
func (f *Fixtures) CreateSuperAdmin(ctx context.Context, fullName, email string) models.User {
	f.t.Helper()

	now := time.Now().UTC()
	loginIDCI := text.Fold(email)
	user := models.User{
		ID:          primitive.NewObjectID(),
		WorkspaceID: nil, // Superadmins have no workspace
		FullName:    fullName,
		FullNameCI:  text.Fold(fullName),
		LoginID:     &email,
		LoginIDCI:   &loginIDCI,
		AuthMethod:  "trust",
		Role:        "superadmin",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := f.db.Collection("users").InsertOne(ctx, user)
	if err != nil {
		f.t.Fatalf("failed to create test superadmin: %v", err)
	}

	return user
}

// CreateAdmin creates a test admin user.
func (f *Fixtures) CreateAdmin(ctx context.Context, fullName, email string) models.User {
	f.t.Helper()
	return f.CreateUser(ctx, fullName, email, "admin", nil)
}

// CreateAnalyst creates a test analyst user.
func (f *Fixtures) CreateAnalyst(ctx context.Context, fullName, email string) models.User {
	f.t.Helper()
	return f.CreateUser(ctx, fullName, email, "analyst", nil)
}

// CreateDisabledUser creates a test user with disabled status.
func (f *Fixtures) CreateDisabledUser(ctx context.Context, fullName, email string) models.User {
	f.t.Helper()

	now := time.Now().UTC()
	loginIDCI := text.Fold(email)
	user := models.User{
		ID:         primitive.NewObjectID(),
		FullName:   fullName,
		FullNameCI: text.Fold(fullName),
		LoginID:    &email,
		LoginIDCI:  &loginIDCI,
		AuthMethod: "trust",
		Role:       "member",
		Status:     "disabled",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	_, err := f.db.Collection("users").InsertOne(ctx, user)
	if err != nil {
		f.t.Fatalf("failed to create disabled test user: %v", err)
	}

	return user
}

// CreateLeader creates a test leader user in the given organization.
func (f *Fixtures) CreateLeader(ctx context.Context, fullName, email string, orgID primitive.ObjectID) models.User {
	f.t.Helper()
	return f.CreateUser(ctx, fullName, email, "leader", &orgID)
}

// CreateMember creates a test member user in the given organization.
func (f *Fixtures) CreateMember(ctx context.Context, fullName, email string, orgID primitive.ObjectID) models.User {
	f.t.Helper()
	return f.CreateUser(ctx, fullName, email, "member", &orgID)
}

// CreateGroup creates a test group in the given organization.
func (f *Fixtures) CreateGroup(ctx context.Context, name string, orgID primitive.ObjectID) models.Group {
	f.t.Helper()

	now := time.Now().UTC()
	group := models.Group{
		ID:             primitive.NewObjectID(),
		Name:           name,
		NameCI:         text.Fold(name),
		Description:    "Test group description",
		OrganizationID: orgID,
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err := f.db.Collection("groups").InsertOne(ctx, group)
	if err != nil {
		f.t.Fatalf("failed to create test group: %v", err)
	}

	return group
}

// CreateGroupMembership creates a membership record linking a user to a group.
func (f *Fixtures) CreateGroupMembership(ctx context.Context, userID, groupID, orgID primitive.ObjectID, role string) models.GroupMembership {
	f.t.Helper()

	now := time.Now().UTC()
	membership := models.GroupMembership{
		ID:        primitive.NewObjectID(),
		UserID:    userID,
		GroupID:   groupID,
		OrgID:     orgID,
		Role:      role,
		CreatedAt: now,
	}

	_, err := f.db.Collection("group_memberships").InsertOne(ctx, membership)
	if err != nil {
		f.t.Fatalf("failed to create test group membership: %v", err)
	}

	return membership
}

// CreateResource creates a test resource.
func (f *Fixtures) CreateResource(ctx context.Context, title, launchURL string) models.Resource {
	f.t.Helper()

	now := time.Now().UTC()
	resource := models.Resource{
		ID:            primitive.NewObjectID(),
		Title:         title,
		TitleCI:       text.Fold(title),
		LaunchURL:     launchURL,
		Status:        "active",
		Type:          "link",
		ShowInLibrary: true,
		CreatedAt:     now,
		UpdatedAt:     &now,
	}

	_, err := f.db.Collection("resources").InsertOne(ctx, resource)
	if err != nil {
		f.t.Fatalf("failed to create test resource: %v", err)
	}

	return resource
}
