# StrataHub Development Context

This document provides all necessary context for an AI assistant to continue development on the StrataHub and Waffle projects.

## Project Overview

**StrataHub** is a web application for managing organizations, groups, leaders, and members. It's built using the **Waffle** framework, a custom Go web framework developed by Dale Musser.

- **Repository**: `github.com/dalemusser/stratahub`
- **Framework**: `github.com/dalemusser/waffle` (v0.1.24)
- **Database**: MongoDB
- **Go Version**: 1.24.1

## Project Structure

```
stratahub/
├── cmd/stratahub/         # Main entry point
├── internal/
│   ├── app/
│   │   ├── bootstrap/     # Application initialization (routes, config, deps)
│   │   ├── features/      # Feature modules (handlers + templates)
│   │   ├── resources/     # Static assets and shared templates
│   │   ├── store/         # Data access layer (MongoDB stores)
│   │   ├── system/        # Shared utilities (auth, viewdata, paging, etc.)
│   │   └── policy/        # Business logic policies
│   └── domain/
│       └── models/        # Domain entities
├── static/                # Static files served at /static/*
└── ai/                    # AI context documents
```

## Key Directories and Files

### Bootstrap (`internal/app/bootstrap/`)

| File | Purpose |
|------|---------|
| `appconfig.go` | Application configuration struct |
| `config.go` | Config loading from env vars (STRATAHUB_* prefix) |
| `db.go` | Database and storage initialization |
| `dbdeps.go` | Dependency container (MongoDB client, FileStorage) |
| `routes.go` | HTTP router setup, route mounting |
| `startup.go` | Application startup hooks |
| `shutdown.go` | Graceful shutdown logic |
| `hooks.go` | Lifecycle hook definitions |

### Features (`internal/app/features/`)

Each feature is a self-contained module with:
- `handler.go` - HTTP handlers
- `routes.go` - Chi router setup
- `types.go` - View models and form types
- `templates/` - Go HTML templates

| Feature | Purpose |
|---------|---------|
| dashboard | Role-based dashboard views |
| organizations | Organization CRUD |
| groups | Group management + member/resource assignment |
| leaders | Leader user management |
| members | Member user management |
| systemusers | Admin user management |
| resources | Resources (admin) + member resource views |
| materials | Materials (admin) + leader material views |
| reports | Reporting (members report) |
| settings | Site settings (name, logo, footer) |
| login | Authentication |
| logout | Session termination |
| pages | Dynamic content pages (about, contact, terms, privacy) |
| home | Landing page |
| health | Health check endpoint |
| errors | Error page handlers |

### Store Layer (`internal/app/store/`)

MongoDB data access with consistent patterns:

| Package | Collection |
|---------|------------|
| `users/` | users |
| `organizations/` | organizations |
| `groups/` | groups |
| `memberships/` | group_memberships |
| `resources/` | resources |
| `resourceassign/` | group_resource_assignments |
| `materials/` | materials |
| `materialassign/` | material_assignments |
| `settings/` | site_settings |
| `pages/` | pages |
| `logins/` | login_history |
| `metrics/` | metrics |

### Domain Models (`internal/domain/models/`)

| Model | Description |
|-------|-------------|
| `user.go` | User entity (admin, analyst, leader, member roles) |
| `organization.go` | Organization entity |
| `group.go` | Group entity |
| `groupmembership.go` | User-Group relationship |
| `resource.go` | Resource entity (assigned to groups) |
| `groupresourceassignment.go` | Resource-Group assignment |
| `material.go` | Material entity (assigned to orgs/leaders) |
| `materialassignment.go` | Material assignment |
| `sitesettings.go` | Site-wide settings |
| `page.go` | Dynamic content page |
| `resourcetypes.go` | Resource type constants |
| `materialtypes.go` | Material type constants |

### System Utilities (`internal/app/system/`)

