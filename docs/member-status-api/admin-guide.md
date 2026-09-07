# Member Status API & Surveys Tab — Guide for Admins

_Audience: StrataHub admins and coordinators running a workspace whose survey
provider reports survey status to StrataHub, and leaders reading the result on
the MHS Dashboard._

StrataHub can show each student's survey status — not started, opened,
started, completed — on the MHS Dashboard. Two things feed it:

- **The survey provider** (Abt) calls StrataHub's Member Status API when a
  student starts or completes a survey. The API is protected by a shared key
  you set on the workspace's Settings page.
- **StrataHub itself** records that a student *opened* a survey when they
  launch a survey resource you have linked to that survey.

This guide covers setting the key, linking survey resources, reading the tab,
and changing the survey list.

---

## 1. Enable the API: set the shared key

Settings → **Member Status API — Shared Key** (in the Mission HydroSci section).

1. Click **Generate** to create a strong key (or paste a key the provider gave
   you — 16 to 128 characters, no spaces).
2. Click **Save**. The key takes effect immediately.
3. Click **Show** and **Copy** to hand the key to the provider, along with the
   endpoint shown on the same page
   (`POST https://<workspace-host>/api/member-status`). Send it through a
   private channel, not email in the clear if you can avoid it.

Give the provider the [Provider Guide](provider-guide.md); it has everything
their developers need, including a `ping` call to confirm the key and the
survey names.

**Rotating the key:** Generate a new one and Save; the old key stops working
at once, so coordinate the switch with the provider.

**Disabling the API:** clear the field and Save. Requests are then refused with
`not_configured` until a key is set again.

Each key change is written to the workspace **Audit Log** (event
`member_status_key_changed`, with who changed it and whether it was set or
cleared — never the key itself).

---

## 2. Link each survey resource (for the "Opened" state)

The provider can only tell StrataHub about *started* and *completed*. StrataHub
adds *opened* — the student clicked the survey link — but only if it knows
which resource is which survey.

For each survey resource (Resources → open the resource → **Edit**):

1. Under **Survey Tracking**, choose the survey this resource is
   (Pre, MHS Engagement, EWS Engagement, or Post).
2. Save.

That's all. From then on, when a student launches the resource, the Surveys
tab shows **Opened** with the time — until the provider reports it started or
completed. Several resources may point at the same survey (for example two
link variants); they all count.

The View page for a resource shows its current Survey Tracking selection.

---

## 3. Read the Surveys tab

For the full picture — every state, the ladder rules, the modal, and how an
event received by the API becomes a cell — see the
[Surveys Tab](surveys-tab.md) guide. In short:

MHS Dashboard → **Surveys** tab (leaders, admins, coordinators).

One row per student, one cell per survey:

| Cell | Meaning | Who reported it |
|------|---------|-----------------|
| ○ Not started | Nothing recorded | — |
| ◔ Opened · date | The student opened the survey link in StrataHub | StrataHub |
| ◐ Started · date | The student began the survey | Provider |
| ✓ Completed · date | The student finished the survey | Provider |

- The symbol carries the meaning, so the tab reads correctly in every color
  theme and in dark mode.
- The date is when the current state was reached, in the organization's time
  zone. Hover a cell for every recorded time; **click a cell** for the full
  timeline (opened, started, completed) and which system reported each step.
- The tab refreshes with the rest of the dashboard (every 30 seconds, or with
  the Refresh button).
- A status never goes backwards: once a survey is completed it stays
  completed even if a later "started" or "opened" event arrives.

---

## 4. Change the survey list (names, number, order)

The surveys are defined in one file in the StrataHub source:

`internal/app/resources/mhs_member_status.json`

```json
{
  "tab_title": "Surveys",
  "items": [
    {
      "id": "pre",
      "title": "Pre",
      "short_name": "Pre",
      "api_names": ["Pre"],
      "description": "Survey taken before students begin Mission HydroSci."
    }
  ]
}
```

| Field | Purpose |
|-------|---------|
| `tab_title` | The dashboard tab's label |
| `id` | Stable internal id. **Never change an existing id** — it is the key under which student status is stored and the value saved on linked resources. |
| `title` | Column header on the tab, label in the resource form and modal |
| `short_name` | Compact label (reserved for narrow layouts) |
| `api_names` | Every name the provider may send for this survey. Matching is case-insensitive. |
| `description` | Shown as the column tooltip and in the detail modal |

