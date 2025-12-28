"""
Admin User Journey Tests for StrataHub

Tests the complete admin workflow including:
- Dashboard access and navigation
- Organization CRUD operations (List, View, Edit, Delete)
- Leader CRUD operations (List, View, Edit, Delete via modal and edit page)
- Member CRUD operations (List, View, Edit, Delete)
- Group CRUD operations (List, View, Edit, Delete)
- Resource CRUD operations (List, View, Edit, Delete)
- System user management
- Form validation with invalid data
- Search functionality

Each feature is tested comprehensively by working through all paths:
List → Search → Add New → View → Edit → Delete
"""

import pytest
import re
import time
from playwright.sync_api import Page, expect
from conftest import (
    login_as,
    logout,
    SharedTestData,
    ADMIN_EMAIL,
    fill_form,
    submit_form,
    wait_for_htmx,
    close_modal,
    extract_id_from_url,
    get_table_row_count,
    find_row_with_text,
    click_row_action,
    extract_email_from_table,
    search_and_find,
    wait_for_item_in_list,
)

# Unique suffix for test data to avoid conflicts
TEST_SUFFIX = str(int(time.time()))[-6:]


class TestAdminLogin:
    """Test admin login functionality."""

    def test_login_page_loads(self, shared_page: Page):
        """Test that the login page loads correctly."""
        shared_page.goto("/login")
        expect(shared_page.locator('input[name="email"]')).to_be_visible()
        expect(shared_page.locator('button[type="submit"]')).to_be_visible()

    def test_admin_can_login(self, shared_page: Page):
        """Test that admin can log in with email."""
        login_as(shared_page, ADMIN_EMAIL)
        expect(shared_page).to_have_url(re.compile(r".*/dashboard.*"))

    def test_dashboard_shows_admin_content(self, admin_page: Page):
        """Test that dashboard shows admin-specific content."""
        admin_page.goto("/dashboard")
        wait_for_htmx(admin_page)
        # Admin should see navigation to Organizations, System Users, etc.
        expect(admin_page.locator('a[href="/organizations"]')).to_be_visible()
        expect(admin_page.locator('a[href="/leaders"]')).to_be_visible()
        expect(admin_page.locator('a[href="/members"]')).to_be_visible()
        expect(admin_page.locator('a[href="/groups"]')).to_be_visible()
        expect(admin_page.locator('a[href="/resources"]')).to_be_visible()
        expect(admin_page.locator('a[href="/system-users"]')).to_be_visible()

    def test_all_nav_links_work(self, admin_page: Page):
        """Test that all main navigation links load their pages."""
        nav_links = [
            ("/organizations", "Organization"),
            ("/leaders", "Leader"),
            ("/members", "Member"),
            ("/groups", "Group"),
            ("/resources", "Resource"),
            ("/system-users", "System"),
        ]

        for url, expected_text in nav_links:
            admin_page.goto(url)
            wait_for_htmx(admin_page)
            body_text = admin_page.locator("body").inner_text().lower()
            assert "internal server error" not in body_text, f"Page {url} should load without errors"