| Package | Purpose |
|---------|---------|
| `auth/` | Session management, authentication middleware |
| `authz/` | Authorization helpers, role extraction |
| `viewdata/` | Base view model, site settings loading |
| `paging/` | Pagination utilities |
| `search/` | Search query parsing |
| `indexes/` | MongoDB index management |
| `inputval/` | Input validation |
| `formutil/` | Form parsing utilities |
| `normalize/` | String normalization (case-insensitive search) |
| `htmlsanitize/` | HTML sanitization |
| `timeouts/` | Standard timeout durations |
| `txn/` | Transaction wrapper with DocumentDB fallback |
| `status/` | Status constants (active, disabled) |

**Important**: When implementing new features, always check these shared utilities first. Do not duplicate functionality that already exists here. For example:
- Use `viewdata.LoadBase()` for view models, not custom site settings loading
- Use `paging` for pagination, not custom page calculation
- Use `normalize` for case-insensitive fields, not custom string manipulation
- Use `timeouts.Short()` / `timeouts.Long()` for context timeouts
- Use `htmlsanitize` for user-provided HTML content

### Aggregation Query Packages (`internal/app/store/queries/`)

Complex cross-collection queries are in dedicated packages:

| Package | Purpose |
|---------|---------|
| `memberresources/` | Member → group_memberships → group_resource_assignments → resources |
| `groupmembers/` | Group members with user data, leaders sorted first |
| `leadermaterials/` | Leader → material_assignments → materials |

**Pattern:**
```go
// Returns typed structs, not raw maps
results, err := memberresources.ListForMember(ctx, db, memberID)
for _, res := range results {
    fmt.Println(res.Resource.Title, res.GroupName)
}
```

### Input Validation (`internal/app/system/inputval/`)

Struct-based validation with tags:

```go
type ItemInput struct {
    Name   string `validate:"required,max=200" label:"Name"`
    Email  string `validate:"required,email" label:"Email"`
    Type   string `validate:"required,resourcetype" label:"Type"`
    URL    string `validate:"omitempty,httpurl" label:"URL"`
    OrgID  string `validate:"required,objectid" label:"Organization"`
}

// Validate and check
result := inputval.Validate(input)
if result.HasErrors() {
    return result.First()  // First error message
    // or result.All()     // All errors joined
}
```

**Custom validators:**
- `authmethod` - internal, google, classlink, clever, microsoft
- `resourcetype` - valid resource/material types
- `httpurl` - valid HTTP/HTTPS URL
- `objectid` - valid MongoDB ObjectID

### Normalization (`internal/app/system/normalize/`)

Always normalize user input:

```go
email := normalize.Email(r.FormValue("email"))     // lowercase + trim
name := normalize.Name(r.FormValue("name"))        // trim only
role := normalize.Role(r.FormValue("role"))        // lowercase + trim
status := normalize.Status(r.FormValue("status"))  // lowercase + trim
```

For case-insensitive storage, use waffle's `text.Fold()`:
```go
import "github.com/dalemusser/waffle/pantry/text"

user.FullName = name
user.FullNameCI = text.Fold(name)  // For searching/sorting
```

### User Context Extraction (`internal/app/system/authz/`)

Extract current user info from request context:

```go
import "github.com/dalemusser/stratahub/internal/app/system/authz"

// Get user details
role, name, userID, ok := authz.UserCtx(r)
if !ok {
    // Not authenticated
}

// Convenience predicates
if authz.IsAdmin(r) { /* ... */ }
if authz.IsLeader(r) { /* ... */ }

// Get user's organization ID
orgID := authz.UserOrgID(r)  // Returns primitive.NilObjectID if not set
```

### Status Constants (`internal/app/system/status/`)

Use constants for status values:

```go
import "github.com/dalemusser/stratahub/internal/app/system/status"

user.Status = status.Active    // "active"
user.Status = status.Disabled  // "disabled"

if status.IsValid(input.Status) { /* ... */ }
defaultStatus := status.Default()  // Returns "active"
```

## Waffle Framework

Waffle is Dale's custom Go web framework providing:

### Key Packages (`waffle/pantry/`)

| Package | Purpose |
|---------|---------|
| `storage/` | File storage abstraction (local, S3, GCS, Azure) |
| `templates/` | Template engine with hot-reload |
| `fileserver/` | Static file serving with compression |
| `session/` | Session management |
| `mongo/` | MongoDB utilities |
| `search/` | Search helpers |
| `pagination/` | Pagination |

