# Time Progression and Academic Cycles

This document captures architectural considerations for handling the cyclical nature of educational environments in StrataHub.

---

## The Problem

Educational environments are cyclical:
- Semesters/terms end (spring classes finish)
- New terms begin (fall classes start)
- Same courses may run again with different students
- Teachers may want to reuse content/structure
- Historical data needs to be preserved for reporting
- Active views shouldn't be cluttered with past classes

**Example scenario:** Spring 2025 classes end. Fall 2025 classes need to be set up. Teachers need access to historical data from spring, but students shouldn't see old assignments.

---

## Approaches Considered

### 1. Term/Semester Entity (Canvas-style)

Add a first-class "Term" concept:

```
Term:
  - id, workspace_id
  - name ("Spring 2025", "Fall 2025")
  - start_date, end_date
  - status (upcoming, active, completed)

Group:
  - term_id (optional, null = ongoing/no term)
  - ... existing fields
```

**Pros:**
- Filter/archive groups by term
- Bulk operations ("end Spring 2025")
- Term-scoped reporting
- Clear mental model for educators

**Cons:**
- Another entity to manage
- Not all use cases are term-based (ongoing cohorts, corporate training)
- More complex UI

### 2. Course Template + Instance

Separate the *definition* from the *offering*:

```
Course (template):
  - id, name, description
  - default_resources (what to assign)

Group (instance):
  - course_id (optional - links to template)
  - term_id
  - members, leaders, resource assignments
```

**Pros:**
- "Clone from last semester" is easy
- Course exists independently of any instance
- Can track course-level analytics across terms

**Cons:**
- More complex data model
- Might be overkill for current needs
- Requires UI for managing templates

### 3. Group Lifecycle + Clone (Pragmatic) ✓

Keep current model, add:
- **Archive status**: Groups can be `active` or `archived`
- **Clone operation**: Copy group → new group with same resources, empty roster
- **Bulk archive**: "Archive all groups in this org"

**Pros:**
- Minimal schema changes
- Works with current implementation
- Can evolve to terms later if needed
- Flexible for non-term-based use cases

**Cons:**
- No explicit term concept for filtering/reporting
- Manual process to set up new semesters

### 4. Date-Driven Visibility

Resources already have `visible_from` and `visible_until`. Groups could have similar:

```
Group:
  - active_from, active_until (optional)
```

Groups outside their active window are automatically hidden from members.

**Pros:**
- Automatic based on dates
- No manual archiving needed

**Cons:**
- Requires knowing dates upfront
- Less explicit control than status-based

---

## Decision: Archive-Based Approach (December 2024)

For the current implementation and use case, the **archive-based approach** (Option 3) is the right fit:

### Groups
- Groups can be marked as `archived`
- **Members** cannot access archived groups or their resources
- **Leaders, Admins, Coordinators** can still view archived groups for historical data
- Lists default to showing only active groups, with filter option to include archived
- Resources can be reassigned to new groups as needed (ad-hoc approach)

### Members
- Members not assigned to any active groups simply have no access
- Members can optionally be archived if needed (same pattern as groups)
- Archived members are hidden from active lists but data is preserved

### Implementation Notes
- Current `status` field (`active`, `disabled`) may need to distinguish between:
  - `disabled` = account suspended/inactive
  - `archived` = historical, preserved for records
- Or add separate `archived` boolean field
- List views need "Show archived" filter toggle

---

## Data Growth Considerations

Over time, data accumulates:
- Past groups with their assignments
- Historical member records
- Resource assignment history

This is normal for any long-running system. Potential future mitigations:
- Dashboard/reporting filtered to recent terms by default
- Optional data export + purge for very old records
- Storage tier considerations for historical vs. active data

---

## COPPA Compliance: Data Deletion

**Important:** Under COPPA, if a parent requests deletion of their child's data, we must:
- Delete the user record
- Delete all instances of their data (assignments, progress, etc.)
- Not retain any personally identifiable information

This requires:
- Cascade delete functionality for users
- Audit of all places user data is stored
- Ensure archived records are also deleted when user is deleted
- Consider what happens to aggregate/anonymous data (may be retained)

**Current collections that reference users:**
- `users` - the user record itself
- `group_memberships` - user's group associations
- `group_resource_assignments` - indirectly via group membership
- `material_assignments` - for leaders
- `login_records` - login history

When implementing user deletion, all related records must be removed.

---

## Future Evolution Path

If needs grow more sophisticated:

1. **Phase A (Current):** Archive-based lifecycle
   - Add archive capability to groups
   - Filter toggles in list views
   - Clone group feature (nice to have)

2. **Phase B (If Needed):** Term entity
   - Add optional Term to groups
   - Term-based filtering and bulk operations
   - Term-scoped reporting

3. **Phase C (If Needed):** Course templates
   - Separate course definition from instance
   - Template-based group creation
   - Cross-term course analytics

---

## Questions for Future Consideration

1. **Clone feature:** How valuable is "clone this group for next semester"?
   - Copies group settings and resource assignments
   - Empty member roster
   - Teachers manually add new students

2. **Bulk operations:**
   - "Archive all groups in Organization X"
   - "Archive all groups older than [date]"

3. **Member lifecycle:**
   - When a member has no active group memberships, what's their experience?
   - Should there be a "graduated" or "alumni" status?

4. **Reporting across time:**
   - Do admins need to compare performance across semesters?
   - If yes, Term entity becomes more valuable

---

*Document created: December 2024*
*Status: No immediate action required. Revisit when implementing group archiving.*
