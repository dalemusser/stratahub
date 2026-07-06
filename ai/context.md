# StrataHub Development Context

This document provides all necessary context for an AI assistant to continue development on the StrataHub application.

## Project Overview

**StrataHub** is a modular Go web application for managing organizations, groups, users, resources, and materials across multiple workspaces. It supports role-based access control (Admin, Analyst, Coordinator, Leader, Member) and integrates with multiple authentication methods.

- **Repository**: `github.com/dalemusser/stratahub`
- **Framework**: `github.com/dalemusser/waffle` (v0.1.36) — custom Go web framework by Dale Musser
- **Database**: MongoDB (6.0+) with AWS DocumentDB compatibility
- **Go Version**: 1.24.1
- **Frontend**: HTMX, Tailwind CSS (standalone CLI, no npm)

## Project Structure

```
stratahub/
├── cmd/stratahub/              # Application entry point
├── internal/
│   ├── app/
│   │   ├── bootstrap/          # Startup initialization, routes, config
│   │   ├── features/           # 34 feature modules (handlers + templates)
│   │   ├── resources/          # Embedded assets and shared templates
│   │   ├── store/              # MongoDB data access layer (27 collections)
│   │   ├── system/             # Shared utilities (auth, validation, etc.)
│   │   ├── policy/             # Business logic and authorization
│   │   └── loginactions/       # Login-related workflows
│   └── domain/models/          # 25 domain entities
├── static/                     # Static files served at /static/*
├── tests/e2e/                  # Playwright browser tests (Python)
├── docs/                       # Project documentation and guides
├── csvsamples/                 # Sample CSV files for data import
├── scripts/                    # Utility scripts
└── ai/                         # AI context documents
```

## Key Directories and Files

### Bootstrap (`internal/app/bootstrap/`)

Handles application initialization and lifecycle:

| File | Purpose |
|------|---------|
| `appconfig.go` | Configuration struct with all env vars |
| `config.go` | Config loading from TOML, env vars (STRATAHUB_* prefix) |
| `db.go` | MongoDB client and database initialization |
| `dbdeps.go` | Dependency container (DB client, file storage, logger) |
| `routes.go` | Chi HTTP router setup and route mounting |
| `startup.go` | Application startup hooks (schema init, etc.) |
| `shutdown.go` | Graceful shutdown logic |
| `hooks.go` | Lifecycle hook definitions for waffle |

### Features (`internal/app/features/`)

34 feature modules organized as self-contained units. Each feature typically includes:
- `handler.go` — HTTP handlers
- `routes.go` — Chi router setup with middleware
- `types.go` — View models and form input types
- `templates/` — Go HTML templates

**Core Features:**

| Feature | Purpose |
|---------|---------|
| **dashboard** | Role-based dashboard views for each user type |
| **organizations** | Organization CRUD (admin only) |
| **groups** | Group management with member/resource assignment |
| **leaders** | Leader user management (admin + coordinator) |
| **members** | Member user management (admin + leader + coordinator) |
| **systemusers** | Admin and analyst user management |
| **resources** | Resource management (admin + coordinator) + member resource views |
| **materials** | Material management (admin + coordinator) + leader/member views |
| **reports** | Reporting features (members report, activity logs) |
| **settings** | Site settings (name, logo, footer) |
| **workspaces** | Multi-workspace management (admin feature) |

**Authentication & Navigation:**

| Feature | Purpose |
|---------|---------|
| **login** | Authentication entry point (OAuth2 + password auth) |
| **logout** | Session termination |
| **authgoogle** | Google OAuth2 provider integration |
| **userinfo** | User profile/preferences page |
| **profile** | User profile management |

**Content & Pages:**

| Feature | Purpose |
|---------|---------|
| **home** | Landing page |
| **about** | About page (configurable content) |
| **contact** | Contact/support page |
| **terms** | Terms of service page |
| **pages** | Dynamic content pages (CRUD) |

**Mission HydroSci Features:**

| Feature | Purpose |
|---------|---------|
| **mhsdashboard** | MHS-specific dashboard views |
| **mhsbuilds** | Build/activity tracking for MHS platform |
| **missionhydrosci** | MHS-specific configuration and management |
| **gameconfig** | Game/activity configuration (MHS) |
| **uploadcsv** | CSV import for MHS data |

