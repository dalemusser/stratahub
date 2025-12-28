"""
Playwright E2E Test Configuration for StrataHub

This module provides fixtures and configuration for end-to-end testing
using Playwright with Python.
"""

import pytest
import re
from playwright.sync_api import Page, expect
from typing import Optional

# Base URL for the application
BASE_URL = "http://localhost:8080"

# Admin credentials (email only - no password required)
ADMIN_EMAIL = "sysadmin@adroit.games"


class SessionData:
    """Session-scoped data shared across all test modules."""
    def __init__(self):
        self.leader_email: Optional[str] = None
        self.member_email: Optional[str] = None
        self.analyst_email: Optional[str] = None


# Global session data instance
_session_data = SessionData()


@pytest.fixture(scope="session")
def session_data() -> SessionData:
    """Session-scoped data for sharing between test modules."""
    return _session_data


@pytest.fixture(scope="session")
def browser_type_launch_args(browser_type_launch_args):
    """Configure browser launch settings."""
    return {
        **browser_type_launch_args,
        "args": ["--window-position=0,0"],
    }


@pytest.fixture(scope="session")
def browser_context_args(browser_context_args):
    """Configure browser context settings."""
    return {
        **browser_context_args,
        "base_url": BASE_URL,
        "viewport": {"width": 1700, "height": 1000},  # Fits MacBook display (1800x1169)
    }


# Session-scoped fixtures - ONE browser window for ALL tests
@pytest.fixture(scope="session")
def session_context(browser, browser_context_args):
    """Single browser context that persists for the entire test session."""
    context = browser.new_context(**browser_context_args)
    yield context
    context.close()


@pytest.fixture(scope="session")
def shared_page(session_context) -> Page:
    """Single page/tab that persists for ALL tests across all files."""
    page = session_context.new_page()
    yield page
    page.close()


# Override the default 'page' fixture to use our shared page
@pytest.fixture(scope="session")
def page(shared_page: Page) -> Page:
    """Override default page fixture to use shared page."""
    return shared_page


@pytest.fixture(scope="module")
def admin_page(shared_page: Page, session_data: SessionData) -> Page:
    """
    Provides the shared page logged in as admin.
    Logs in once at the start of admin tests (if not already logged in).
    Also extracts leader/member emails for use by other test modules.
    """
    # Check if already on dashboard (logged in)
    current_url = shared_page.url
    if "/dashboard" not in current_url:
        login_as(shared_page, ADMIN_EMAIL)

    yield shared_page

    # Before logging out, extract leader and member emails for other tests
    _extract_emails_for_session(shared_page, session_data)

    # Logout at end of admin tests so next user can login
    logout(shared_page)


def _extract_emails_for_session(page: Page, session_data: SessionData) -> None:
    """Extract leader, member, and analyst emails and store in session data."""
    # Need to be logged in as admin to access these pages
    # Check if we're on login page (logged out)
    if "/login" in page.url:
        login_as(page, ADMIN_EMAIL)

    # Extract leader email
    if not session_data.leader_email:
        page.goto("/leaders")
        wait_for_htmx(page)
        rows = page.locator("table tbody tr")
        if rows.count() > 0:
            first_row = rows.first
            cells = first_row.locator("td")
            for i in range(cells.count()):
                cell_text = cells.nth(i).inner_text().strip()
                if "@" in cell_text:
                    session_data.leader_email = cell_text
                    break

    # Extract member email
    if not session_data.member_email:
        page.goto("/members")
        wait_for_htmx(page)
        rows = page.locator("table tbody tr")
        if rows.count() > 0:
            first_row = rows.first
            cells = first_row.locator("td")
            for i in range(cells.count()):
                cell_text = cells.nth(i).inner_text().strip()
                if "@" in cell_text:
                    session_data.member_email = cell_text
                    break

    # Extract analyst email from system-users
    if not session_data.analyst_email:
        page.goto("/system-users")
        wait_for_htmx(page)
        rows = page.locator("table tbody tr")
        for i in range(rows.count()):
            row = rows.nth(i)
            row_text = row.inner_text().lower()
            # Look for a row with "analyst" role
            if "analyst" in row_text:
                cells = row.locator("td")
                for j in range(cells.count()):
                    cell_text = cells.nth(j).inner_text().strip()
                    if "@" in cell_text:
                        session_data.analyst_email = cell_text
                        break
                if session_data.analyst_email:
                    break



