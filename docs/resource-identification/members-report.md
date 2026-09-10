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
The same report can also be exported **de-identified** (hex IDs only), which is
how you give a party on the de-identified side a roster of those identifiers
without any names.

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

## Choosing what the export carries

The page has an **Identity in export** selector, next to **Member status**. It
decides which identity columns the CSV contains; the rows (who is in the file)
are the same whichever you pick.

| Selection | Columns | Contains PII? | Use it for |
|-----------|---------|---------------|------------|
| **De-identified — hex IDs only** | `workspace_id`, `user_id`, `organization_id`, `group_id`, `status` | No | A roster for a party that holds de-identified data — for example the respondent-ID list a survey provider preloads. This is the one export from this report that can go to the de-identified side. |
| **Identified — names, logins, emails** | `workspace`, `full_name`, `login_id`, `email`, `organization`, `group`, `leaders`, `status` | **Yes** | A human-readable roster with no join keys, such as a class list. |
| **Both — hex IDs and names** *(default)* | All twelve columns | **Yes** | The crosswalk: resolving hex IDs back to people. |

The selection stays put while you change organization, group, or status; it is
sent with the download; and it is written into the offered filename
(`..._deidentified_...`, `..._identified_...`) so a file's contents are
recognizable by name. The summary panel lists the exact columns the file will
contain, with a green note (safe to share with the de-identified side) or an
amber note (contains PII).

The column sets mirror the resource URL identity schemes: **De-identified** is
the same four hex IDs a **De-identified hex IDs** launch URL sends, and
**Identified** the same names a **Human-readable** URL sends. A de-identified
export and a de-identified launch URL therefore speak the same identifiers.

On the export URL the selection is the `identity` parameter
(`deidentified`, `identified`, or `both`; anything else, or nothing, means
`both`).

---

## What the CSV provides

The full (**Both**) export has these columns; the other two selections keep the
subset shown in the last column, in the same order.

| Column | Meaning | Type | In export |
|--------|---------|------|-----------|
| `workspace` | Workspace subdomain | name | Identified, Both |
| `workspace_id` | Workspace ObjectID | **hex** | De-identified, Both |
| `user_id` | Member ObjectID | **hex** | De-identified, Both |
| `full_name` | Member's full name | PII | Identified, Both |
| `login_id` | Member's login | PII | Identified, Both |
| `email` | Member's email | PII | Identified, Both |
| `organization` | Organization name | name | Identified, Both |
| `organization_id` | Organization ObjectID | **hex** | De-identified, Both |
| `group` | Group name | name | Identified, Both |
| `group_id` | Group ObjectID | **hex** | De-identified, Both |
| `leaders` | Teacher(s) for the group, by name, `|`-separated | PII | Identified, Both |
| `status` | `active` / `disabled` | — | all |

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
workspace, downloads the CSV with **Identity in export** left on **Both**, and
finds the row where `user_id` equals
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

The **Both** export contains direct PII (names, logins, emails) next to the hex
IDs and **is the re-identification key** for otherwise de-identified data; the
**Identified** export contains the same PII without the keys. Treat both as the
most sensitive artifacts in this system:

- Keep it on the authorized/StrataHub side. Do **not** hand it to a party that is
  only supposed to hold de-identified data — that would defeat the
  de-identification.
- Share or store it only as permitted by your data-handling agreements
  (FERPA / COPPA / IRB).
- When you only need de-identified analysis, work from the hex IDs alone: use
  the **De-identified** export, which carries no names, logins, or emails and is
  the one file from this report that may be handed to the de-identified side.

---

## See also

- **`data-consumer-guide.md`** — for the party receiving the URL parameters (the
  de-identified side).
- **`vocabulary.md`** — the permanent definition of each identifier.
