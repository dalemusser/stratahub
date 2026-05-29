# Members Report — Resolving Identity from Hex IDs

_Audience: authorized StrataHub staff (admins, analysts, coordinators, leaders)
and research-team members who hold de-identified data and need to re-link it to
real people._

The de-identified URL schemes (and any data a consumer collects keyed on them)
identify a student only by opaque 24-char hex IDs: `user_id`, `group_id`,
`org_id`, `ws_id`. The **Members Report** is the authorized crosswalk that maps
those same hex IDs back to names, organizations, groups, and logins.

In other words: the launch URL sends out **de-identified** identifiers; the
Members Report is how an authorized person turns them back into **identities**.

---

## Where it is

- **Page:** `https://<workspace>.adroit.games/reports/members` — an org/group
  browser with member counts and a CSV download.
- **Export:** the **Download** button streams a CSV (`/reports/members.csv`)
  with one row per member-group membership (plus one row for members not in any
  group in scope).

## Who can run it

| Role | Scope |
|------|-------|
| Superadmin / Admin / Analyst | All organizations in the workspace |
| Coordinator | Their assigned organization(s) |
| Leader | Their own organization |
| Member (or not signed in) | No access |

The report is always scoped to one workspace (the one in the URL).

---

## What the CSV provides

| Column | Meaning | Type |
|--------|---------|------|
| `workspace` | Workspace subdomain | name |
| `workspace_id` | Workspace ObjectID | **hex** |
| `user_id` | Member ObjectID | **hex** |
| `full_name` | Member's full name | PII |
| `login_id` | Member's login | PII |
| `email` | Member's email | PII |
| `organization` | Organization name | name |
| `organization_id` | Organization ObjectID | **hex** |
| `group` | Group name | name |
| `group_id` | Group ObjectID | **hex** |
| `leaders` | Teacher(s) for the group, by name, `|`-separated | PII |
| `status` | `active` / `disabled` | — |

The four **hex** columns are the join keys back to de-identified data. The other
columns are the identity they resolve to.

---

## The key idea: the hex IDs are the same everywhere

A hex ID means the same thing across the whole platform. The `user_id` a survey
received in its launch URL is byte-for-byte the same `user_id` in this report;
the `group_id` is the same `group_id`; and so on. Because ObjectIDs are globally
unique and immutable, the join is exact and stable — even if someone later
renames the group or the student.

So to re-identify de-identified data, you **join on the hex columns**.

### Parameter ↔ column crosswalk

If your de-identified data came from the URL identity parameters, note that two
of the hex keys have a slightly different *name* in the URL vs. the CSV (the
*value* is identical):

| In a launch URL (param) | In the Members Report (column) | Same value? |
|-------------------------|--------------------------------|-------------|
| `user_id` | `user_id` | yes |
| `group_id` | `group_id` | yes |
| `org_id` | `organization_id` | yes |
| `ws_id` | `workspace_id` | yes |
| `org` | `organization` | yes (name) |
| `ws` | `workspace` | yes (subdomain) |
| `group` | `group` | yes (name) |
| `login_id` | `login_id` | yes |
| `user` | `full_name` | yes (name) |

The URL parameters are terse (URLs favor brevity); the CSV columns are verbose
(spreadsheets favor readability). The values match — only the labels differ, and
only for `org_id`/`ws_id`.

---

## Worked example

Suppose a survey was launched with the **De-identified hex IDs** scheme, so the
research team's stored data contains:

```
user_id  = 68f138c495cdf54a392b20aa
group_id = 68049e383adb985c4a637183
org_id   = 68041da03916f91f24b1ec78
ws_id    = 695f5a3fa323f290a63b3fce
```

To resolve who that is, an authorized person runs the Members Report for that
workspace, downloads the CSV, and finds the row where `user_id` equals
`68f138c495cdf54a392b20aa`:

```
workspace      = dev
workspace_id   = 695f5a3fa323f290a63b3fce
user_id        = 68f138c495cdf54a392b20aa
full_name      = Adrian Cole
login_id       = acole@students.example.org
email          = acole@students.example.org
organization   = Hillsdale Middle School
organization_id= 68041da03916f91f24b1ec78
group          = Dale's Fun Science
group_id       = 68049e383adb985c4a637183
leaders        = Dale Musser
status         = active
```

The de-identified `org_id` (`68041da0…`) matches the report's
`organization_id`; the `ws_id` (`695f5a3f…`) matches `workspace_id`. The student
is **Adrian Cole**, in **Dale's Fun Science** at **Hillsdale Middle School**,
taught by **Dale Musser**.

For an analysis dataset, you'd typically join your hex-keyed table to this CSV on
`user_id` (and/or `group_id`) to attach the human-readable columns.

---

## This report is identifiable data — handle it accordingly

The Members Report CSV contains direct PII (names, logins, emails) and **is the
re-identification key** for otherwise de-identified data. Treat it as the most
sensitive artifact in this system:

- Keep it on the authorized/StrataHub side. Do **not** hand it to a party that is
  only supposed to hold de-identified data — that would defeat the
  de-identification.
- Share or store it only as permitted by your data-handling agreements
  (FERPA / COPPA / IRB).
- When you only need de-identified analysis, you do not need this report at all —
  work from the hex IDs alone.

---

## See also

- **`data-consumer-guide.md`** — for the party receiving the URL parameters (the
  de-identified side).
- **`vocabulary.md`** — the permanent definition of each identifier.