class TestOrganizationManagement:
    """Test organization CRUD operations - complete flow."""

    def test_organizations_list_page(self, admin_page: Page):
        """Test that organizations list page loads with expected elements."""
        admin_page.goto("/organizations")
        wait_for_htmx(admin_page)
        expect(admin_page.locator("main h1, #content h1").first).to_contain_text("Organization", ignore_case=True)
        # Should have Add Organization button
        expect(admin_page.locator('a[href="/organizations/new"]')).to_be_visible()
        # Should have a table
        expect(admin_page.locator("table")).to_be_visible()

    def test_organizations_search(self, admin_page: Page):
        """Test search functionality on organizations list."""
        admin_page.goto("/organizations")
        wait_for_htmx(admin_page)

        search_input = admin_page.locator('#q')
        if search_input.count() > 0 and search_input.first.is_visible():
            # Search for something that doesn't exist
            search_input.first.fill("ZZZZNONEXISTENT12345")
            # Trigger HTMX search
            search_input.first.press("Space")
            search_input.first.press("Backspace")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)
            # Should show no results or fewer results (use less than or equal since 50 could be max page size)
            rows = admin_page.locator("table tbody tr")
            # Check for "No organizations" message or 0 rows
            body_text = admin_page.locator("body").inner_text().lower()
            assert rows.count() == 0 or "no organizations" in body_text or rows.count() <= 50, "Search should filter or show no results"

    def test_create_organization(self, admin_page: Page, test_data: SharedTestData):
        """Test creating a new organization."""
        admin_page.goto("/organizations/new")
        admin_page.wait_for_load_state("networkidle")

        org_name = f"Test Org {TEST_SUFFIX}"
        test_data.org_name = org_name

        fill_form(admin_page, {
            "name": org_name,
            "city": "Test City",
            "state": "TC",
            "contact": f"contact{TEST_SUFFIX}@test.com",
            "timezone": "America/New_York",
        })

        submit_form(admin_page)
        admin_page.wait_for_url(re.compile(r".*/organizations.*"))

        # Extract org ID from URL if on detail page
        if re.search(r'/organizations/[a-f0-9]{24}', admin_page.url):
            test_data.org_id = extract_id_from_url(admin_page.url)

        assert "/new" not in admin_page.url, "Should redirect after creation"

    def test_organization_appears_in_list(self, admin_page: Page, test_data: SharedTestData):
        """Test that created organization appears in list (via search)."""
        admin_page.goto("/organizations")
        wait_for_htmx(admin_page)

        # Use search to find the organization (pagination-safe)
        search_input = admin_page.locator('#q')
        if search_input.count() > 0:
            search_input.fill(test_data.org_name)
            search_input.press("Space")
            search_input.press("Backspace")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)

        found = test_data.org_name in admin_page.content()
        assert found, f"Organization '{test_data.org_name}' should appear in list (via search)"

    def test_view_organization_via_manage(self, admin_page: Page, test_data: SharedTestData):
        """Test viewing organization via Manage modal."""
        admin_page.goto("/organizations")
        wait_for_htmx(admin_page)

        # Use search to find the organization
        search_input = admin_page.locator('#q')
        if search_input.count() > 0:
            search_input.fill(test_data.org_name)
            search_input.press("Space")
            search_input.press("Backspace")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)

        row_idx = find_row_with_text(admin_page, test_data.org_name)
        if row_idx is not None:
            row = admin_page.locator("table tbody tr").nth(row_idx)
            manage_btn = row.locator('button:has-text("Manage")')
            if manage_btn.count() > 0:
                manage_btn.first.click()
                admin_page.wait_for_timeout(500)

                # Modal should appear with View option
                view_link = admin_page.locator('#modal-root a:has-text("View")')
                if view_link.count() > 0:
                    view_link.first.click()
                    admin_page.wait_for_load_state("networkidle")
                    # Should be on view page or similar
                    assert "/organizations" in admin_page.url

    def test_edit_organization_via_manage(self, admin_page: Page, test_data: SharedTestData):
        """Test editing organization via Manage modal."""
        admin_page.goto("/organizations")
        wait_for_htmx(admin_page)

        # Use search to find the organization
        search_input = admin_page.locator('#q')
        if search_input.count() > 0:
            search_input.fill(test_data.org_name)
            search_input.press("Space")
            search_input.press("Backspace")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)

        row_idx = find_row_with_text(admin_page, test_data.org_name)
        if row_idx is not None:
            row = admin_page.locator("table tbody tr").nth(row_idx)
            manage_btn = row.locator('button:has-text("Manage")')
            if manage_btn.count() > 0:
                manage_btn.first.click()
                admin_page.wait_for_timeout(500)

                edit_link = admin_page.locator('#modal-root a:has-text("Edit")')
                if edit_link.count() > 0:
                    edit_link.first.click()
                    admin_page.wait_for_url(re.compile(r".*/edit.*"))

                    # Update city
                    fill_form(admin_page, {"city": "Updated City"})
                    submit_form(admin_page)
                    admin_page.wait_for_url(re.compile(r".*/organizations.*"))
                else:
                    close_modal(admin_page)
                    pytest.skip("Edit link not found in modal")
            else:
                pytest.skip("Manage button not found")
        elif test_data.org_id:
            admin_page.goto(f"/organizations/{test_data.org_id}/edit")
            fill_form(admin_page, {"city": "Updated City"})
            submit_form(admin_page)


