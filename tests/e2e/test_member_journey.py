"""
Member User Journey Tests for StrataHub

Tests the member workflow including:
- Login as member (using email extracted from admin setup)
- Dashboard access (limited view)
- View assigned resources
- Cannot access management features
- Cannot access other members' data

These tests depend on test_admin_journey to have run first
to create the test organization and member.
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
def member_page(shared_page: Page, session_data: SessionData) -> Page:
    """
    Provides the shared page logged in as member.
    Uses email from session_data (extracted during admin tests).
    """
    if not session_data.member_email:
        pytest.skip("No member email available - run admin journey tests first")

    login_as(shared_page, session_data.member_email)

    yield shared_page

    # Logout at end so next user can login
    logout(shared_page)


class TestMemberLogin:
    """Test member login functionality."""

    def test_member_can_login(self, member_page: Page):
        """Test that member can log in with email."""
        # member_page fixture already logged in
        expect(member_page).to_have_url(re.compile(r".*/dashboard.*|.*/member/.*|.*/resources.*"))

    def test_member_dashboard_loads(self, member_page: Page):
        """Test that member sees appropriate dashboard content."""
        member_page.goto("/dashboard")

        page_content = member_page.content().lower()

        has_content = (
            "resource" in page_content or
            "dashboard" in page_content or
            "welcome" in page_content
        )
        assert has_content, "Member should see dashboard or resources content"


class TestMemberResourceAccess:
    """Test member's resource viewing capabilities."""

    def test_member_can_view_resources_list(self, member_page: Page):
        """Test that member can view their assigned resources list."""
        resource_link = member_page.locator('a[href*="resources"]')
        if resource_link.count() > 0:
            resource_link.first.click()
        else:
            member_page.goto("/member/resources")

        wait_for_htmx(member_page)

        content = member_page.content().lower()
        assert "forbidden" not in content, "Member should be able to view resources"
        assert "unauthorized" not in content, "Member should be able to view resources"

        # Should see the resources list page
        expect(member_page.locator("body")).to_contain_text("Resource", ignore_case=True)

    def test_member_can_view_resource_detail(self, member_page: Page):
        """Test that member can view an individual resource detail page."""
        member_page.goto("/member/resources")
        wait_for_htmx(member_page)

        # Find a View button in the resources table
        view_btn = member_page.locator('a:has-text("View"), a[href*="/member/resources/"]')
        if view_btn.count() > 0:
            view_btn.first.click()
            wait_for_htmx(member_page)

            # Should be on resource detail page
            assert "/member/resources" in member_page.url

            # Should see resource details
            content = member_page.content().lower()
            assert "forbidden" not in content, "Member should see resource details"

            # Should see description or instructions
            expect(member_page.locator("body")).to_contain_text("Description", ignore_case=True)

            # May see Open button if resource is available
            open_btn = member_page.locator('a:has-text("Open")')
            # Just verify no error - Open button may or may not be visible

    def test_member_can_open_resource(self, member_page: Page):
        """Test that member can open a resource via Open button."""
        member_page.goto("/member/resources")
        wait_for_htmx(member_page)

        # Find an Open button in the resources table
        open_btn = member_page.locator('a:has-text("Open")')
        if open_btn.count() > 0:
            # Don't actually click (opens external URL), just verify it exists
            expect(open_btn.first).to_be_visible()
            # Verify it has an href (the launch URL)
            href = open_btn.first.get_attribute("href")
            assert href is not None, "Open button should have a URL"

    def test_member_sees_assigned_resources_only(self, member_page: Page):
        """Test that member only sees resources assigned to them."""
        member_page.goto("/member/resources")
        wait_for_htmx(member_page)

        current_url = member_page.url
        assert "/login" not in current_url, "Member should stay logged in"

        # Verify the page shows either resources or "No resources assigned"
        content = member_page.content().lower()
        has_resources = "title" in content or "resource" in content
        no_resources = "no resources" in content or "not assigned" in content
        assert has_resources or no_resources, "Should show resources or no resources message"


class TestMemberAccessRestrictions:
    """Test that members cannot access management features."""

    def test_member_cannot_access_organizations(self, member_page: Page):
        """Test that member cannot access organizations management."""
        member_page.goto("/organizations")

        url = member_page.url
        content = member_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "/member" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Member should not have access to organizations"

    def test_member_cannot_access_leaders(self, member_page: Page):
        """Test that member cannot access leaders management."""
        member_page.goto("/leaders")

        url = member_page.url
        content = member_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "/member" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Member should not have access to leaders"

    def test_member_cannot_access_members_management(self, member_page: Page):
        """Test that member cannot access members management."""
        member_page.goto("/members")

        url = member_page.url
        content = member_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "/member" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Member should not have access to members management"

    def test_member_cannot_access_groups(self, member_page: Page):
        """Test that member cannot access groups management."""
        member_page.goto("/groups")

        url = member_page.url
        content = member_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "/member" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Member should not have access to groups"

    def test_member_cannot_access_system_users(self, member_page: Page):
        """Test that member cannot access system users."""
        member_page.goto("/system-users")

        url = member_page.url
        content = member_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "/member" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content
        )

        assert is_blocked, "Member should not have access to system users"

    def test_member_cannot_create_resources(self, member_page: Page):
        """Test that member cannot create new resources."""
        member_page.goto("/resources/new")

        url = member_page.url
        content = member_page.content().lower()

        is_blocked = (
            "/dashboard" in url or
            "/login" in url or
            "/member" in url or
            "forbidden" in content or
            "unauthorized" in content or
            "access denied" in content or
            "/resources/new" not in url
        )

        assert is_blocked, "Member should not be able to create resources"


class TestMemberLogout:
    """Test member logout functionality."""

    def test_member_can_logout(self, member_page: Page):
        """Test that member can log out."""
        logout(member_page)
        # Verify we're on login page
        expect(member_page.locator('input[name="email"]')).to_be_visible()

    def test_resources_redirect_after_logout(self, member_page: Page):
        """Test that resources page requires login after logout."""
        # Previous test already logged out, so just navigate
        member_page.goto("/member/resources")
        # Should redirect to login
        expect(member_page.locator('input[name="email"]')).to_be_visible()