- **Order** in the file is the column order.
- **Rename on the provider's side:** add the new name to `api_names` and keep
  the old one, so earlier events still match.
- **Add a survey:** add an item with a new `id`. Link its resource (section 2).
- **Remove a survey:** delete the item. Stored status for it is kept but not
  shown; resources still linked to it show "(no longer configured)" on the
  edit form until you change them.

Changes take effect after a rebuild and deploy (the file is embedded in the
binary). A malformed file stops the server at startup with a clear error, so
validate JSON before deploying.

---

## 5. Check that events arrived: the Survey Events view

Menu → **Survey Events** (admins, coordinators, leaders, and analysts; also
linked from the Surveys tab's legend, from a survey's status modal, and from
the Settings page). It lists every event StrataHub received, newest first —
each provider request, accepted or rejected, and each survey link a student
opened — so you (or the provider's developer, with an analyst login) can
confirm an event arrived, was stored, and carried the right content.

![Survey Events after a test run: four accepted events for one student, one of them under an unrecognized survey name, and four rejected requests](images/survey-events-test.png)

_Above: a test workspace after the checks in the rollout to-do
(`scripts/member-status-api-check.sh`) — the four rejected requests need no
real student; the four accepted ones are one test student's `started`,
`completed`, a late `started` that left the status at Completed, and a send
under an unrecognized name._

- **Filters:** received time (last hour / 24 h / 7 d / 30 d / custom), source
  (provider or launch), survey, state sent, result (accepted, rejected, or a
  specific error), student (name or 24-character id), event id, and — for
  roles that span several — organization and group. The address bar follows
  the filters, so a view can be bookmarked or pasted to someone.
- **Rows:** received time (in the student's organization time zone), source,
  student, survey (unrecognized names are flagged), the state as sent, and
  the result — the student's resulting status when accepted, or the error
  code when rejected.
- **▸ Details:** everything stored for that event, including the request
  exactly as it arrived and the stored record as JSON. This is the "is it
  what I sent?" view; the `event_id` at the top matches the one the API
  returned to the provider.
- **Live:** tick the box to refresh every 10 seconds while watching a test
  send arrive. **Export CSV** downloads the current filter.
- You see only what your role may: leaders their groups' students,
  coordinators their organizations, admins and analysts the whole workspace.
  Rejected events with no matching student are visible to admins and
  analysts only.

## 6. Troubleshooting

The quickest way to reproduce a symptom is `scripts/member-status-api-check.sh`
(see the [Check Script](check-script.md) guide); its output names the HTTP
status and error code to look up below.

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Provider gets `401 not_configured` | No key set on this workspace | Set the key (section 1) |
| Provider gets `401 unauthorized` | Key mismatch | Compare with Settings → Show; re-send the key |
| Provider gets `404 unknown_user` | The `user_id` is not an active member of this workspace | Check the student exists, is active, and is a *member* (not a leader); confirm the provider is using the `__userid` from the de-identified links, not an email |
| Provider gets `429 rate_limited` | 20 failed authentications from one address in 5 minutes | Fix the key; the limit clears on the next success or after 5 minutes |
| A survey shows Started/Completed but the name in the provider's data differs | Name not in `api_names` | Add it (section 4). Events sent under the unknown name were stored and will appear once the name is configured |
| Opened never appears | Resource not linked | Set Survey Tracking on the resource (section 2) |
| The server log shows `member status recorded for unrecognized entity name` | Provider sent a name not in the configuration | Same as above — add the alias |

Every accepted event is logged (workspace, student id, survey key, state, and
the provider's IP address). Rejected `unknown_user` requests are logged as
warnings (`member status: unknown user`) with the reason — no such id, member
in a different workspace, or member not active — which the provider's uniform
404 does not reveal. The shared key is never logged.

In addition, every request that passes authentication — accepted or rejected
— and every tracked survey launch is recorded in the **event log**
(`member_status_log`, kept for 400 days): the request exactly as sent, what
it resolved to (student, organization, survey, state), and the outcome. The
API returns each record's `event_id` to the provider, so they can quote it
when asking about a specific send. Requests with a wrong or missing key are
not in the event log (they are throttled and appear only in the server log).