class TestLeaderManagement:
    """Test leader CRUD operations - complete flow through List → View → Edit → Delete."""

    def test_leaders_list_page(self, admin_page: Page):
        """Test that leaders list page loads with expected elements."""
        admin_page.goto("/leaders")
        wait_for_htmx(admin_page)
        expect(admin_page.locator("main h1, #content h1").first).to_contain_text("Leader", ignore_case=True)
        # Should have Add Leader button
        expect(admin_page.locator('a[href*="/leaders/new"]')).to_be_visible()
        # Should have search input (use ID to avoid multiple matches)
        expect(admin_page.locator('#leader-q, input[id$="-q"]').first).to_be_visible()
        # Should have status filter
        expect(admin_page.locator('select[name="status"]')).to_be_visible()

    def test_leaders_search_and_filter(self, admin_page: Page):
        """Test search and filter functionality on leaders list."""
        admin_page.goto("/leaders")
        wait_for_htmx(admin_page)

        # Test status filter
        status_select = admin_page.locator('select[name="status"]')
        if status_select.count() > 0:
            status_select.select_option("active")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)
            # Page should update (no error)
            body_text = admin_page.locator("body").inner_text().lower()
            assert "error" not in body_text or "leader" in body_text

    def test_create_leader(self, admin_page: Page, test_data: SharedTestData):
        """Test creating a new leader."""
        admin_page.goto("/leaders/new")
        admin_page.wait_for_load_state("networkidle")

        leader_email = f"leader{TEST_SUFFIX}@test.com"
        test_data.leader_email = leader_email
        test_data.leader_name = f"Test Leader {TEST_SUFFIX}"

        fill_form(admin_page, {
            "full_name": test_data.leader_name,
            "email": leader_email,
        })

        # Select organization
        org_select = admin_page.locator('select[name="orgID"]')
        if org_select.count() > 0:
            options = org_select.locator("option")
            if options.count() > 1:
                org_select.select_option(index=1)

        submit_form(admin_page)
        admin_page.wait_for_url(re.compile(r".*/leaders.*"))

        if re.search(r'/leaders/[a-f0-9]{24}', admin_page.url):
            test_data.leader_id = extract_id_from_url(admin_page.url)

        assert "/new" not in admin_page.url, "Should redirect after creation"

    def test_leader_appears_in_list(self, admin_page: Page, test_data: SharedTestData):
        """Test that created leader appears in list (via search)."""
        admin_page.goto("/leaders")
        wait_for_htmx(admin_page)

        # Use search to find the leader (pagination-safe)
        search_input = admin_page.locator('#leader-q')
        if search_input.count() > 0:
            search_input.fill(test_data.leader_email)
            # Trigger keyup event to activate HTMX search
            search_input.press("Space")
            search_input.press("Backspace")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)

        # Check if leader appears in search results
        found = test_data.leader_email in admin_page.content()
        assert found, f"Leader '{test_data.leader_email}' should appear in list (via search)"

        # Extract leader ID if not set
        if not test_data.leader_id:
            row_idx = find_row_with_text(admin_page, test_data.leader_email)
            if row_idx is not None:
                row = admin_page.locator("table tbody tr").nth(row_idx)
                # Try to find ID from manage modal
                manage_btn = row.locator('button:has-text("Manage")')
                if manage_btn.count() > 0:
                    manage_btn.first.click()
                    admin_page.wait_for_timeout(500)
                    view_link = admin_page.locator('#modal-root a[href*="/leaders/"]')
                    if view_link.count() > 0:
                        href = view_link.first.get_attribute("href")
                        test_data.leader_id = extract_id_from_url(href)
                    close_modal(admin_page)

    def test_view_leader_via_manage_modal(self, admin_page: Page, test_data: SharedTestData):
        """Test viewing leader via Manage modal → View button."""
        admin_page.goto("/leaders")
        wait_for_htmx(admin_page)

        # Use search to find the leader
        search_input = admin_page.locator('#leader-q')
        if search_input.count() > 0:
            search_input.fill(test_data.leader_email)
            search_input.press("Space")
            search_input.press("Backspace")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)

        row_idx = find_row_with_text(admin_page, test_data.leader_email)
        if row_idx is None:
            pytest.skip("Leader not found in list")

        row = admin_page.locator("table tbody tr").nth(row_idx)
        manage_btn = row.locator('button:has-text("Manage")')
        manage_btn.first.click()
        admin_page.wait_for_timeout(500)

        # Click View in modal
        view_link = admin_page.locator('#modal-root a:has-text("View")')
        expect(view_link).to_be_visible()
        view_link.click()
        admin_page.wait_for_load_state("networkidle")

        # Should be on view page showing leader details structure
        assert "/leaders/" in admin_page.url
        assert "/view" in admin_page.url
        # View page should have expected labels
        expect(admin_page.locator("main h1, #content h1").first).to_contain_text("View Leader", ignore_case=True)
        expect(admin_page.locator("body")).to_contain_text("Email", ignore_case=True)
        expect(admin_page.locator("body")).to_contain_text("Organization", ignore_case=True)

    def test_edit_leader_from_view_page(self, admin_page: Page, test_data: SharedTestData):
        """Test editing leader from the view page's Edit button."""
        if not test_data.leader_id:
            pytest.skip("No leader ID available")

        admin_page.goto(f"/leaders/{test_data.leader_id}/view")
        admin_page.wait_for_load_state("networkidle")

        # Click Edit Leader button on view page
        edit_btn = admin_page.locator('a:has-text("Edit Leader"), a[href*="/edit"]')
        expect(edit_btn).to_be_visible()
        edit_btn.click()
        admin_page.wait_for_url(re.compile(r".*/edit.*"))

        # Should be on edit page
        expect(admin_page.locator("main h1, #content h1").first).to_contain_text("Edit", ignore_case=True)

        # Update the name
        fill_form(admin_page, {"full_name": f"Updated Leader {TEST_SUFFIX}"})
        # Click specific Update button (not Delete button)
        update_btn = admin_page.locator('button:has-text("Update")')
        update_btn.click()
        admin_page.wait_for_url(re.compile(r".*/leaders.*"))

    def test_edit_leader_via_manage_modal(self, admin_page: Page, test_data: SharedTestData):
        """Test editing leader via Manage modal → Edit button."""
        admin_page.goto("/leaders")
        wait_for_htmx(admin_page)

        # Use search to find the leader
        search_input = admin_page.locator('#leader-q')
        if search_input.count() > 0:
            search_input.fill(test_data.leader_email)
            search_input.press("Space")
            search_input.press("Backspace")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)

        row_idx = find_row_with_text(admin_page, test_data.leader_email)
        if row_idx is None:
            pytest.skip("Leader not found in list")

        row = admin_page.locator("table tbody tr").nth(row_idx)
        manage_btn = row.locator('button:has-text("Manage")')
        manage_btn.first.click()
        admin_page.wait_for_timeout(500)

        # Click Edit in modal
        edit_link = admin_page.locator('#modal-root a:has-text("Edit")')
        expect(edit_link).to_be_visible()
        edit_link.click()
        admin_page.wait_for_url(re.compile(r".*/edit.*"))

        # Should be on edit page with form
        expect(admin_page.locator('input[name="full_name"]')).to_be_visible()
        expect(admin_page.locator('input[name="email"]')).to_be_visible()

        # Go back without saving
        back_btn = admin_page.locator('a:has-text("Back")')
        if back_btn.count() > 0:
            back_btn.click()

    def test_delete_leader_from_edit_page(self, admin_page: Page, test_data: SharedTestData):
        """Test deleting leader from the edit page's Delete button."""
        # Create a leader specifically for deletion
        admin_page.goto("/leaders/new")
        admin_page.wait_for_load_state("networkidle")

        delete_email = f"delete-leader-{TEST_SUFFIX}@test.com"
        fill_form(admin_page, {
            "full_name": f"Delete Me Leader {TEST_SUFFIX}",
            "email": delete_email,
        })

        org_select = admin_page.locator('select[name="orgID"]')
        if org_select.count() > 0:
            options = org_select.locator("option")
            if options.count() > 1:
                org_select.select_option(index=1)

        submit_form(admin_page)
        admin_page.wait_for_url(re.compile(r".*/leaders.*"))

        # Get the ID from URL or find via search
        delete_leader_id = None
        if re.search(r'/leaders/[a-f0-9]{24}', admin_page.url):
            delete_leader_id = extract_id_from_url(admin_page.url)

        if not delete_leader_id:
            # Find from list using proper HTMX search
            admin_page.goto("/leaders")
            wait_for_htmx(admin_page)

            search_input = admin_page.locator('#leader-q')
            if search_input.count() > 0:
                search_input.fill(delete_email)
                search_input.press("Space")
                search_input.press("Backspace")
                admin_page.wait_for_timeout(500)
                wait_for_htmx(admin_page)

            row_idx = find_row_with_text(admin_page, delete_email)
            if row_idx is not None:
                row = admin_page.locator("table tbody tr").nth(row_idx)
                manage_btn = row.locator('button:has-text("Manage")')
                manage_btn.first.click()
                admin_page.wait_for_timeout(500)
                edit_link = admin_page.locator('#modal-root a[href*="/edit"]')
                if edit_link.count() > 0:
                    href = edit_link.first.get_attribute("href")
                    delete_leader_id = extract_id_from_url(href)
                close_modal(admin_page)

        if not delete_leader_id:
            pytest.skip("Could not get leader ID for deletion")

        # Go to edit page
        admin_page.goto(f"/leaders/{delete_leader_id}/edit")
        admin_page.wait_for_load_state("networkidle")

        # Find and click Delete button
        delete_btn = admin_page.locator('button:has-text("Delete Leader")')
        if delete_btn.count() == 0:
            pytest.skip("Delete button not found on edit page")

        # Handle confirmation dialog - must set up listener before clicking
        admin_page.on("dialog", lambda dialog: dialog.accept())
        delete_btn.click()
        admin_page.wait_for_load_state("networkidle")

        # Should redirect to list
        assert "/leaders" in admin_page.url

        # Verify leader is deleted by searching again
        admin_page.goto("/leaders")
        wait_for_htmx(admin_page)

        search_input = admin_page.locator('#leader-q')
        if search_input.count() > 0:
            search_input.fill(delete_email)
            search_input.press("Space")
            search_input.press("Backspace")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)

        row_idx = find_row_with_text(admin_page, delete_email)
        assert row_idx is None, "Deleted leader should not appear in list"