### Storage Interface

```go
type Store interface {
    Put(ctx, path string, r io.Reader, opts *PutOptions) error
    Get(ctx, path string) (io.ReadCloser, error)
    Delete(ctx, path string) error
    Exists(ctx, path string) (bool, error)
    URL(path string) string
    PresignedURL(ctx, path string, opts *PresignOptions) (string, error)
    // ... more methods
}
```

## Configuration

Environment variables use `STRATAHUB_` prefix:

| Variable | Description |
|----------|-------------|
| `STRATAHUB_MONGO_URI` | MongoDB connection string |
| `STRATAHUB_MONGO_DATABASE` | Database name |
| `STRATAHUB_SESSION_KEY` | Session signing key (32+ chars) |
| `STRATAHUB_SESSION_NAME` | Session cookie name |
| `STRATAHUB_SESSION_DOMAIN` | Cookie domain |
| `STRATAHUB_STORAGE_TYPE` | "local" or "s3" |
| `STRATAHUB_STORAGE_LOCAL_PATH` | Local file storage path |
| `STRATAHUB_STORAGE_LOCAL_URL` | URL prefix for local files |
| `STRATAHUB_STORAGE_S3_*` | S3/CloudFront settings |

## User Roles

StrataHub has five roles with hierarchical access levels:

### Admin
Full system access with ability to manage all entities across all organizations.

**Menu Access:**
- Dashboard
- Members Report
- Organizations (CRUD all)
- Groups (CRUD all)
- Leaders (CRUD all)
- Members (CRUD all)
- System Users (manage admins, analysts)
- Resources (CRUD + assign to groups)
- Materials (CRUD + assign to orgs/leaders)
- Site Settings (name, logo, footer)

**Scope:** Global - can see and manage everything

### Analyst
Read-only access to reports and dashboards for data analysis.

**Menu Access:**
- Dashboard
- Members Report

**Scope:** Global read-only - can view reports across all organizations

### Leader
Manages members and groups within their organization.

**Menu Access:**
- Dashboard
- Groups (view/manage org-scoped)
- Members (view/manage org-scoped)
- My Materials (view materials assigned to them)

**Scope:** Organization-scoped - can only see/manage within their own organization

### Member
End user who accesses resources assigned to their groups.

**Menu Access:**
- Dashboard
- Resources (view resources assigned via group membership)

**Scope:** Personal - sees only resources assigned to groups they belong to

### Visitor (Not Logged In)
Unauthenticated user with access to public pages only.

**Menu Access:**
- About, Contact, Terms, Privacy
- Login

**Role Hierarchy:**
```
Admin > Analyst > Leader > Member > Visitor
```

**Authorization Notes:**
- Leaders can only manage members/groups in their organization
- Members access resources through group memberships with optional visibility windows
- Admins bypass all organization scoping
- Policy functions in `internal/app/policy/` handle fine-grained authorization

## Session Middleware Chain

Authentication middleware is applied in order:

1. **`LoadSessionUser`** (global) - Injects user into context if authenticated
2. **`RequireSignedIn`** - Enforces user in context, redirects to `/login`
3. **`RequireRole(roles...)`** - Enforces specific role(s), redirects to `/forbidden`

```go
// In routes.go
r.Use(sessionMgr.LoadSessionUser)  // Global - on all routes

// Feature routes
r.Route("/admin-feature", func(r chi.Router) {
    r.Use(sessionMgr.RequireSignedIn)
    r.Use(sessionMgr.RequireRole("admin"))
    // ... handlers
})
```

**Session Manager Features:**
- Fresh user fetch on every request (catches role changes, disabled accounts)
- Cookie-based with SameSite handling (Lax in dev, None in prod)
- Session error classification: expired, tampered, corrupted, backend failure

## View Model Pattern

All handlers use a BaseVM pattern for consistent template data:

```go
type BaseVM struct {
    SiteName   string
    LogoURL    string
    FooterHTML template.HTML
    IsLoggedIn bool
    Role       string
    UserName   string
}

// Usage in handlers:
base := viewdata.LoadBase(r, h.DB)
data := struct {
    viewdata.BaseVM
    // Feature-specific fields
    Items []Item
}{
    BaseVM: base,
    Items:  items,
}
templates.Render(w, r, "template_name", data)
```

## Template Structure

Templates use Go's html/template with layout inheritance:

```
internal/app/resources/templates/
├── layout.gohtml      # Base layout with sidebar, dark mode, loader
├── menu.gohtml        # Role-based navigation menus
└── ...

internal/app/features/{feature}/templates/
├── {feature}_list.gohtml
├── {feature}_new.gohtml
├── {feature}_edit.gohtml
├── {feature}_view.gohtml
└── ...
```

### Layout Features

- **Dark Mode**: Toggle with localStorage persistence
- **Collapsible Sidebar**: Remembers state in localStorage
- **Global Loading Spinner**: Shows during navigation/HTMX requests
- **Footer HTML**: Configurable in site settings

### Menu Structure

Menu templates are role-based (`menu_admin`, `menu_analyst`, `menu_leader`, `menu_member`, `menu_visitor`) with shared components (`menu_common`, `menu_footer`).

## Styling with Tailwind CSS

### Build Setup

Tailwind uses a **standalone CLI binary** (no npm/node required):

```bash
# Build CSS
make css

# Watch for changes during development
make css-watch

# Build minified for production
make css-prod
```

**Files:**
- Binary: `./tailwindcss` (standalone CLI in project root)
- Config: `./tailwind.config.js`
- Source: `./internal/app/resources/assets/css/src/input.css`
- Output: `./internal/app/resources/assets/css/tailwind.css`

### Content Scanning

Tailwind scans these paths for class names (in `tailwind.config.js`):

```javascript
content: [
  './internal/app/resources/templates/**/*.gohtml',
  './internal/app/features/**/templates/**/*.gohtml',
  './internal/app/**/*.go',
]
```

### Dark Mode

Dark mode uses **class-based strategy** (`.dark` class on `<html>` element):

```css
@custom-variant dark (&:where(.dark, .dark *));
```

The `input.css` includes base styles for:
- Default border colors (light and dark)
- Form elements in dark mode
- Hover state overrides for dark mode
- Selection highlights and badges

### Template Dark Mode Requirements

**Important**: All templates must support both light and dark modes. Always include `dark:` variants for colors, backgrounds, borders, and text:

```html
<div class="bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 border-gray-200 dark:border-gray-700">
  <h1 class="text-gray-800 dark:text-gray-200">Title</h1>
  <p class="text-gray-600 dark:text-gray-400">Description</p>
</div>
```

**Common dark mode pairs:**
| Light | Dark |
|-------|------|
| `bg-white` | `dark:bg-gray-800` |
| `bg-gray-50` | `dark:bg-gray-700` |
| `bg-gray-100` | `dark:bg-gray-700` |
| `text-gray-900` | `dark:text-gray-100` |
| `text-gray-700` | `dark:text-gray-300` |
| `text-gray-600` | `dark:text-gray-400` |
| `text-gray-500` | `dark:text-gray-400` |
| `border-gray-200` | `dark:border-gray-700` |
| `border-gray-300` | `dark:border-gray-600` |

### Rich Text Editor

- **TipTap** for rich text editing (footer HTML, instructions)
- Styles in `internal/app/resources/assets/css/tiptap.css`
- JavaScript in `internal/app/resources/assets/js/tiptap.min.js`

## Frontend Libraries

- **HTMX** for dynamic updates
- **TipTap** for rich text editing
- Bundled in `internal/app/resources/assets/js/`

## Embedded Assets & Templates

Assets and templates are embedded into the binary using Go's `embed` package.

**Location:** `internal/app/resources/`

```go
//go:embed templates/*.gohtml
var FS embed.FS  // Templates

//go:embed assets/*
var AssetsFS embed.FS  // CSS, JS, images
```