def login_as(page: Page, email: str) -> None:
    """
    Log in to the application with the given email.

    Args:
        page: Playwright page object
        email: Email address to log in with
    """
    page.goto("/login")
    page.fill('input[name="email"]', email)
    page.click('button[type="submit"]')
    # Wait for redirect to dashboard
    page.wait_for_url(re.compile(r".*/dashboard.*"), timeout=10000)


def logout(page: Page) -> None:
    """Log out of the application."""
    page.goto("/logout")
    # May redirect to / or /login
    page.wait_for_load_state("networkidle")
    # Navigate to login to confirm logged out
    page.goto("/login")


class SharedTestData:
    """Container for test data extracted during test runs."""

    def __init__(self):
        self.org_id: Optional[str] = None
        self.org_name: Optional[str] = None
        self.leader_id: Optional[str] = None
        self.leader_email: Optional[str] = None
        self.member_id: Optional[str] = None
        self.member_email: Optional[str] = None
        self.group_id: Optional[str] = None
        self.group_name: Optional[str] = None
        self.resource_id: Optional[str] = None
        self.resource_title: Optional[str] = None


@pytest.fixture(scope="module")
def test_data() -> SharedTestData:
    """Shared test data container for a test module."""
    return SharedTestData()


def extract_id_from_url(url: str) -> Optional[str]:
    """
    Extract MongoDB ObjectID from a URL path.

    Args:
        url: URL containing an ObjectID

    Returns:
        The extracted ID or None
    """
    # Match MongoDB ObjectID pattern (24 hex characters)
    match = re.search(r'/([a-f0-9]{24})(?:/|$)', url)
    return match.group(1) if match else None


def wait_for_htmx(page: Page, timeout: int = 5000) -> None:
    """
    Wait for HTMX requests to complete.

    Args:
        page: Playwright page object
        timeout: Maximum time to wait in milliseconds
    """
    try:
        page.wait_for_function(
            "() => !document.body.classList.contains('htmx-request')",
            timeout=timeout
        )
    except:
        pass  # HTMX may not be active
    page.wait_for_timeout(300)


def close_modal(page: Page) -> None:
    """Close any open modal dialog."""
    # Try clicking the close button if visible
    close_btn = page.locator('[data-modal-close], .modal-close, button:has-text("Close")')
    if close_btn.count() > 0 and close_btn.first.is_visible():
        close_btn.first.click()
    else:
        # Press Escape as fallback
        page.keyboard.press("Escape")

    # Wait for modal to disappear
    page.wait_for_timeout(300)


def fill_form(page: Page, form_data: dict) -> None:
    """
    Fill a form with the provided data.

    Args:
        page: Playwright page object
        form_data: Dictionary of field names to values
    """
    for field_name, value in form_data.items():
        selector = f'[name="{field_name}"]'
        element = page.locator(selector)

        if element.count() == 0:
            continue

        tag_name = element.evaluate("el => el.tagName.toLowerCase()")

        if tag_name == "select":
            element.select_option(value)
        elif tag_name == "input":
            input_type = element.get_attribute("type") or "text"
            if input_type == "checkbox":
                if value:
                    element.check()
                else:
                    element.uncheck()
            else:
                element.fill(value)
        elif tag_name == "textarea":
            element.fill(value)


def submit_form(page: Page) -> None:
    """Submit the current form and wait for navigation or HTMX response."""
    # Try multiple submit button selectors
    submit_btn = page.locator('button[type="submit"]')
    if submit_btn.count() == 0:
        # Try button inside form
        submit_btn = page.locator('form button').last
    if submit_btn.count() == 0:
        # Try any visible button
        submit_btn = page.locator('button:visible').last

    submit_btn.click()
    # Wait for either navigation or HTMX to complete
    try:
        page.wait_for_load_state("networkidle", timeout=5000)
    except:
        wait_for_htmx(page)