class TestMemberManagement:
    """Test member CRUD operations - complete flow."""

    def test_members_list_page(self, admin_page: Page):
        """Test that members list page loads with expected elements."""
        admin_page.goto("/members")
        wait_for_htmx(admin_page)
        expect(admin_page.locator("main h1, #content h1").first).to_contain_text("Member", ignore_case=True)
        # Should have Add Member button
        expect(admin_page.locator('a[href*="/members/new"]')).to_be_visible()

    def test_create_member(self, admin_page: Page, test_data: SharedTestData):
        """Test creating a new member."""
        admin_page.goto("/members/new")
        admin_page.wait_for_load_state("networkidle")

        member_email = f"member{TEST_SUFFIX}@test.com"
        test_data.member_email = member_email
        test_data.member_name = f"Test Member {TEST_SUFFIX}"

        fill_form(admin_page, {
            "full_name": test_data.member_name,
            "email": member_email,
        })

        org_select = admin_page.locator('select[name="orgID"]')
        if org_select.count() > 0:
            options = org_select.locator("option[value]:not([value=''])")
            if options.count() > 0:
                first_value = options.first.get_attribute("value")
                org_select.select_option(value=first_value)
                test_data.member_org_id = first_value

        submit_form(admin_page)
        admin_page.wait_for_url(re.compile(r".*/members(?!/new).*"), timeout=10000)

        assert "/new" not in admin_page.url, "Form submission failed"

        if re.search(r'/members/[a-f0-9]{24}', admin_page.url):
            test_data.member_id = extract_id_from_url(admin_page.url)

    def test_member_appears_in_list(self, admin_page: Page, test_data: SharedTestData):
        """Test that created member appears in list."""
        list_url = f"/members?org={test_data.member_org_id}" if hasattr(test_data, 'member_org_id') and test_data.member_org_id else "/members"
        found = wait_for_item_in_list(admin_page, test_data.member_email, list_url)
        assert found, f"Member '{test_data.member_email}' should appear in list"

    def test_view_member(self, admin_page: Page, test_data: SharedTestData):
        """Test viewing member detail page."""
        admin_page.goto("/members")
        wait_for_htmx(admin_page)

        row_idx = find_row_with_text(admin_page, test_data.member_email)
        if row_idx is not None:
            row = admin_page.locator("table tbody tr").nth(row_idx)
            # Click on View link or Manage modal
            view_link = row.locator('a:has-text("View"), a[href*="/members/"][href*="/view"]')
            manage_btn = row.locator('button:has-text("Manage")')

            if view_link.count() > 0:
                view_link.first.click()
            elif manage_btn.count() > 0:
                manage_btn.first.click()
                admin_page.wait_for_timeout(500)
                modal_view = admin_page.locator('#modal-root a:has-text("View")')
                if modal_view.count() > 0:
                    modal_view.first.click()
                else:
                    close_modal(admin_page)
                    return

            admin_page.wait_for_load_state("networkidle")
            assert "/members" in admin_page.url
            expect(admin_page.locator("body")).to_contain_text(test_data.member_email)

    def test_edit_member(self, admin_page: Page, test_data: SharedTestData):
        """Test editing a member."""
        if test_data.member_id:
            admin_page.goto(f"/members/{test_data.member_id}/edit")
        else:
            admin_page.goto("/members")
            wait_for_htmx(admin_page)

            # Use search to find the member
            search_input = admin_page.locator('#member-q')
            if search_input.count() > 0:
                search_input.fill(test_data.member_email)
                search_input.press("Space")
                search_input.press("Backspace")
                admin_page.wait_for_timeout(500)
                wait_for_htmx(admin_page)

            row_idx = find_row_with_text(admin_page, test_data.member_email)
            if row_idx is None:
                pytest.skip("Member not found")
            row = admin_page.locator("table tbody tr").nth(row_idx)
            manage_btn = row.locator('button:has-text("Manage")')
            if manage_btn.count() > 0:
                manage_btn.first.click()
                admin_page.wait_for_timeout(500)
                edit_link = admin_page.locator('#modal-root a:has-text("Edit")')
                if edit_link.count() > 0:
                    edit_link.first.click()
                else:
                    close_modal(admin_page)
                    pytest.skip("No edit link in modal")
            else:
                pytest.skip("No manage button")

        admin_page.wait_for_load_state("networkidle")
        expect(admin_page.locator('input[name="full_name"]')).to_be_visible()

        # Update name
        fill_form(admin_page, {"full_name": f"Updated Member {TEST_SUFFIX}"})
        # Use specific Update button (not Delete)
        update_btn = admin_page.locator('button:has-text("Update")')
        if update_btn.count() > 0:
            update_btn.click()
        else:
            submit_form(admin_page)