**Initialization (in bootstrap):**
```go
// Load shared templates
appresources.LoadSharedTemplates()

// Serve embedded assets at /assets/*
r.Handle("/assets/*", appresources.AssetsHandler("/assets"))
```

**Template Registration:**
- Feature templates in `internal/app/features/{feature}/templates/`
- Shared templates in `internal/app/resources/templates/`
- All registered with waffle's template engine at startup
- Use `templates.Render(w, r, "template_name", data)` to render

## Recent Work Completed

### Materials Feature (Complete)
- Domain models, stores, indexes
- Admin CRUD with file upload support
- Two-pane assignment picker (orgs + leaders)
- Leader view with material list
- File storage abstraction (local + S3/CloudFront)

### Site Settings Feature (Complete)
- Site name customization
- Logo upload/replacement
- Footer HTML with TipTap editor

### Dark Mode (Complete)
- All templates updated with dark mode support
- Toggle in menu footer
- localStorage persistence

### BaseVM Updates (Complete)
- All features updated to include SiteName, LogoURL, FooterHTML
- Consistent view model pattern across all features

## File Serving

Local storage files served from configured path:

```go
// In routes.go:
if appCfg.StorageType == "local" || appCfg.StorageType == "" {
    r.Handle(appCfg.StorageLocalURL+"/*",
        fileserver.Handler(appCfg.StorageLocalURL, appCfg.StorageLocalPath))
}
```

Default: `/files/materials/*` serves from `./uploads/materials/`

## Common Patterns

### Handler Structure

```go
type Handler struct {
    DB      *mongo.Database
    Storage storage.Store  // if needed
    ErrLog  *errorsfeature.ErrorLogger
    Log     *zap.Logger
}

func NewHandler(db *mongo.Database, storage storage.Store,
    errLog *errorsfeature.ErrorLogger, logger *zap.Logger) *Handler {
    return &Handler{DB: db, Storage: storage, ErrLog: errLog, Log: logger}
}
```

### Route Mounting

```go
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
    r := chi.NewRouter()
    r.Use(sm.RequireAuth)
    r.Use(sm.RequireRole("admin"))

    r.Get("/", h.ServeList)
    r.Get("/new", h.ServeNew)
    r.Post("/", h.HandleCreate)
    r.Get("/{id}/edit", h.ServeEdit)
    r.Post("/{id}/edit", h.HandleEdit)
    r.Post("/{id}/delete", h.HandleDelete)

    return r
}
```

### Store Pattern

```go
type Store struct {
    coll *mongo.Collection
}

func New(db *mongo.Database) *Store {
    return &Store{coll: db.Collection("items")}
}

func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.Item, error) {
    var item models.Item
    err := s.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&item)
    if err == mongo.ErrNoDocuments {
        return item, ErrNotFound
    }
    return item, err
}
```

### Policy Layer (`internal/app/policy/`)

Authorization and business rules are separated from handlers. Each policy package handles a specific domain.

**Pattern:**
- All authorization functions return `(bool, error)` to distinguish "not authorized" from "database error"
- Functions take `*http.Request` as first param for user context extraction
- Role hierarchy: Admin > Analyst > Leader > Member > Guest
- Leaders are scoped to their organization; admins see all

```go
// Example: Check if user can manage a member
func CanManageMember(r *http.Request, db *mongo.Database, memberID primitive.ObjectID) (bool, error) {
    role, _, userID, ok := authz.UserCtx(r)
    if !ok {
        return false, nil
    }
    if role == "admin" {
        return true, nil
    }
    if role == "leader" {
        // Check if member is in leader's organization
        return checkSameOrg(r.Context(), db, userID, memberID)
    }
    return false, nil
}
```

### Error Handling Pattern

Use `ErrorLogger` from `internal/app/features/errors/` for consistent error handling:

```go
// In handler
h.ErrLog.LogServerError(w, r, "Failed to load items", err, "/items")

// For HTMX requests
h.ErrLog.HTMXServerError(w, r, "Failed to save")

// Available methods:
// - LogServerError, LogBadRequest, LogForbidden, LogNotFound
// - HTMXServerError, HTMXBadRequest, HTMXForbidden, HTMXNotFound
```

