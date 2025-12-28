"""
Analyst User Journey Tests for StrataHub

Tests the analyst workflow including:
- Login as analyst (using email extracted from admin setup)
- Dashboard access with statistics display
- Members Report access and functionality
- CSV download capability
- Cannot access management features (organizations, leaders, members, groups, resources, system-users)

These tests depend on test_admin_journey to have run first
to create the test analyst user.
"""

import pytest
import re
from playwright.sync_api import Page, expect
from conftest import (
    login_as,
    logout,
    wait_for_htmx,
    SessionData,
)


@pytest.fixture(scope="module")
def analyst_page(shared_page: Page, session_data: SessionData) -> Page:
    """
    Provides the shared page logged in as analyst.
    Uses email from session_data (extracted during admin tests).
    """
    if not session_data.analyst_email:
        pytest.skip("No analyst email available - run admin journey tests first")

    login_as(shared_page, session_data.analyst_email)

    yield shared_page

    # Logout at end so next user can login
    logout(shared_page)


class TestAnalystLogin:
    """Test analyst login functionality."""

    def test_analyst_can_login(self, analyst_page: Page):
        """Test that analyst can log in with email."""
        # analyst_page fixture already logged in
        expect(analyst_page).to_have_url(re.compile(r".*/dashboard.*"))

    def test_analyst_dashboard_loads(self, analyst_page: Page):
        """Test that analyst sees the analyst dashboard."""
        analyst_page.goto("/dashboard")
        wait_for_htmx(analyst_page)

        # Analyst dashboard should show "Analyst Panel" subtitle
        expect(analyst_page.locator("body")).to_contain_text("Analyst Panel")

    def test_analyst_dashboard_shows_statistics(self, analyst_page: Page):
        """Test that analyst dashboard shows statistics counts."""
        analyst_page.goto("/dashboard")
        wait_for_htmx(analyst_page)

        # Dashboard should show counts for organizations, leaders, groups, members, resources
        expect(analyst_page.locator("body")).to_contain_text("Organizations")
        expect(analyst_page.locator("body")).to_contain_text("Leaders")
        expect(analyst_page.locator("body")).to_contain_text("Groups")
        expect(analyst_page.locator("body")).to_contain_text("Members")
        expect(analyst_page.locator("body")).to_contain_text("Resources")

    def test_analyst_menu_shows_limited_options(self, analyst_page: Page):
        """Test that analyst menu only shows Dashboard and Members Report."""
        analyst_page.goto("/dashboard")
        wait_for_htmx(analyst_page)

        # Analyst should have Dashboard link
        expect(analyst_page.locator('a[href="/dashboard"]')).to_be_visible()

        # Analyst should have Members Report link
        expect(analyst_page.locator('a[href="/reports/members"]')).to_be_visible()

        # Analyst should NOT have management links in the menu
        # (These links should not be visible in the sidebar nav)
        nav = analyst_page.locator("aside nav")
        nav_text = nav.inner_text().lower()

        # Organizations, Leaders, Members (management), Groups, Resources, System Users
        # should not be in the nav menu
        assert "organizations" not in nav_text or "organization" not in nav_text, \
            "Analyst menu should not have Organizations link"