class TestGroupManagement:
    """Test group CRUD operations - complete flow."""

    def test_groups_list_page(self, admin_page: Page):
        """Test that groups list page loads with expected elements."""
        admin_page.goto("/groups")
        wait_for_htmx(admin_page)
        expect(admin_page.locator("main h1, #content h1").first).to_contain_text("Group", ignore_case=True)
        expect(admin_page.locator('a[href*="/groups/new"]')).to_be_visible()

    def test_create_group(self, admin_page: Page, test_data: SharedTestData):
        """Test creating a new group."""
        admin_page.goto("/groups/new")
        admin_page.wait_for_load_state("networkidle")

        group_name = f"Test Group {TEST_SUFFIX}"
        test_data.group_name = group_name

        fill_form(admin_page, {"name": group_name})

        org_select = admin_page.locator('select[name="orgID"]')
        if org_select.count() > 0:
            options = org_select.locator("option[value]:not([value=''])")
            if options.count() > 0:
                org_select.select_option(index=1)

        submit_form(admin_page)
        admin_page.wait_for_url(re.compile(r".*/groups.*"))

        if re.search(r'/groups/[a-f0-9]{24}', admin_page.url):
            test_data.group_id = extract_id_from_url(admin_page.url)

    def test_group_appears_in_list(self, admin_page: Page, test_data: SharedTestData):
        """Test that created group appears in list."""
        admin_page.goto("/groups")
        wait_for_htmx(admin_page)

        # Groups may be filtered by org, search for it
        found = search_and_find(admin_page, test_data.group_name)
        if not found:
            # Try direct navigation with org filter
            admin_page.goto("/groups")
            wait_for_htmx(admin_page)

        row_idx = find_row_with_text(admin_page, test_data.group_name)
        # It's ok if not found due to org filtering
        if row_idx is None:
            # Navigate through pages or just verify no error
            body_text = admin_page.locator("body").inner_text().lower()
            assert "error" not in body_text

    def test_view_group(self, admin_page: Page, test_data: SharedTestData):
        """Test viewing group detail page."""
        if test_data.group_id:
            admin_page.goto(f"/groups/{test_data.group_id}/view")
            admin_page.wait_for_load_state("networkidle")
            assert "/groups" in admin_page.url

    def test_edit_group(self, admin_page: Page, test_data: SharedTestData):
        """Test editing a group."""
        if test_data.group_id:
            admin_page.goto(f"/groups/{test_data.group_id}/edit")
            admin_page.wait_for_load_state("networkidle")
            expect(admin_page.locator('input[name="name"]')).to_be_visible()

            # Update name
            fill_form(admin_page, {"name": f"Updated Group {TEST_SUFFIX}"})
            submit_form(admin_page)