def get_table_row_count(page: Page, table_selector: str = "table tbody") -> int:
    """
    Get the number of rows in a table.

    Args:
        page: Playwright page object
        table_selector: CSS selector for the table body

    Returns:
        Number of rows
    """
    return page.locator(f"{table_selector} tr").count()


def find_row_with_text(page: Page, text: str, table_selector: str = "table tbody") -> Optional[int]:
    """
    Find a table row containing specific text.

    Args:
        page: Playwright page object
        text: Text to search for
        table_selector: CSS selector for the table body

    Returns:
        Row index (0-based) or None if not found
    """
    rows = page.locator(f"{table_selector} tr")
    for i in range(rows.count()):
        if text in rows.nth(i).inner_text():
            return i
    return None


def search_and_find(page: Page, search_text: str, timeout: int = 5000) -> bool:
    """
    Use the search box to find an item, waiting for HTMX to update.

    Args:
        page: Playwright page object
        search_text: Text to search for
        timeout: Maximum time to wait

    Returns:
        True if item found after search, False otherwise
    """
    # Try to find a search input
    search_selectors = [
        'input[type="search"]',
        'input[name="q"]',
        'input[placeholder*="earch"]',
        'input[placeholder*="Search"]',
        'input[hx-get]',  # HTMX search input
    ]

    search_input = None
    for selector in search_selectors:
        elem = page.locator(selector)
        if elem.count() > 0 and elem.first.is_visible():
            search_input = elem.first
            break

    if search_input:
        search_input.fill(search_text)
        # Wait for HTMX debounce and response
        page.wait_for_timeout(500)
        wait_for_htmx(page)

    # Check if text appears in the page
    return search_text in page.content()


def wait_for_item_in_list(page: Page, item_text: str, list_url: str, max_attempts: int = 5) -> bool:
    """
    Navigate to list page and wait for item to appear.

    Args:
        page: Playwright page object
        item_text: Text to look for
        list_url: URL of the list page
        max_attempts: Number of refresh attempts

    Returns:
        True if item found, False otherwise
    """
    for attempt in range(max_attempts):
        page.goto(list_url)
        page.wait_for_load_state("networkidle")
        wait_for_htmx(page)

        # Check if visible on page
        if item_text in page.content():
            return True

        # Try clicking "Last" page link if exists
        last_link = page.locator('a:has-text("Last"), button:has-text("Last")')
        if last_link.count() > 0:
            last_link.first.click()
            wait_for_htmx(page)
            if item_text in page.content():
                return True

        # Wait and retry
        if attempt < max_attempts - 1:
            page.wait_for_timeout(500)

    return False


def click_row_action(page: Page, row_index: int, action: str, table_selector: str = "table tbody") -> None:
    """
    Click an action button in a specific table row.

    Args:
        page: Playwright page object
        row_index: Row index (0-based)
        action: Action name ("edit", "delete", "view")
        table_selector: CSS selector for the table body
    """
    row = page.locator(f"{table_selector} tr").nth(row_index)

    # Try different selectors for the action button
    selectors = [
        f'a:has-text("{action}")',
        f'button:has-text("{action}")',
        f'[data-action="{action}"]',
        f'.btn-{action}',
    ]

    for selector in selectors:
        btn = row.locator(selector)
        if btn.count() > 0:
            btn.first.click()
            return

    raise ValueError(f"Could not find {action} button in row {row_index}")


def extract_email_from_table(page: Page, row_index: int, column_index: int = 1) -> str:
    """
    Extract email from a table cell.

    Args:
        page: Playwright page object
        row_index: Row index (0-based)
        column_index: Column index (0-based)

    Returns:
        Email address from the cell
    """
    cell = page.locator(f"table tbody tr:nth-child({row_index + 1}) td:nth-child({column_index + 1})")
    text = cell.inner_text().strip()

    # Extract email if it's in a cell with other content
    email_match = re.search(r'[\w\.-]+@[\w\.-]+\.\w+', text)
    return email_match.group(0) if email_match else text