### Form Submission Pattern

```go
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
    if err := r.ParseForm(); err != nil {
        h.ErrLog.LogBadRequest(w, r, "Invalid form", err, "/items/new")
        return
    }

    ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
    defer cancel()

    // Define renderWithError for re-rendering form with error
    renderWithError := func(msg string) {
        data := FormData{/* pre-fill form fields */}
        data.Error = msg
        templates.Render(w, r, "item_new", data)
    }

    // Parse and validate input
    input := ItemInput{
        Name: strings.TrimSpace(r.FormValue("name")),
    }

    if result := inputval.Validate(input); result.HasErrors() {
        renderWithError(result.First())
        return
    }

    // Create via store
    store := itemstore.New(h.DB)
    if _, err := store.Create(ctx, item); err != nil {
        if err == itemstore.ErrDuplicate {
            renderWithError("An item with this name already exists")
            return
        }
        h.ErrLog.LogServerError(w, r, "Failed to create", err, "/items")
        return
    }

    http.Redirect(w, r, "/items", http.StatusSeeOther)
}
```

### Transaction Support

Use `txn.Run()` for multi-document operations. It gracefully falls back for DocumentDB/standalone MongoDB:

```go
import "github.com/dalemusser/stratahub/internal/app/system/txn"

err := txn.Run(ctx, h.DB, h.Log, func(sessCtx mongo.SessionContext) error {
    // All operations here are in a transaction (if supported)
    if err := store1.Delete(sessCtx, id1); err != nil {
        return err
    }
    if err := store2.Delete(sessCtx, id2); err != nil {
        return err
    }
    return nil
})
```

### HTMX Partial Rendering

Check for HTMX requests and render partials:

```go
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
    // ... fetch data ...

    // Check if HTMX request targeting specific element
    if r.Header.Get("HX-Request") == "true" {
        target := r.Header.Get("HX-Target")
        if target == "items-table" {
            templates.RenderSnippet(w, "items_table", data)
            return
        }
    }

    // Full page render
    templates.Render(w, r, "items_list", data)
}
```

## Testing

StrataHub has two types of tests: Go unit tests and Playwright browser tests.

### Go Unit Tests

Unit tests are located alongside the code in `*_test.go` files.

**Location examples:**
- `internal/app/store/users/userstore_test.go`
- `internal/app/system/auth/auth_test.go`
- `internal/app/features/groups/handler_test.go`

**Running unit tests:**
```bash
go test ./...                           # All tests
go test ./internal/app/store/users/...  # Specific package
go test -v ./...                        # Verbose output
```

**Requirements:**
- MongoDB must be running for store tests
- Use table-driven tests following Go conventions

### Browser Tests (Playwright + Python)

End-to-end tests using Python Playwright to verify complete user journeys.

**Location:** `tests/e2e/`

**Files:**
| File | Description |
|------|-------------|
| `conftest.py` | Shared fixtures and helper functions |
| `test_admin_journey.py` | Admin user complete workflow |
| `test_leader_journey.py` | Leader user workflow |
| `test_member_journey.py` | Member user workflow |
| `test_analyst_journey.py` | Analyst user workflow |
| `requirements.txt` | Python dependencies |
| `README.md` | Detailed test documentation |

**Setup:**
```bash
cd tests/e2e
python -m venv venv
source venv/bin/activate
pip install -r requirements.txt
playwright install chromium
```

**Running browser tests:**
```bash
# Start the app first
make run  # In another terminal

# Run tests
cd tests/e2e
source venv/bin/activate
pytest                              # All tests
pytest --headed                     # With visible browser
pytest test_admin_journey.py        # Specific file
pytest -v --headed --slowmo=500     # Debug mode
```

**Test Order:**
Tests run sequentially as user journeys:
1. **Admin Journey** - Creates orgs, leaders, members, groups, resources
2. **Analyst Journey** - Tests analyst-specific access
3. **Leader Journey** - Tests leader-scoped features
4. **Member Journey** - Tests member resource access