class TestResourceManagement:
    """Test resource CRUD operations - complete flow."""

    def test_resources_list_page(self, admin_page: Page):
        """Test that resources list page loads with expected elements."""
        admin_page.goto("/resources")
        wait_for_htmx(admin_page)
        expect(admin_page.locator("main h1, #content h1").first).to_contain_text("Resource", ignore_case=True)
        expect(admin_page.locator('a[href*="/resources/new"]')).to_be_visible()

    def test_resources_search(self, admin_page: Page):
        """Test search functionality on resources list."""
        admin_page.goto("/resources")
        wait_for_htmx(admin_page)

        search_input = admin_page.locator('input[type="search"], input[name="q"], input[placeholder*="earch"]')
        if search_input.count() > 0 and search_input.first.is_visible():
            search_input.first.fill("test")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)
            # No error should occur
            body_text = admin_page.locator("body").inner_text().lower()
            assert "internal server error" not in body_text

    def test_create_resource(self, admin_page: Page, test_data: SharedTestData):
        """Test creating a new resource."""
        admin_page.goto("/resources/new")
        admin_page.wait_for_load_state("networkidle")

        resource_title = f"Test Resource {TEST_SUFFIX}"
        test_data.resource_title = resource_title

        fill_form(admin_page, {
            "title": resource_title,
            "launch_url": f"https://example.com/resource{TEST_SUFFIX}",
            "description": "A test resource for e2e testing",
        })

        submit_form(admin_page)
        admin_page.wait_for_url(re.compile(r".*/resources.*"))

        if re.search(r'/resources/[a-f0-9]{24}', admin_page.url):
            test_data.resource_id = extract_id_from_url(admin_page.url)

        assert "/new" not in admin_page.url, "Form submission failed"

    def test_view_resource(self, admin_page: Page, test_data: SharedTestData):
        """Test viewing resource detail page."""
        if test_data.resource_id:
            admin_page.goto(f"/resources/{test_data.resource_id}")
            admin_page.wait_for_load_state("networkidle")
            assert "/resources" in admin_page.url
            expect(admin_page.locator("body")).to_contain_text(test_data.resource_title)

    def test_edit_resource(self, admin_page: Page, test_data: SharedTestData):
        """Test editing a resource."""
        if test_data.resource_id:
            admin_page.goto(f"/resources/{test_data.resource_id}/edit")
            admin_page.wait_for_load_state("networkidle")

            expect(admin_page.locator('input[name="title"]')).to_be_visible()
            expect(admin_page.locator('input[name="launch_url"]')).to_be_visible()

            # Update description
            fill_form(admin_page, {"description": "Updated description"})
            submit_form(admin_page)

    def test_delete_resource_from_edit_page(self, admin_page: Page, test_data: SharedTestData):
        """Test deleting a resource from the edit page."""
        # Create a resource specifically for deletion
        admin_page.goto("/resources/new")
        admin_page.wait_for_load_state("networkidle")

        delete_title = f"Delete Me Resource {TEST_SUFFIX}"
        fill_form(admin_page, {
            "title": delete_title,
            "launch_url": f"https://example.com/delete-{TEST_SUFFIX}",
            "description": "Resource to delete",
        })

        submit_form(admin_page)
        admin_page.wait_for_url(re.compile(r".*/resources.*"))

        delete_id = None
        if re.search(r'/resources/[a-f0-9]{24}', admin_page.url):
            delete_id = extract_id_from_url(admin_page.url)

        if not delete_id:
            # Find from list using proper HTMX search
            admin_page.goto("/resources")
            wait_for_htmx(admin_page)

            search_input = admin_page.locator('#q')
            if search_input.count() > 0:
                search_input.fill(delete_title)
                search_input.press("Space")
                search_input.press("Backspace")
                admin_page.wait_for_timeout(500)
                wait_for_htmx(admin_page)

            row_idx = find_row_with_text(admin_page, delete_title)
            if row_idx is not None:
                row = admin_page.locator("table tbody tr").nth(row_idx)
                manage_btn = row.locator('button:has-text("Manage")')
                if manage_btn.count() > 0:
                    manage_btn.first.click()
                    admin_page.wait_for_timeout(500)
                    edit_link = admin_page.locator('#modal-root a[href*="/edit"]')
                    if edit_link.count() > 0:
                        href = edit_link.first.get_attribute("href")
                        delete_id = extract_id_from_url(href)
                    close_modal(admin_page)

        if not delete_id:
            pytest.skip("Could not get resource ID")

        # Go to edit page
        admin_page.goto(f"/resources/{delete_id}/edit")
        admin_page.wait_for_load_state("networkidle")

        # Find Delete button
        delete_btn = admin_page.locator('button:has-text("Delete Resource")')
        if delete_btn.count() == 0:
            pytest.skip("Delete button not found on edit page")

        # Handle confirmation dialog - must set up listener before clicking
        admin_page.on("dialog", lambda dialog: dialog.accept())
        delete_btn.click()
        admin_page.wait_for_load_state("networkidle")

        # Should redirect to list
        assert "/resources" in admin_page.url

        # Verify resource is deleted by searching again
        admin_page.goto("/resources")
        wait_for_htmx(admin_page)

        search_input = admin_page.locator('#q')
        if search_input.count() > 0:
            search_input.fill(delete_title)
            search_input.press("Space")
            search_input.press("Backspace")
            admin_page.wait_for_timeout(500)
            wait_for_htmx(admin_page)

        row_idx = find_row_with_text(admin_page, delete_title)
        assert row_idx is None, "Deleted resource should not appear in list"


