# Testing Guide for StrataHub

This document explains how tests are implemented in StrataHub, a Go web application built with the Waffle framework. It's designed for developers who may be new to Go testing.

## Table of Contents

- [Go Testing Basics](#go-testing-basics)
- [Test Categories](#test-categories)
- [Running Tests](#running-tests)
- [Test Requirements](#test-requirements)
- [Test Utilities](#test-utilities)
- [Writing Store Tests](#writing-store-tests)
- [Writing Handler Tests](#writing-handler-tests)
- [Common Patterns](#common-patterns)
- [Troubleshooting](#troubleshooting)
- [End-to-End (E2E) Browser Tests](#end-to-end-e2e-browser-tests)

---

## Go Testing Basics

Go has built-in testing support. Test files:
- Must end with `_test.go`
- Must be in the same package (or `packagename_test` for black-box testing)
- Contain functions starting with `Test` that take `*testing.T`

```go
// Example: internal/app/store/users/store_test.go
package users_test

import "testing"

func TestSomething(t *testing.T) {
    // Test code here
    if 1+1 != 2 {
        t.Errorf("expected 2, got something else")
    }
}
```

### Key Testing Methods

| Method | Purpose |
|--------|---------|
| `t.Errorf(format, args...)` | Log error and continue |
| `t.Fatalf(format, args...)` | Log error and stop test |
| `t.Helper()` | Mark function as helper (better stack traces) |
| `t.Run(name, func)` | Run a subtest |
| `t.Parallel()` | Run test in parallel with others |

---

## Test Categories

StrataHub has five categories of tests:

### 1. Store Tests (`internal/app/store/*/`)
Test database operations (CRUD) against MongoDB.

**Files:** `*_test.go` in each store package
**Examples:** `users/store_test.go`, `groups/store_test.go`
**Run with:** `make test-store`

### 2. Handler Tests (`internal/app/features/*/`)
Test HTTP handlers - form submission, validation, redirects, database state changes.

**Files:** `handler_test.go` in each feature package
**Examples:** `organizations/handler_test.go`, `members/handler_test.go`
**Run with:** `make test-handlers`

### 3. Auth Middleware Tests (`internal/app/system/auth/`)
Test authentication and authorization middleware.

**Files:** `auth_test.go`
**Run with:** `make test-auth`

### 4. Utility Tests (`internal/app/system/*/`)
Test helper functions - validation, normalization, pagination, etc.

**Files:** Various `*_test.go` files
**Examples:** `inputval/inputval_test.go`, `normalize/normalize_test.go`
**Run with:** `make test` (included in all tests)

### 5. E2E Browser Tests (`tests/e2e/`)
Test complete user journeys in a real browser using Playwright.

**Files:** `test_*_journey.py`
**Examples:** `test_admin_journey.py`, `test_analyst_journey.py`, `test_leader_journey.py`, `test_member_journey.py`
**Run with:** `make test-e2e` or `make test-e2e-headed`

---

## Running Tests

### Make Targets

```bash
# Go unit/integration tests
make test           # Run all Go tests
make test-v         # Run with verbose output (see each test name)
make test-store     # Run only store tests
make test-handlers  # Run only handler tests
make test-auth      # Run only auth tests
make test-safe      # Run sequentially (avoids MongoDB issues)
make test-fresh     # Run without cache
make test-cover     # Generate coverage report

# E2E browser tests (requires app running on localhost:8080)
make test-e2e        # Run headless (fast, for CI)
make test-e2e-headed # Run with visible browser
make test-e2e-slow   # Run with visible browser, 500ms delay (for demos)
```

### Direct Go Commands

```bash
go test ./...                           # All tests
go test ./internal/app/store/users/...  # Specific package
go test -v ./...                        # Verbose
go test -run TestHandleCreate ./...     # Run tests matching pattern
go test -count=1 ./...                  # Disable cache
```

---

## Test Requirements

### MongoDB
Store and handler tests require a running MongoDB instance.

**Default connection:** `mongodb://localhost:27017`

**Override with environment variable:**
```bash
export TEST_MONGO_URI="mongodb://localhost:27017"
```

### Test Database Isolation
Each test gets its own database (named after the test) to prevent interference:
```
strata_hub_test_TestStore_Create
strata_hub_test_TestHandleEdit_Success
```

Databases are automatically cleaned up after tests complete.

---

## Test Utilities

The `internal/testutil` package provides helpers for testing.

### SetupTestDB
Creates an isolated MongoDB database for a test:

```go
func TestSomething(t *testing.T) {
    db := testutil.SetupTestDB(t)  // Creates unique DB, auto-cleanup
    // Use db for test...
}
```

### TestContext
Creates a context with timeout for database operations:

```go
ctx, cancel := testutil.TestContext()
defer cancel()
```

### Fixtures
Helpers to create test data:

```go
fixtures := testutil.NewFixtures(t, db)

// Create test data
org := fixtures.CreateOrganization(ctx, "Test Org")
leader := fixtures.CreateLeader(ctx, "John", "john@example.com", org.ID)
member := fixtures.CreateMember(ctx, "Jane", "jane@example.com", org.ID)
group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
fixtures.CreateGroupMembership(ctx, member.ID, group.ID, org.ID, "member")
resource := fixtures.CreateResource(ctx, "Math Game", "https://example.com/game")
admin := fixtures.CreateAdmin(ctx, "Admin", "admin@example.com")
disabled := fixtures.CreateDisabledUser(ctx, "Disabled", "disabled@example.com")

// Access the database directly
db := fixtures.DB()
```

### WithChiURLParam
Sets URL parameters for chi router (needed for edit/delete handlers):

```go
req := httptest.NewRequest("POST", "/members/123/edit", body)
req = testutil.WithChiURLParam(req, "id", "123")
```

---

## Writing Store Tests

Store tests verify database operations work correctly.

### Basic Pattern

```go
package users_test

import (
    "testing"

    "github.com/dalemusser/stratahub/internal/app/store/users"
    "github.com/dalemusser/stratahub/internal/testutil"
)

func TestStore_Create(t *testing.T) {
    // 1. Setup test database
    db := testutil.SetupTestDB(t)
    ctx, cancel := testutil.TestContext()
    defer cancel()

    // 2. Create the store
    store := users.New(db)

    // 3. Perform operation
    user, err := store.Create(ctx, users.CreateInput{
        FullName: "Test User",
        Email:    "test@example.com",
        Role:     "member",
    })

    // 4. Assert results
    if err != nil {
        t.Fatalf("Create failed: %v", err)
    }
    if user.FullName != "Test User" {
        t.Errorf("FullName: got %q, want %q", user.FullName, "Test User")
    }
}
```

### Testing Error Cases

```go
func TestStore_Create_DuplicateEmail(t *testing.T) {
    db := testutil.SetupTestDB(t)
    ctx, cancel := testutil.TestContext()
    defer cancel()

    store := users.New(db)
    fixtures := testutil.NewFixtures(t, db)

    // Create first user
    fixtures.CreateMember(ctx, "First", "test@example.com", orgID)

    // Try to create duplicate
    _, err := store.Create(ctx, users.CreateInput{
        Email: "test@example.com",  // Same email
        // ...
    })

    // Expect error
    if err == nil {
        t.Error("expected error for duplicate email, got nil")
    }
}
```

---

## Writing Handler Tests

Handler tests verify HTTP endpoints work correctly.

### Basic Pattern

```go
package organizations_test

import (
    "net/http"
    "net/http/httptest"
    "net/url"
    "strings"
    "testing"

    "github.com/dalemusser/stratahub/internal/app/features/organizations"
    "github.com/dalemusser/stratahub/internal/app/system/auth"
    "github.com/dalemusser/stratahub/internal/testutil"
    "go.mongodb.org/mongo-driver/bson"
)

func TestHandleCreate_Success(t *testing.T) {
    // 1. Setup
    db := testutil.SetupTestDB(t)
    handler := organizations.NewHandler(db, errLog, logger)
    fixtures := testutil.NewFixtures(t, db)
    ctx, cancel := testutil.TestContext()
    defer cancel()

    // 2. Build form data
    form := url.Values{
        "name":     {"Test Org"},
        "city":     {"New York"},
        "timezone": {"America/New_York"},
    }

    // 3. Create request
    req := httptest.NewRequest("POST", "/organizations",
        strings.NewReader(form.Encode()))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    // 4. Add authenticated user to context
    req = auth.WithTestUser(req, &auth.SessionUser{
        ID:    "123",
        Name:  "Admin",
        Email: "admin@test.com",
        Role:  "admin",
    })

    // 5. Execute handler
    rec := httptest.NewRecorder()
    handler.HandleCreate(rec, req)

    // 6. Assert response
    if rec.Code != http.StatusSeeOther {
        t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
    }

    // 7. Verify database state
    count, _ := fixtures.DB().Collection("organizations").
        CountDocuments(ctx, bson.M{"name": "Test Org"})
    if count != 1 {
        t.Errorf("expected 1 organization, got %d", count)
    }
}
```

### Testing Edit/Delete (with URL parameters)

```go
func TestHandleEdit_Success(t *testing.T) {
    db := testutil.SetupTestDB(t)
    handler := organizations.NewHandler(db, errLog, logger)
    fixtures := testutil.NewFixtures(t, db)
    ctx, cancel := testutil.TestContext()
    defer cancel()

    // Create existing record
    org := fixtures.CreateOrganization(ctx, "Original Name")

    // Build update request
    form := url.Values{"name": {"Updated Name"}, "timezone": {"America/Chicago"}}
    req := httptest.NewRequest("POST", "/organizations/"+org.ID.Hex()+"/edit",
        strings.NewReader(form.Encode()))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    // Set URL parameter (required for chi.URLParam to work)
    req = testutil.WithChiURLParam(req, "id", org.ID.Hex())
    req = auth.WithTestUser(req, adminUser())

    rec := httptest.NewRecorder()
    handler.HandleEdit(rec, req)

    if rec.Code != http.StatusSeeOther {
        t.Errorf("expected redirect, got %d", rec.Code)
    }

    // Verify update
    var updated struct{ Name string `bson:"name"` }
    fixtures.DB().Collection("organizations").
        FindOne(ctx, bson.M{"_id": org.ID}).Decode(&updated)
    if updated.Name != "Updated Name" {
        t.Errorf("Name not updated: got %q", updated.Name)
    }
}

func TestHandleDelete_Success(t *testing.T) {
    db := testutil.SetupTestDB(t)
    handler := organizations.NewHandler(db, errLog, logger)
    fixtures := testutil.NewFixtures(t, db)
    ctx, cancel := testutil.TestContext()
    defer cancel()

    org := fixtures.CreateOrganization(ctx, "To Delete")

    req := httptest.NewRequest("POST", "/organizations/"+org.ID.Hex()+"/delete", nil)
    req = testutil.WithChiURLParam(req, "id", org.ID.Hex())
    req = auth.WithTestUser(req, adminUser())

    rec := httptest.NewRecorder()
    handler.HandleDelete(rec, req)

    // Verify deletion
    count, _ := fixtures.DB().Collection("organizations").
        CountDocuments(ctx, bson.M{"_id": org.ID})
    if count != 0 {
        t.Error("organization was not deleted")
    }
}
```

### Handling Template Panics

Handlers that render templates will panic in tests (templates aren't initialized). Use recover to catch panics and verify database state instead:

```go
func TestHandleCreate_ValidationError(t *testing.T) {
    // ... setup ...

    form := url.Values{
        "name": {""},  // Empty name - validation error
    }

    req := httptest.NewRequest("POST", "/organizations",
        strings.NewReader(form.Encode()))
    // ... set headers and user ...

    rec := httptest.NewRecorder()

    // Catch template panic
    func() {
        defer func() { recover() }()
        handler.HandleCreate(rec, req)
    }()

    // Verify no record was created (validation failed)
    count, _ := db.Collection("organizations").CountDocuments(ctx, bson.M{})
    if count != 0 {
        t.Error("record should not be created on validation error")
    }
}
```

---

## Common Patterns

### Table-Driven Tests

Test multiple cases in one function:

```go
func TestValidateEmail(t *testing.T) {
    tests := []struct {
        name    string
        email   string
        isValid bool
    }{
        {"valid email", "user@example.com", true},
        {"no domain", "user@", false},
        {"no at sign", "userexample.com", false},
        {"empty", "", false},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            result := IsValidEmail(tc.email)
            if result != tc.isValid {
                t.Errorf("IsValidEmail(%q) = %v, want %v",
                    tc.email, result, tc.isValid)
            }
        })
    }
}
```

### Helper Functions

Create reusable setup functions:

```go
func newTestHandler(t *testing.T) (*Handler, *testutil.Fixtures) {
    t.Helper()  // Marks this as helper for better error reporting
    db := testutil.SetupTestDB(t)
    logger := zap.NewNop()
    errLog := uierrors.NewErrorLogger(logger)
    handler := NewHandler(db, errLog, logger)
    fixtures := testutil.NewFixtures(t, db)
    return handler, fixtures
}

func adminUser() *auth.SessionUser {
    return &auth.SessionUser{
        ID:    "507f1f77bcf86cd799439011",
        Name:  "Test Admin",
        Email: "admin@test.com",
        Role:  "admin",
    }
}
```

### Testing Cascade Deletes

Verify related records are also deleted:

```go
func TestHandleDelete_CascadeDeletesRelatedData(t *testing.T) {
    // ... setup ...

    // Create org with related data
    org := fixtures.CreateOrganization(ctx, "Org")
    fixtures.CreateMember(ctx, "Member", "m@test.com", org.ID)
    fixtures.CreateGroup(ctx, "Group", org.ID)

    // Delete org
    // ...

    // Verify cascade
    userCount, _ := db.Collection("users").
        CountDocuments(ctx, bson.M{"organization_id": org.ID})
    if userCount != 0 {
        t.Error("users should be cascade deleted")
    }

    groupCount, _ := db.Collection("groups").
        CountDocuments(ctx, bson.M{"organization_id": org.ID})
    if groupCount != 0 {
        t.Error("groups should be cascade deleted")
    }
}
```

---

## Troubleshooting

### "signal: killed" Errors

Too many tests running in parallel can exhaust MongoDB connections.

**Solution:** Use `make test-safe` or `go test -p=1 ./...`

### "database name too long" Errors

MongoDB limits database names to 63 characters. Long test names can exceed this.

**Solution:** The `testutil.SetupTestDB` function automatically truncates names.

### Tests Pass Individually But Fail Together

Tests may be sharing state or exhausting resources.

**Solutions:**
- Use `make test-safe` for sequential execution
- Ensure each test uses `testutil.SetupTestDB(t)` for isolation
- Check for global state in your code

### Template Panics

Handlers that render templates will panic in tests.

**Solution:** Wrap handler calls in recover and verify database state:
```go
func() {
    defer func() { recover() }()
    handler.HandleCreate(rec, req)
}()
// Check database state instead of response body
```

### MongoDB Connection Errors

Ensure MongoDB is running on localhost:27017 or set `TEST_MONGO_URI`.

```bash
# Check if MongoDB is running
mongosh --eval "db.adminCommand('ping')"

# Or with Docker
docker run -d -p 27017:27017 mongo:latest
```

---

## End-to-End (E2E) Browser Tests

StrataHub includes comprehensive E2E tests using Playwright that simulate real user interactions in a browser. These tests verify complete user journeys across the application.

### What E2E Tests Cover

The E2E tests are organized into four user journeys:

| Test File | User Role | What's Tested |
|-----------|-----------|---------------|
| `test_admin_journey.py` | System Admin | Organizations, Leaders, Members, Groups, Resources CRUD; Search; Validation; Delete; Analyst creation |
| `test_analyst_journey.py` | Analyst | Dashboard with statistics; Members Report access/filtering; CSV download; Access restrictions |
| `test_leader_journey.py` | Leader | Member/Group management within org; Access restrictions |
| `test_member_journey.py` | Member | Resource viewing; Access restrictions |

**Total: 85 tests covering:**
- Login/logout for all user roles (admin, analyst, leader, member)
- CRUD operations (Create, Read, Update, Delete)
- Search and filtering
- Modal interactions (Manage → View/Edit/Delete)
- Form validation (invalid emails, duplicates)
- Authorization (users can't access unauthorized pages)
- Report generation and CSV download
- Navigation and UI elements

### Installation

E2E tests require Python 3.8+ and Playwright.

#### Quick Setup (Recommended)

From the project root directory:

```bash
make test-e2e-setup
```

This creates the Python virtual environment, installs dependencies, and downloads Chromium.

#### Manual Setup

If you prefer to set up manually:

```bash
# 1. Navigate to the E2E test directory
cd tests/e2e

# 2. Create a Python virtual environment
python3 -m venv venv

# 3. Activate the virtual environment
source venv/bin/activate

# 4. Install dependencies
pip install pytest pytest-playwright playwright

# 5. Install browser binaries
playwright install chromium
```

### Running E2E Tests

**Prerequisites:** The application must be running on `localhost:8080` with MongoDB available.

```bash
# Start the app in one terminal
make run

# Run tests in another terminal
```

#### Make Targets

```bash
make test-e2e        # Run headless (fast, for CI)
make test-e2e-headed # Run with visible browser
make test-e2e-slow   # Run with visible browser, 500ms delay (for demos)
```

#### Direct Commands

```bash
cd tests/e2e
source venv/bin/activate

pytest -v                              # Headless, verbose
pytest -v --headed                     # Visible browser
pytest -v --headed --slowmo=500        # Slow motion (500ms between actions)
pytest -v test_admin_journey.py        # Run specific file
pytest -v -k "test_delete"             # Run tests matching pattern
```

### Test Structure

```
tests/e2e/
├── conftest.py              # Fixtures and configuration
├── test_admin_journey.py    # Admin user tests (41 tests)
├── test_analyst_journey.py  # Analyst user tests (19 tests)
├── test_leader_journey.py   # Leader user tests (11 tests)
└── test_member_journey.py   # Member user tests (14 tests)
```

#### Key Fixtures (conftest.py)

| Fixture | Scope | Purpose |
|---------|-------|---------|
| `shared_page` | Session | Single browser page for all tests |
| `admin_page` | Module | Page logged in as admin |
| `analyst_page` | Module | Page logged in as analyst |
| `leader_page` | Module | Page logged in as leader |
| `member_page` | Module | Page logged in as member |
| `test_data` | Module | Shared data (IDs, emails) between tests |
| `session_data` | Session | Data shared across test files (leader, member, analyst emails) |

#### Helper Functions (conftest.py)

| Function | Purpose |
|----------|---------|
| `login_as(page, email)` | Log in with email |
| `logout(page)` | Log out |
| `fill_form(page, data)` | Fill form fields |
| `submit_form(page)` | Submit and wait |
| `wait_for_htmx(page)` | Wait for HTMX to complete |
| `find_row_with_text(page, text)` | Find table row containing text |
| `close_modal(page)` | Close modal dialog |

### Configuration

Browser settings are configured in `conftest.py`:

```python
# Viewport size (adjust for your display)
"viewport": {"width": 1700, "height": 1000}

# Window position
"args": ["--window-position=0,0"]
```

### Writing New E2E Tests

#### Basic Test Pattern

```python
def test_feature_works(self, admin_page: Page):
    """Test description."""
    # Navigate
    admin_page.goto("/some-page")
    wait_for_htmx(admin_page)

    # Interact
    admin_page.locator('button:has-text("Click Me")').click()

    # Assert
    expect(admin_page.locator("h1")).to_contain_text("Expected")
```

#### Testing HTMX Search

HTMX search requires triggering keyup events:

```python
search_input = admin_page.locator('#search-input')
search_input.fill("search term")
search_input.press("Space")
search_input.press("Backspace")
admin_page.wait_for_timeout(500)
wait_for_htmx(admin_page)
```

#### Testing Modal Interactions

```python
# Open modal
manage_btn = row.locator('button:has-text("Manage")')
manage_btn.click()
admin_page.wait_for_timeout(500)

# Click action in modal
edit_link = admin_page.locator('#modal-root a:has-text("Edit")')
edit_link.click()

# Close modal
close_modal(admin_page)
```

#### Testing Delete with Confirmation

```python
# Set up dialog handler BEFORE clicking
admin_page.on("dialog", lambda dialog: dialog.accept())

# Click delete button
delete_btn.click()
admin_page.wait_for_load_state("networkidle")
```

### Troubleshooting E2E Tests

#### Browser window too large/small

Edit viewport in `conftest.py`:
```python
"viewport": {"width": 1700, "height": 1000}
```

#### Tests fail due to timing

Add waits for HTMX:
```python
wait_for_htmx(admin_page)
admin_page.wait_for_timeout(500)
```

#### Element not found

Use more specific selectors:
```python
# Instead of:
admin_page.locator('button')

# Use:
admin_page.locator('button:has-text("Submit")')
admin_page.locator('#specific-id')
admin_page.locator('[data-testid="my-button"]')
```

#### Search not finding items (pagination)

Use HTMX search to find items:
```python
search_input = admin_page.locator('#search-q')
search_input.fill(item_name)
search_input.press("Space")
search_input.press("Backspace")
wait_for_htmx(admin_page)
```

---

## Summary

| What to Test | How to Test | Key Tools |
|--------------|-------------|-----------|
| Database CRUD | Store tests | `testutil.SetupTestDB`, `testutil.Fixtures` |
| HTTP handlers | Handler tests | `httptest`, `auth.WithTestUser`, `testutil.WithChiURLParam` |
| Middleware | Direct unit tests | `httptest.NewRequest`, `httptest.NewRecorder` |
| Utilities | Table-driven tests | `t.Run` for subtests |
| **User journeys** | **E2E tests** | **Playwright, pytest** |

### Quick Start Checklist

**Go Tests:**
1. Create `*_test.go` file in same package
2. Import `testing` and `testutil`
3. Use `testutil.SetupTestDB(t)` for database tests
4. Use `testutil.Fixtures` to create test data
5. For handlers: use `httptest` and `auth.WithTestUser`
6. Run with `make test` or `make test-safe`

**E2E Tests:**
1. Run `make test-e2e-setup` (once, to install dependencies)
2. Start the app with `make run`
3. Run with `make test-e2e`, `make test-e2e-headed`, or `make test-e2e-slow`