**Key Fixtures (from conftest.py):**
- `admin_page` - Page logged in as admin
- `shared_page` - Single page shared across all tests
- `session_data` - Data shared between test modules
- `test_data` - Data container for passing IDs between tests

**Helper Functions:**
| Function | Purpose |
|----------|---------|
| `login_as(page, email)` | Log in with email |
| `logout(page)` | Log out current user |
| `fill_form(page, data)` | Fill form fields from dict |
| `submit_form(page)` | Submit current form |
| `wait_for_htmx(page)` | Wait for HTMX requests to complete |
| `close_modal(page)` | Close modal dialog |
| `find_row_with_text(page, text)` | Find table row containing text |
| `click_row_action(page, row, action)` | Click action button in row |
| `search_and_find(page, text)` | Use search box and verify result |

**Adding New Browser Tests:**
```python
from playwright.sync_api import Page, expect
from conftest import login_as, fill_form, submit_form, wait_for_htmx

def test_new_feature(admin_page: Page):
    admin_page.goto("/new-feature")
    wait_for_htmx(admin_page)

    fill_form(admin_page, {
        "field1": "value1",
        "field2": "value2",
    })

    submit_form(admin_page)

    expect(admin_page.locator("body")).to_contain_text("Success")
```

### Testing Conventions

1. **New features should have both unit tests and browser tests**
2. **Browser tests follow the "journey" pattern** - test complete user workflows, not isolated actions
3. **Use existing fixtures and helpers** from conftest.py
4. **Wait for HTMX** after any action that triggers an HTMX request
5. **Test all user roles** that can access the feature

## Build and Run

```bash
# Build
go build -o stratahub ./cmd/stratahub

# Run (development)
STRATAHUB_MONGO_URI=mongodb://localhost:27017 \
STRATAHUB_MONGO_DATABASE=stratahub \
STRATAHUB_SESSION_KEY=dev-key-for-testing-only-32chars \
./stratahub

# Or with make
make run
```

## Makefile Commands

The Makefile provides all common development tasks:

### Build & Run
| Command | Description |
|---------|-------------|
| `make build` | Build the application to `bin/stratahub` |
| `make run` | Run the application with `go run` |

### Testing
| Command | Description |
|---------|-------------|
| `make test` | Run all tests |
| `make test-v` | Run all tests with verbose output |
| `make test-cover` | Run tests with coverage report (generates `coverage.html`) |
| `make test-store` | Run only store tests (requires MongoDB) |
| `make test-handlers` | Run handler tests (requires MongoDB) |
| `make test-auth` | Run auth middleware tests |
| `make test-safe` | Run all tests sequentially (avoids MongoDB contention) |
| `make test-fresh` | Run tests without cache |

### E2E Browser Tests
| Command | Description |
|---------|-------------|
| `make test-e2e-setup` | Set up E2E test environment (run once) |
| `make test-e2e` | Run Playwright E2E tests (requires app running) |
| `make test-e2e-headed` | Run E2E tests with visible browser |
| `make test-e2e-slow` | Run E2E tests with visible browser in slow motion |

### Tailwind CSS
| Command | Description |
|---------|-------------|
| `make css` | Build Tailwind CSS |
| `make css-watch` | Watch and rebuild Tailwind CSS on changes |
| `make css-prod` | Build minified Tailwind CSS for production |

### Maintenance
| Command | Description |
|---------|-------------|
| `make clean` | Remove build artifacts |
| `make tidy` | Tidy go.mod dependencies |
| `make verify` | Verify dependencies |
| `make fmt` | Format code with `go fmt` |
| `make vet` | Run `go vet` |
| `make help` | Show all available targets |

## Known Considerations

1. **Session Key**: Must be 32+ characters in production
2. **File Storage**: Local storage requires write permissions to storage path
3. **MongoDB**: Indexes are created on startup via EnsureSchema
4. **Templates**: In dev mode, templates hot-reload; in prod, they're cached

## Database Compatibility

**Important**: All database code must support both MongoDB and AWS DocumentDB.

DocumentDB is MongoDB-compatible but has limitations. When writing database queries and aggregations:

1. **Avoid unsupported features** - DocumentDB doesn't support all MongoDB features (e.g., some aggregation stages, certain index types)
2. **Test on both** - Code that works on MongoDB may fail on DocumentDB
3. **Use compatible aggregation stages** - Stick to well-supported stages like `$match`, `$lookup`, `$project`, `$sort`, `$limit`, `$skip`, `$group`, `$unwind`
4. **Avoid MongoDB-specific features** like `$facet` with complex nested pipelines, `$graphLookup`, or schema validation
5. **Check DocumentDB documentation** when using advanced features to ensure compatibility

## Lessons Learned

These are hard-won insights from development that will save time and prevent errors:

### 1. BaseVM Fields Are Required in All Handlers
Every handler that renders a template using the layout **must** include all BaseVM fields (SiteName, LogoURL, FooterHTML, IsLoggedIn, Role, UserName). If you add a new field to BaseVM, you must update every handler. Missing fields cause template errors like `can't evaluate field LogoURL`.

### 2. File Serving Requires Route Configuration
When adding file upload features, remember to add a route to serve the files. Without this, uploaded files return 404:
```go
// In routes.go - easy to forget!
if appCfg.StorageType == "local" || appCfg.StorageType == "" {
    r.Handle(appCfg.StorageLocalURL+"/*",
        fileserver.Handler(appCfg.StorageLocalURL, appCfg.StorageLocalPath))
}
```

### 3. Download Links Need Special Handling
Links with the `download` attribute don't navigate away from the page, so the global loading spinner will spin forever. The layout.gohtml excludes these with `!link.hasAttribute('download')`. Keep this in mind for any new loader/spinner logic.

### 4. Flexbox for Centering, Not Just text-center
When centering elements like logos and titles together, use flexbox:
```html
<div class="flex flex-col items-center">
  <img src="..." class="h-10 w-auto">
  <h1>Title</h1>
</div>
```
Using just `text-center` or `mx-auto` often doesn't align elements properly.

### 5. File Replacement UX
When a file (like a logo) already exists, always show both:
- The current file with view/download options
- An upload field to replace it
- A checkbox to remove it entirely

Don't require users to remove before replacing.

### 6. Case-Insensitive Search Fields
For searchable text fields, always store a normalized version (lowercase, diacritics stripped) in a `*_ci` field:
```go
Title   string `bson:"title"`
TitleCI string `bson:"title_ci"` // Use normalize.ForSearch()
```
Search and sort on the `_ci` field, display the original.

### 7. Test Dark Mode During Development
Don't wait until the end to add dark mode. Test both modes as you develop templates. The toggle is in the menu footer.

### 8. Check Existing Patterns Before Implementing
Before implementing something new, look at how similar features do it:
- New CRUD feature? Look at `groups` or `resources`
- New assignment feature? Look at `materials` assignment
- New two-pane picker? Look at `groups` member assignment

## Pending/Future Work

From the Materials implementation plan, cascade deletes are implemented but should be verified:
- Organization delete → deletes material assignments
- Leader delete → deletes material assignments
- Material delete → deletes file + all assignments

## Quick Reference

### Important Files to Read First

1. `internal/app/bootstrap/routes.go` - See all routes
2. `internal/app/bootstrap/appconfig.go` - Configuration structure
3. `internal/app/system/viewdata/viewdata.go` - View model pattern
4. `internal/app/resources/templates/layout.gohtml` - Base layout
5. `internal/app/resources/templates/menu.gohtml` - Navigation
6. `internal/domain/models/` - All domain entities

### Adding a New Feature

1. Create directory: `internal/app/features/{feature}/`
2. Add models if needed: `internal/domain/models/{feature}.go`
3. Add store: `internal/app/store/{feature}/`
4. Add indexes in `internal/app/system/indexes/indexes.go`
5. Create handler, routes, types, templates
6. Mount routes in `bootstrap/routes.go`
7. Add navigation links in `menu.gohtml`

### Debugging

- Logs use `zap.Logger`
- Template errors show in response
- Check MongoDB indexes with `db.collection.getIndexes()`