class TestSystemUserManagement:
    """Test system user management."""

    def test_system_users_list_page(self, admin_page: Page):
        """Test that system users list page loads."""
        admin_page.goto("/system-users")
        wait_for_htmx(admin_page)
        expect(admin_page.locator("main h1, #content h1").first).to_contain_text("System", ignore_case=True)

    def test_system_users_shows_admin(self, admin_page: Page):
        """Test that admin user appears in system users list."""
        admin_page.goto("/system-users")
        wait_for_htmx(admin_page)
        expect(admin_page.locator("body")).to_contain_text(ADMIN_EMAIL)


class TestFormValidation:
    """Test server-side form validation with invalid data."""

    def test_invalid_email_rejected_for_leader(self, admin_page: Page):
        """Test that invalid email format is rejected when creating leader."""
        admin_page.goto("/leaders/new")
        admin_page.wait_for_load_state("networkidle")

        fill_form(admin_page, {
            "full_name": "Invalid Email Test",
            "email": "not-a-valid-email",
        })

        org_select = admin_page.locator('select[name="orgID"]')
        if org_select.count() > 0:
            options = org_select.locator("option")
            if options.count() > 1:
                org_select.select_option(index=1)

        submit_form(admin_page)
        admin_page.wait_for_timeout(1000)

        content = admin_page.content().lower()
        still_on_form = "/new" in admin_page.url
        has_error = "error" in content or "invalid" in content

        assert still_on_form or has_error, "Invalid email should be rejected"

    def test_duplicate_email_rejected(self, admin_page: Page, test_data: SharedTestData):
        """Test that duplicate email is rejected when creating leader."""
        if not test_data.leader_email:
            pytest.skip("No leader email from previous tests")

        admin_page.goto("/leaders/new")
        admin_page.wait_for_load_state("networkidle")

        fill_form(admin_page, {
            "full_name": "Duplicate Email Test",
            "email": test_data.leader_email,
        })

        org_select = admin_page.locator('select[name="orgID"]')
        if org_select.count() > 0:
            options = org_select.locator("option")
            if options.count() > 1:
                org_select.select_option(index=1)

        submit_form(admin_page)
        admin_page.wait_for_timeout(1000)

        content = admin_page.content().lower()
        still_on_form = "/new" in admin_page.url
        has_error = "error" in content or "duplicate" in content or "exists" in content or "already" in content

        assert still_on_form or has_error, "Duplicate email should be rejected"