**Utility & System:**

| Feature | Purpose |
|---------|---------|
| **health** | Health check endpoint (`/health`) |
| **errors** | Error page handlers and logging |
| **activity** | Activity logging and tracking |
| **announcements** | Announcement management |
| **auditlog** | Audit trail logging |
| **heartbeat** | Application heartbeat/monitoring |
| **status** | Status indicators |

### Store Layer (`internal/app/store/`)

MongoDB data access with consistent patterns. Each store package handles one collection:

| Collection | Package | Purpose |
|-----------|---------|---------|
| **users** | `users/` | User accounts (admin, analyst, coordinator, leader, member) |
| **workspaces** | `workspaces/` | Workspace isolation containers |
| **organizations** | `organizations/` | Organizations within workspaces |
| **groups** | `groups/` | Groups for member organization |
| **group_memberships** | `memberships/` | User-Group relationships |
| **resources** | `resources/` | Resources assigned to groups |
| **group_resource_assignments** | `resourceassign/` | Resource-Group assignments |
| **materials** | `materials/` | Materials (documents, media) |
| **material_assignments** | `materialassign/` | Material-Org/Leader assignments |
| **coordinator_assignments** | `coordinatorassign/` | Coordinator org assignments |
| **site_settings** | `settings/` | Site-wide settings (name, logo, footer) |
| **global_settings** | `globalsettings/` | Global system settings |
| **pages** | `pages/` | Dynamic content pages |
| **login_history** | `logins/` | Login audit trail |
| **sessions** | `sessions/` | Session management |
| **activity** | `activity/` | Activity logging |
| **announcements** | `announcement/` | Site announcements |
| **audit** | `audit/` | Audit trail |
| **email_verification** | `emailverify/` | Email verification tokens |
| **oauth_state** | `oauthstate/` | OAuth2 state tokens |
| **metrics** | `metrics/` | Application metrics |
| **log_data** | `logdata/` | Structured logging data |
| **mhs_builds** | `mhsbuilds/` | MHS build tracking |
| **mhs_collections** | `mhscollections/` | MHS collections data |
| **mhs_device_status** | `mhsdevicestatus/` | MHS device tracking |
| **mhs_user_progress** | `mhsuserprogress/` | MHS user progress tracking |
| **group_app_settings** | `groupapps/` | Group-level app settings |

**Store Pattern:**
```go
// Consistent structure across all stores
type Store struct {
    coll *mongo.Collection
}

func New(db *mongo.Database) *Store {
    return &Store{coll: db.Collection("collection_name")}
}

// Methods return typed structs, not raw maps
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (Model, error) {
    var item Model
    err := s.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&item)
    if err == mongo.ErrNoDocuments {
        return item, ErrNotFound
    }
    return item, err
}
```

### Domain Models (`internal/domain/models/`)

25 domain entities representing core business objects:

| Model | Description |
|-------|-------------|
| **user.go** | User entity (roles: admin, analyst, coordinator, leader, member) with auth fields (LoginID, AuthReturnID, Email) |
| **workspace.go** | Workspace container for multi-tenancy |
| **organization.go** | Organization within a workspace |
| **group.go** | Group for organizing members |
| **groupmembership.go** | User-Group relationship with visibility windows |
| **resource.go** | Resource entity (assigned to groups) with URLIdentityMode |
| **groupresourceassignment.go** | Resource-Group assignment with visibility windows |
| **material.go** | Material entity (documents, media, links) with file storage |
| **materialassignment.go** | Material assignment to organizations/leaders |
| **coordinatorassignment.go** | Coordinator assignment to organizations |
| **sitesettings.go** | Site-wide settings (name, logo, footer HTML) |
| **globalsettings.go** | Global system settings |
| **page.go** | Dynamic content page (about, contact, terms, etc.) |
| **resourcetypes.go** | Resource type constants and validators |
| **materialtypes.go** | Material type constants (document, video, link, etc.) |
| **resourceurlmodes.go** | URLIdentityMode for resource identification (none/hex/human/both/legacy) |
| **loginhistory.go** | Login audit entry |
| **app.go** | Application configuration model |
| **mhs_build.go** | MHS build tracking data |
| **mhs_collection.go** | MHS collection data |
| **mhs_device_status.go** | MHS device status tracking |
| **mhs_user_progress.go** | MHS user progress data |
| **groupappsetting.go** | Group-level application settings |
| **authmethods.go** | Authentication method constants |