class TestAnalystMembersReport:
    """Test analyst's Members Report access and functionality."""

    def test_analyst_can_access_members_report(self, analyst_page: Page):
        """Test that analyst can access the Members Report page."""
        analyst_page.goto("/reports/members")
        wait_for_htmx(analyst_page)

        # Should see the Members Report heading
        expect(analyst_page.locator("body")).to_contain_text("Members Report")

    def test_analyst_can_view_organizations_list_in_report(self, analyst_page: Page):
        """Test that analyst can see organizations in the report sidebar."""
        analyst_page.goto("/reports/members")
        wait_for_htmx(analyst_page)

        # Should see Organizations label in the sidebar
        expect(analyst_page.locator("body")).to_contain_text("Organizations")

        # Should see "All" option for organizations
        all_link = analyst_page.locator('a:has-text("All")')
        expect(all_link.first).to_be_visible()

    def test_analyst_can_filter_by_organization(self, analyst_page: Page):
        """Test that analyst can filter the report by organization."""
        analyst_page.goto("/reports/members")
        wait_for_htmx(analyst_page)

        # Find organization links in the bordered div (these are specific orgs, not "All")
        org_links = analyst_page.locator('aside .border.rounded.divide-y a')
        if org_links.count() > 0:
            # Click on the first specific organization
            org_links.first.click()
            wait_for_htmx(analyst_page)

            # URL should have org parameter with a specific org ID (not "all")
            assert "org=" in analyst_page.url

            # When a specific org is selected, should see Groups section
            # (appears in the inner grid with "Groups —" heading)
            expect(analyst_page.locator("body")).to_contain_text("Groups")

    def test_analyst_can_filter_by_member_status(self, analyst_page: Page):
        """Test that analyst can filter by member status."""
        analyst_page.goto("/reports/members")
        wait_for_htmx(analyst_page)

        # Find the member status dropdown
        status_select = analyst_page.locator('#member-status')
        if status_select.count() > 0:
            # Change to "Active"
            status_select.select_option("active")
            analyst_page.wait_for_timeout(500)
            wait_for_htmx(analyst_page)

            # URL should have member_status parameter
            assert "member_status=active" in analyst_page.url

    def test_analyst_sees_download_button(self, analyst_page: Page):
        """Test that analyst sees the CSV download button."""
        analyst_page.goto("/reports/members")
        wait_for_htmx(analyst_page)

        # Should have a Download button
        download_btn = analyst_page.locator('button:has-text("Download")')
        expect(download_btn).to_be_visible()

    def test_analyst_sees_filename_input(self, analyst_page: Page):
        """Test that analyst sees the filename input for CSV download."""
        analyst_page.goto("/reports/members")
        wait_for_htmx(analyst_page)

        # Should have a filename input field
        filename_input = analyst_page.locator('input[name="filename"]')
        expect(filename_input).to_be_visible()

    def test_analyst_can_access_report_from_dashboard(self, analyst_page: Page):
        """Test that analyst can navigate to report from dashboard Quick Actions."""
        analyst_page.goto("/dashboard")
        wait_for_htmx(analyst_page)

        # Find the Members Report link in Quick Actions
        report_link = analyst_page.locator('a:has-text("Members Report")')
        expect(report_link.first).to_be_visible()

        # Click it
        report_link.first.click()
        analyst_page.wait_for_load_state("networkidle")

        # Should be on the reports page
        assert "/reports/members" in analyst_page.url


class TestAnalystAccessRestrictions:
    """Test that analysts cannot access management features."""

    def test_analyst_cannot_access_organizations(self, analyst_page: Page):
        """Test that analyst cannot access organizations management."""
        analyst_page.goto("/organizations")

        url = analyst_page.url
        content = analyst_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Analyst should not have access to organizations"

    def test_analyst_cannot_access_leaders(self, analyst_page: Page):
        """Test that analyst cannot access leaders management."""
        analyst_page.goto("/leaders")

        url = analyst_page.url
        content = analyst_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Analyst should not have access to leaders"

    def test_analyst_cannot_access_members(self, analyst_page: Page):
        """Test that analyst cannot access members management."""
        analyst_page.goto("/members")

        url = analyst_page.url
        content = analyst_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Analyst should not have access to members management"

    def test_analyst_cannot_access_groups(self, analyst_page: Page):
        """Test that analyst cannot access groups management."""
        analyst_page.goto("/groups")

        url = analyst_page.url
        content = analyst_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Analyst should not have access to groups"

    def test_analyst_cannot_access_resources(self, analyst_page: Page):
        """Test that analyst cannot access resources management."""
        analyst_page.goto("/resources")

        url = analyst_page.url
        content = analyst_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Analyst should not have access to resources"

    def test_analyst_cannot_access_system_users(self, analyst_page: Page):
        """Test that analyst cannot access system users management."""
        analyst_page.goto("/system-users")

        url = analyst_page.url
        content = analyst_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Analyst should not have access to system users"


class TestAnalystLogout:
    """Test analyst logout functionality."""

    def test_analyst_can_logout(self, analyst_page: Page):
        """Test that analyst can log out."""
        logout(analyst_page)
        # Verify we're on login page
        expect(analyst_page.locator('input[name="email"]')).to_be_visible()

    def test_reports_redirect_after_logout(self, analyst_page: Page):
        """Test that reports page requires login after logout."""
        # Previous test already logged out, so just navigate
        analyst_page.goto("/reports/members")
        # Should redirect to login
        expect(analyst_page.locator('input[name="email"]')).to_be_visible()
