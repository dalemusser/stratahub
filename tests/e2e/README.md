# Playwright E2E Tests for StrataHub

End-to-end tests using Python Playwright to verify complete user journeys through the StrataHub application.

## Prerequisites

- Python 3.8+
- StrataHub application running locally on `http://localhost:8080`
- MongoDB running (for the application)

## Setup

1. Create a virtual environment (recommended):
   ```bash
   cd tests/e2e
   python -m venv venv
   source venv/bin/activate  # On Windows: venv\Scripts\activate
   ```

2. Install dependencies:
   ```bash
   pip install -r requirements.txt
   ```

3. Install Playwright browsers:
   ```bash
   playwright install chromium
   ```

## Running Tests

### Start the Application First

Make sure StrataHub is running:
```bash
# From the project root
make run
```

### Run All E2E Tests

```bash
cd tests/e2e
pytest
```

### Run with Visible Browser

```bash
pytest --headed
```

### Run Specific Test File

```bash
pytest test_admin_journey.py      # Admin tests only
pytest test_leader_journey.py     # Leader tests only
pytest test_member_journey.py     # Member tests only
```

### Run Specific Test

```bash
pytest test_admin_journey.py::TestOrganizationManagement::test_create_organization
```

### Run with Verbose Output

```bash
pytest -v
```

### Generate HTML Report

```bash
pip install pytest-html
pytest --html=report.html
```

## Test Structure

### Test Files

| File | Description |
|------|-------------|
| `conftest.py` | Shared fixtures and utilities |
| `test_admin_journey.py` | Admin user journey tests |
| `test_leader_journey.py` | Leader user journey tests |
| `test_member_journey.py` | Member user journey tests |

### Test Order

Tests are designed to run in order:

1. **Admin Journey** (runs first)
   - Creates test organization
   - Creates test leader
   - Creates test member
   - Creates test group
   - Creates test resource

2. **Leader Journey** (extracts leader email from admin UI)
   - Logs in as the leader created above
   - Tests org-scoped member/group management
   - Verifies access restrictions

3. **Member Journey** (extracts member email from admin UI)
   - Logs in as the member created above
   - Tests resource viewing
   - Verifies access restrictions

## Configuration

### Base URL

Edit `conftest.py` to change the base URL:
```python
BASE_URL = "http://localhost:8080"
```

### Admin Credentials

Edit `conftest.py` to change admin credentials:
```python
ADMIN_EMAIL = "sysadmin@adroit.games"
```

## Key Fixtures

### `admin_page`
Provides a page already logged in as admin.

```python
def test_example(admin_page: Page):
    admin_page.goto("/organizations")
    # Already logged in
```

### `leader_credentials` / `member_credentials`
Automatically extracts leader/member emails from the admin interface.

```python
def test_example(page: Page, leader_credentials: dict):
    login_as(page, leader_credentials["email"])
    # Now logged in as leader
```

### `test_data`
Shared data container for passing IDs between tests.

```python
def test_create(admin_page: Page, test_data: TestData):
    # Create something...
    test_data.org_id = extracted_id

def test_use(admin_page: Page, test_data: TestData):
    # Use the ID from previous test
    admin_page.goto(f"/organizations/{test_data.org_id}")
```

## Utility Functions

| Function | Description |
|----------|-------------|
| `login_as(page, email)` | Log in with email |
| `logout(page)` | Log out |
| `fill_form(page, data)` | Fill form fields |
| `submit_form(page)` | Submit current form |
| `wait_for_htmx(page)` | Wait for HTMX requests |
| `close_modal(page)` | Close modal dialog |
| `find_row_with_text(page, text)` | Find table row |
| `click_row_action(page, row, action)` | Click row action button |

## Troubleshooting

### Tests fail with "No leaders/members available"
Run the admin journey tests first to create test data:
```bash
pytest test_admin_journey.py
```

### Application not responding
Ensure StrataHub is running on `http://localhost:8080`

### Browser not found
Install browsers:
```bash
playwright install
```

### Slow tests
Use headed mode for debugging:
```bash
pytest --headed --slowmo=500
```

### Debug a failing test
```bash
pytest test_admin_journey.py::TestOrganizationManagement::test_create_organization -v --headed
```

## Adding New Tests

1. Create a new test file or add to existing
2. Use fixtures from `conftest.py`
3. Follow existing patterns for consistency

Example:
```python
from conftest import login_as, fill_form, submit_form

def test_new_feature(admin_page: Page):
    admin_page.goto("/new-feature")

    fill_form(admin_page, {
        "field1": "value1",
        "field2": "value2",
    })

    submit_form(admin_page)

    # Assert result
    expect(admin_page.locator("body")).to_contain_text("Success")
```