### System Utilities (`internal/app/system/`)

Shared packages for common functionality. **Always use these before implementing custom logic:**

| Package | Purpose |
|---------|---------|
| **auth/** | Session management, authentication middleware, session cookies |
| **authz/** | Authorization helpers, role extraction from request context |
| **viewdata/** | Base view model, site settings loading for templates |
| **paging/** | Pagination utilities for list views |
| **search/** | Search query parsing and normalization |
| **indexes/** | MongoDB index creation and management on startup |
| **inputval/** | Input validation with struct tags (email, URL, ObjectID, etc.) |
| **formutil/** | Form parsing and field extraction utilities |
| **normalize/** | String normalization (lowercase, trim, case-insensitive folding) |
| **htmlsanitize/** | HTML sanitization for user-provided content |
| **timeouts/** | Standard timeout durations (Short, Medium, Long) |
| **txn/** | Transaction wrapper (MongoDB + DocumentDB fallback) |
| **status/** | Status constants (active, disabled) |

**Critical**: When implementing features, always check these utilities first. Duplicating functionality adds technical debt and inconsistency.

### Aggregation Query Packages (`internal/app/store/queries/`)

Complex cross-collection queries in dedicated packages:

| Package | Purpose | Example |
|---------|---------|---------|
| **memberresources/** | Member → group_memberships → group_resource_assignments → resources | List resources available to a member |
| **groupmembers/** | Group members with user data, leaders sorted first | List group members with user info |
| **leadermaterials/** | Leader → material_assignments → materials | List materials assigned to a leader |

**Pattern:**
```go
results, err := memberresources.ListForMember(ctx, db, memberID)
for _, res := range results {
    fmt.Println(res.Resource.Title, res.GroupName)
}
```

### Input Validation (`internal/app/system/inputval/`)

Struct-based validation using tags:

```go
type ItemInput struct {
    Name   string `validate:"required,max=200" label:"Name"`
    Email  string `validate:"required,email" label:"Email"`
    Type   string `validate:"required,resourcetype" label:"Type"`
    URL    string `validate:"omitempty,httpurl" label:"URL"`
    OrgID  string `validate:"required,objectid" label:"Organization"`
}

result := inputval.Validate(input)
if result.HasErrors() {
    return result.First()  // First error message
}
```

**Custom validators:**
- `authmethod` — internal, google, classlink, clever, microsoft
- `resourcetype` — valid resource types
- `httpurl` — valid HTTP/HTTPS URL
- `objectid` — valid MongoDB ObjectID
- `email` — valid email format

### Normalization (`internal/app/system/normalize/`)

Always normalize user input for consistency:

```go
email := normalize.Email(r.FormValue("email"))     // lowercase + trim
name := normalize.Name(r.FormValue("name"))        // trim only
role := normalize.Role(r.FormValue("role"))        // lowercase + trim
status := normalize.Status(r.FormValue("status"))  // lowercase + trim
```

For case-insensitive storage (searching/sorting), use waffle's `text.Fold()`:
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
if authz.IsCoordinator(r) { /* ... */ }

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

## User Roles

StrataHub supports **6 roles** with hierarchical access levels:

### Super Admin
Full system access with ability to create and manage workspaces (extremely limited access).

**Unique capabilities:**
- Create/manage workspaces
- Global system settings

**Scope:** Global

### Admin
Full system access within a workspace with ability to manage all entities.

**Menu Access:**
- Dashboard
- Members Report
- Organizations (CRUD all)
- Groups (CRUD all)
- Leaders (CRUD all)
- Members (CRUD all)
- System Users (manage admins, analysts)
- Coordinators (manage coordinator assignments)
- Resources (CRUD + assign to groups)
- Materials (CRUD + assign to orgs/leaders)
- Site Settings (name, logo, footer)
- Workspace Settings

**Scope:** Workspace-scoped — can see and manage everything within their workspace

### Analyst
Read-only access to reports and dashboards for data analysis.

**Menu Access:**
- Dashboard
- Members Report

**Scope:** Workspace read-only — can view reports across all organizations in workspace

### Coordinator
Mid-level access for managing specific organizations and resources/materials.