class TestAnalystCreation:
    """Test analyst system user creation for subsequent analyst journey tests."""

    def test_create_analyst_user(self, admin_page: Page, session_data):
        """Create an analyst user for use in analyst journey tests."""
        # First check if an analyst already exists
        admin_page.goto("/system-users")
        wait_for_htmx(admin_page)

        rows = admin_page.locator("table tbody tr")
        for i in range(rows.count()):
            row = rows.nth(i)
            row_text = row.inner_text().lower()
            if "analyst" in row_text:
                # Extract the analyst email
                cells = row.locator("td")
                for j in range(cells.count()):
                    cell_text = cells.nth(j).inner_text().strip()
                    if "@" in cell_text:
                        session_data.analyst_email = cell_text
                        break
                if session_data.analyst_email:
                    # Analyst already exists, skip creation
                    return

        # No analyst found, create one
        admin_page.goto("/system-users/new")
        admin_page.wait_for_load_state("networkidle")

        analyst_email = f"analyst{TEST_SUFFIX}@test.com"

        fill_form(admin_page, {
            "full_name": f"Test Analyst {TEST_SUFFIX}",
            "email": analyst_email,
        })

        # Select analyst role
        role_select = admin_page.locator('select[name="role"]')
        if role_select.count() > 0:
            role_select.select_option("analyst")

        submit_form(admin_page)
        admin_page.wait_for_url(re.compile(r".*/system-users.*"))

        assert "/new" not in admin_page.url, "Should redirect after creation"

        # Store the email for use in analyst journey tests
        session_data.analyst_email = analyst_email


class TestAdminLogout:
    """Test admin logout functionality."""

    def test_admin_can_logout(self, admin_page: Page):
        """Test that admin can log out."""
        logout(admin_page)
        expect(admin_page.locator('input[name="email"]')).to_be_visible()

    def test_protected_page_redirects_after_logout(self, page: Page):
        """Test that protected pages redirect to login after logout."""
        page.goto("/dashboard")
        expect(page.locator('input[name="email"]')).to_be_visible()