**Menu Access:**
- Dashboard (org-scoped)
- Organizations (assigned orgs only)
- Groups (assigned org scoped)
- Leaders (assigned org scoped)
- Members (assigned org scoped)
- Resources (manage assigned orgs)
- Materials (manage assigned orgs)
- My Materials (view assigned materials)

**Permissions:** Can have per-org assignments with optional "manage materials" and "manage resources" permissions

**Scope:** Organization-scoped — can only manage assigned organizations

### Leader
Manages members and groups within their organization.

**Menu Access:**
- Dashboard
- Groups (view/manage org-scoped)
- Members (view/manage org-scoped)
- My Materials (view materials assigned to them)

**Scope:** Organization-scoped — can only see/manage within their own organization

### Member
End user who accesses resources assigned to their groups.

**Menu Access:**
- Dashboard
- Resources (view resources assigned via group membership)

**Scope:** Personal — sees only resources assigned to groups they belong to

### Visitor (Not Logged In)
Unauthenticated user with access to public pages only.

**Menu Access:**
- About, Contact, Terms, Privacy
- Login

**Role Hierarchy:**
```
Super Admin > Admin > Analyst/Coordinator > Leader > Member > Visitor
```

## Waffle Framework

Waffle is Dale's custom Go web framework providing:

### Key Packages (`waffle/pantry/`)

| Package | Purpose |
|---------|---------|
| `storage/` | File storage abstraction (local, S3, GCS, Azure) |
| `templates/` | Template engine with hot-reload |
| `fileserver/` | Static file serving with compression |
| `session/` | Session management with signing/verification |
| `mongo/` | MongoDB utilities and helpers |
| `search/` | Search helpers |
| `pagination/` | Pagination utilities |
| `text/` | Text manipulation (folding, normalization) |

## Configuration

Environment variables use `STRATAHUB_` prefix:

| Variable | Description | Example |
|----------|-------------|---------|
| `STRATAHUB_MONGO_URI` | MongoDB connection string | `mongodb://localhost:27017` |
| `STRATAHUB_MONGO_DATABASE` | Database name | `stratahub` |
| `STRATAHUB_SESSION_KEY` | Session signing key (32+ chars) | Auto-generated in dev |
| `STRATAHUB_SESSION_NAME` | Session cookie name | `stratahub_session` |
| `STRATAHUB_SESSION_DOMAIN` | Cookie domain (for cross-domain) | `.example.com` |
| `STRATAHUB_STORAGE_TYPE` | "local" or "s3" | `local` |
| `STRATAHUB_STORAGE_LOCAL_PATH` | Local file storage path | `./uploads/` |
| `STRATAHUB_STORAGE_LOCAL_URL` | URL prefix for local files | `/files` |
| `STRATAHUB_STORAGE_S3_*` | S3/CloudFront settings | Various |
| `STRATAHUB_HTTP_PORT` | HTTP port | `8080` |
| `STRATAHUB_USE_HTTPS` | Enable HTTPS | `false` |

For development, use `make run` which loads environment from `.env` or sets sensible defaults.

## Session Middleware Chain

Authentication middleware is applied in order:

1. **`LoadSessionUser`** (global) — Injects user into context if authenticated
2. **`RequireSignedIn`** — Enforces user in context, redirects to `/login`
3. **`RequireRole(roles...)`** — Enforces specific role(s), redirects to `/forbidden`

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
- Support for multiple auth methods (Google OAuth2, password, external tokens)

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
    WorkspaceID string // Added for multi-workspace support
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

- **Dark Mode**: Toggle with localStorage persistence, class-based strategy
- **Collapsible Sidebar**: Remembers state in localStorage
- **Global Loading Spinner**: Shows during navigation/HTMX requests
- **Footer HTML**: Configurable in site settings (rich text with TipTap)
- **Responsive Design**: Mobile-first with Tailwind

### Menu Structure

Menu templates are role-based (`menu_admin`, `menu_analyst`, `menu_coordinator`, `menu_leader`, `menu_member`, `menu_visitor`) with shared components (`menu_common`, `menu_footer`).

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

**Important**: All templates must support both light and dark modes. Always include `dark:` variants:

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

- **HTMX** — Dynamic updates without full page reloads
- **TipTap** — Rich text editing for HTML content
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
appresources.LoadSharedTemplates()
r.Handle("/assets/*", appresources.AssetsHandler("/assets"))
```

**Template Registration:**
- Feature templates in `internal/app/features/{feature}/templates/`
- Shared templates in `internal/app/resources/templates/`
- All registered with waffle's template engine at startup
- Use `templates.Render(w, r, "template_name", data)` to render

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
    r.Use(sm.RequireSignedIn)
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

Authorization and business rules are separated from handlers.

**Pattern:**
- All authorization functions return `(bool, error)` to distinguish "not authorized" from "database error"
- Functions take `*http.Request` as first param for user context extraction
- Role hierarchy: Super Admin > Admin > Analyst/Coordinator > Leader > Member > Guest

```go
func CanManageMember(r *http.Request, db *mongo.Database, memberID primitive.ObjectID) (bool, error) {
    role, _, userID, ok := authz.UserCtx(r)
    if !ok {
        return false, nil
    }
    if role == "admin" {
        return true, nil
    }
    if role == "coordinator" {
        // Check if coordinator is assigned to member's org
        return checkCoordinatorAssignment(r.Context(), db, userID, memberID)
    }
    if role == "leader" {
        // Check if member is in leader's organization
        return checkSameOrg(r.Context(), db, userID, memberID)
    }
    return false, nil
}
```

### Error Handling Pattern

Use `ErrorLogger` from `internal/app/features/errors/`:

```go
h.ErrLog.LogServerError(w, r, "Failed to load items", err, "/items")
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

    renderWithError := func(msg string) {
        data := FormData{/* pre-fill form fields */}
        data.Error = msg
        templates.Render(w, r, "item_new", data)
    }

    input := ItemInput{
        Name: strings.TrimSpace(r.FormValue("name")),
    }

    if result := inputval.Validate(input); result.HasErrors() {
        renderWithError(result.First())
        return
    }

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

Use `txn.Run()` for multi-document operations:

```go
import "github.com/dalemusser/stratahub/internal/app/system/txn"

err := txn.Run(ctx, h.DB, h.Log, func(sessCtx mongo.SessionContext) error {
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

    if r.Header.Get("HX-Request") == "true" {
        target := r.Header.Get("HX-Target")
        if target == "items-table" {
            templates.RenderSnippet(w, "items_table", data)
            return
        }
    }

    templates.Render(w, r, "items_list", data)
}
```

## Testing

StrataHub has two types of tests: Go unit tests and Playwright browser tests.

### Go Unit Tests

Unit tests in `*_test.go` files alongside code:

```bash
go test ./...                           # All tests
go test ./internal/app/store/users/...  # Specific package
go test -v ./...                        # Verbose output
make test-safe                          # Sequential (avoid MongoDB contention)
make test-cover                         # With coverage report
```

**Requirements:**
- MongoDB must be running for store tests
- Use table-driven tests following Go conventions

### Browser Tests (Playwright + Python)

End-to-end tests in `tests/e2e/`:

```bash
make test-e2e-setup  # First time only
make run             # In one terminal
make test-e2e        # In another terminal
make test-e2e-headed # With visible browser
```

**Key files:**
| File | Description |
|------|-------------|
| `conftest.py` | Shared fixtures and helpers |
| `test_admin_journey.py` | Admin workflow |
| `test_leader_journey.py` | Leader workflow |
| `test_member_journey.py` | Member workflow |

## Database Compatibility

**Critical**: All database code must support both MongoDB and AWS DocumentDB.

DocumentDB is MongoDB-compatible but has limitations:
- Avoid unsupported aggregation stages
- Test on both systems
- Use only well-supported stages: `$match`, `$lookup`, `$project`, `$sort`, `$limit`, `$skip`, `$group`, `$unwind`
- Avoid `$facet`, `$graphLookup`, schema validation

## How to Run Locally

```bash
# Build
make build

# Run (with sensible defaults)
make run

# Run with custom MongoDB
STRATAHUB_MONGO_URI=mongodb://localhost:27017 \
STRATAHUB_MONGO_DATABASE=stratahub \
./bin/stratahub

# Watch CSS in development
make css-watch
```

## Related Repos

- **waffle** (`github.com/dalemusser/waffle`) — Custom Go web framework
- **waffle/pantry** — Reusable packages (storage, templates, sessions, etc.)
- **mhscurriculum** — Source curriculum docs for MHS
- **mhsgrading** — Grading system integrated with StrataHub

## Recent Work Completed

### Multi-Workspace & Coordinator Support
- Workspaces for multi-tenancy
- Coordinator role with org-scoped assignments
- Coordinator-specific permissions (manage materials/resources)

### De-identification & User ID Cutover
- Migration from login_id to user_id (ObjectID hex)
- Resource URL identification modes (none/hex/human/both/legacy)
- Members Report CSV with proper identity crosswalk

### Activity Logging & Audit Trails
- Activity logging for user actions
- Audit logging for admin operations
- Email verification for auth

### Materials Feature
- Domain models, stores, indexes
- Admin CRUD with file upload
- Assignment picker (orgs + leaders)
- Leader view with material list

### Site Settings
- Site name customization
- Logo upload/replacement
- Footer HTML with TipTap editor

### Dark Mode
- All templates updated with dark mode support
- Toggle in menu footer
- localStorage persistence

## Notes for Claude

1. **Always read CLAUDE.md first** — It contains project-specific instructions that override defaults.

2. **Check existing patterns** — Before implementing features, look at similar ones in `internal/app/features/`.

3. **Workspaces are now a thing** — Most queries should filter by workspace_id. Super admins have workspace_id=nil.

4. **Use shared utilities** — Don't duplicate validation, auth, or formatting logic. These exist in `internal/app/system/`.

5. **BaseVM must be complete** — All handlers rendering templates must include all BaseVM fields, including WorkspaceID.

6. **Dark mode is required** — Test both light and dark modes as you develop. The toggle is in menu footer.

7. **DocumentDB compatibility** — Always consider MongoDB DocumentDB limitations when writing queries.

8. **Test on both MongoDB and DocumentDB** — Aggregations may work on MongoDB but fail on DocumentDB.

9. **HTMX is standard** — Use HTMX for partial updates. Full page reloads should be rare.

10. **File replacement UX** — When replacing files (logo, material), show current file + upload field + remove checkbox.

11. **Cascade deletes** — Document and implement cascade deletes for referential integrity.

12. **User roles expanded** — We now have 6 roles: Super Admin, Admin, Analyst, Coordinator, Leader, Member. Coordinator is new.

## Makefile Commands

```bash
# Build & Run
make build          # Build to bin/stratahub
make run            # Run with go run

# Testing
make test           # All tests
make test-v         # Verbose
make test-cover     # With coverage (generates coverage.html)
make test-safe      # Sequential (avoids MongoDB contention)
make test-e2e-setup # Setup E2E environment
make test-e2e       # Run E2E tests
make test-e2e-headed # E2E with visible browser

# CSS
make css            # Build Tailwind CSS
make css-watch      # Watch for changes
make css-prod       # Minified for production

# Maintenance
make clean          # Remove build artifacts
make fmt            # Format code
make vet            # Run go vet
make tidy           # Tidy go.mod
make verify         # Verify dependencies
make help           # Show all targets
```

## Quick Reference

### Important Files to Read First

1. `internal/app/bootstrap/routes.go` — See all routes
2. `internal/app/bootstrap/appconfig.go` — Configuration
3. `internal/app/system/viewdata/viewdata.go` — View model pattern
4. `internal/app/resources/templates/layout.gohtml` — Base layout
5. `internal/app/resources/templates/menu.gohtml` — Navigation

### Adding a New Feature

1. Create directory: `internal/app/features/{feature}/`
2. Add models if needed: `internal/domain/models/{feature}.go`
3. Add store: `internal/app/store/{feature}/`
4. Add indexes in `internal/app/system/indexes/indexes.go`
5. Create handler, routes, types, templates
6. Mount routes in `bootstrap/routes.go`
7. Add navigation in `menu.gohtml`
8. Add role middleware (`RequireRole(...)`)
9. Add policy authorization functions
10. Write unit tests and browser tests

### Debugging Tips

- Logs use `zap.Logger`
- Template errors show in response
- Check MongoDB indexes: `db.collection.getIndexes()`
- HTMX requests show in browser Network tab with `HX-Request: true` header
- Session issues: Check session cookie in browser DevTools
- Dark mode toggle: Bottom of menu (press Shift-D or click toggle)
