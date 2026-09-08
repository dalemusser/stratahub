# Member Status API — Check Script

_Audience: whoever confirms the Member Status API on a workspace — the admin
running the rollout, and the developer supporting the integration. The
script is `scripts/member-status-api-check.sh`._

The script exercises the API from the outside, the way the survey provider's
system does: it sends real HTTPS requests to a workspace and compares each
response with what the [Provider Guide](provider-guide.md) promises. It
prints one `PASS` or `FAIL` line per check and finishes with the `event_id`
of every request that was logged, so the same run can be inspected in
**Survey Events**. It replaces the by-hand curl commands in
[Rollout To-Do §5](rollout-todo.md#5-verify-end-to-end-curl), which remain
there as the manual version of the same checks.

---

## 1. Requirements

- `bash` and `curl` (macOS and Linux as shipped; nothing else is used).
- Network access to the workspace host over HTTPS.
- The workspace's shared key: Settings → **Member Status API — Shared Key**
  → Show / Copy. It must have been **saved**; a generated key that was never
  saved does not exist on the server, and every keyed request answers
  `not_configured`.
- For the student checks, the 24-character hex `user_id` of a **test
  student** on that workspace (Members Report CSV, or the student's row in
  Survey Events).

---

## 2. Running it

```bash
MEMBER_STATUS_KEY='<KEY>' scripts/member-status-api-check.sh https://<workspace-host> [<user_id>] [<survey>]

# with the browser (CORS) checks as well:
MEMBER_STATUS_KEY='<KEY>' MEMBER_STATUS_ORIGIN='https://<provider-origin>' \
  scripts/member-status-api-check.sh https://<workspace-host>
```

| Input | Required | Meaning |
|-------|----------|---------|
| `MEMBER_STATUS_KEY` (environment) | yes | The workspace's shared key. It is read from the environment so it never appears in the script, in the command line, or in shell history. Set it on the same line as the command, as above, rather than exporting it. |
| `MEMBER_STATUS_ORIGIN` (environment) | no | An origin listed under Settings → **Member Status API** → **Allowed browser origins** (scheme and host, no path). When set, three browser/CORS checks run as well. |
| `<workspace-host>` | yes | The workspace URL, e.g. the Dev MHS workspace. A trailing slash is tolerated. |
| `<user_id>` | no | A test student's hex id. Without it the script runs only the checks that change nothing. |
| `<survey>` | no | The survey name to use, exactly as the provider sends it (default `Pre`). Pick one the student has not completed so the started → completed ladder is visible. |

Exit status: `0` when every check passed, `1` when any failed, `2` when the
arguments were wrong (the usage text is printed).

**Side effects.** The first group of checks records nothing. The student
checks record survey status for that one student, and the run leaves rows in
Survey Events like any provider traffic. One request with a deliberately
wrong key is sent; it counts as a single failed authentication toward the
per-address throttle (20 in 5 minutes), and the next successful request
clears the count.

---

## 3. What it checks

**Without a student** — eleven checks, nothing recorded:

| Check | Request | Expected |
|-------|---------|----------|
| health | `GET /health` | `200` |
| ping with the key | `POST /api/member-status/ping` with `"key"` | `200`, `ok: true`; the workspace and its survey names are printed |
| ping, empty body | ping with `{}` | `401 unauthorized` (a missing key is rejected before the workspace is looked at) |
| ping, wrong key | ping with a bad key | `401 unauthorized` |
| ping, key in a Bearer header | `Authorization: Bearer <key>` and `{}` | `200` (the undocumented alternative to the body field) |
| GET on the endpoint | `GET /api/member-status` | `405` |
| non-object JSON body | `[1,2]` | `400 bad_json` |
| unknown student | a well-formed id that belongs to nobody | `404 unknown_user`, and the response carries an `event_id` |
| email instead of hex id | `student@example.org` as `user_id` | `400 invalid_user_id` |
| state `opened` from the provider | `"state":"opened"` | `400 invalid_state` (only StrataHub records Opened) |
| bad `occurred_at` | `"occurred_at":"yesterday"` | `400 invalid_occurred_at` |

**With an origin** — three more, nothing recorded (what a browser does when
the provider's page calls the API):

| Check | Request | Expected |
|-------|---------|----------|
| preflight from the origin allowed | `OPTIONS /api/member-status` with `Origin`, `Access-Control-Request-Method: POST`, `Access-Control-Request-Headers: content-type` | `200` with `Access-Control-Allow-Origin` echoing the origin and `Access-Control-Allow-Methods` including `POST`; the allow-origin, methods, and max-age values are printed |
| preflight from an unlisted origin refused | the same from `https://not-listed.example` | `200` with **no** `Access-Control-Allow-Origin` |
| ping with Origin header echoes the origin | a keyed ping carrying `Origin` | `200`, `ok: true`, the allow-origin header present and **no** `Access-Control-Allow-Credentials` |

**With a student** — four more, on the chosen survey:

| Check | Request | Expected |
|-------|---------|----------|
| started | `started` with `occurred_at` now | `200`, `known_entity: true`, `started_at` set. The resulting `state` is `started`, or `completed` if the student had already completed this survey. |
| completed | `completed` | `200`, `state: completed`, `completed_at` set |
| late Started | `Started` on the survey name in lower case | `200`, `state` still `completed`, `started_at` and `completed_at` **identical** to the previous response — the idempotent, monotonic, case-insensitive contract in one request |
| unrecognized survey name | `entity: "Unrecognized Survey Check"` | `200`, `known_entity: false` — stored and flagged, not rejected |

---

## 4. Reading the output

A complete run against a test workspace looks like this (host and workspace
name replaced):

```
Member Status API check against https://<workspace-host>

PASS  health                                           HTTP 200
PASS  ping with the key                                HTTP 200
      workspace: <workspace>
      entities:  ["Pre","MHS Engagement","EWS Engagement","Post"]
PASS  ping, empty body (missing key)                   HTTP 401 error=unauthorized
PASS  ping, wrong key (one failed auth)                HTTP 401 error=unauthorized
PASS  ping, key in a Bearer header                     HTTP 200
PASS  preflight from https://<provider-origin> allowed HTTP 200
      allow-origin=https://<provider-origin> methods=POST max-age=3600
PASS  preflight from an unlisted origin refused        HTTP 200
PASS  ping with Origin header echoes the origin        HTTP 200
PASS  GET on the endpoint                              HTTP 405
PASS  non-object JSON body                             HTTP 400 error=bad_json
PASS  unknown student (carries an event_id)            HTTP 404 error=unknown_user event_id=6a9e50a7e2ada9cb13ea7f5a
PASS  email instead of hex id                          HTTP 400 error=invalid_user_id event_id=6a9e50a7e2ada9cb13ea7f5b
PASS  state 'opened' from the provider                 HTTP 400 error=invalid_state event_id=6a9e50a8e2ada9cb13ea7f5c
PASS  bad occurred_at                                  HTTP 400 error=invalid_occurred_at event_id=6a9e50a8e2ada9cb13ea7f5d

Student checks for 68f1387595cdf54a392b20a4 on "Post"

PASS  started                                          HTTP 200 event_id=6a9e50a8e2ada9cb13ea7f5e
      state=started started_at=2026-09-07T05:50:32.597Z
PASS  completed                                        HTTP 200 event_id=6a9e50a8e2ada9cb13ea7f60
      state=completed started_at=2026-09-07T05:50:32.597Z completed_at=2026-09-07T05:50:32.912Z
PASS  late 'Started' on 'post': stays completed, same times HTTP 200 event_id=6a9e50a9e2ada9cb13ea7f62
      state=completed started_at=2026-09-07T05:50:32.597Z completed_at=2026-09-07T05:50:32.912Z
PASS  unrecognized survey name: stored and flagged     HTTP 200 event_id=6a9e50a9e2ada9cb13ea7f64

18 passed, 0 failed.
Event ids to look up in Survey Events (▸ Details shows each request as received):
  6a9e50a7e2ada9cb13ea7f5a
  …
```

(Without `MEMBER_STATUS_ORIGIN` the three preflight/origin lines are
replaced by a note that the browser checks were skipped, and the total is
15.) Each line shows the HTTP status, the error code when there is one, and
the `event_id` when the request was logged. The `ping` line also prints the
survey names the workspace expects — the same list the provider should
compare its strings against.

Then look at the same run from the inside:

- **Survey Events** (Menu → Survey Events): the rejected checks appear with
  the raw value that was sent and the error code; the student checks appear
  under the student's name. Paste any id from the list into the **Event id**
  filter, or tick **Live** before running the script to watch rows arrive.
  ▸ Details on a row shows the request exactly as received.
- **MHS Dashboard → Surveys**: the test student shows Completed for the
  chosen survey, and the status modal shows the started and completed receipt
  times from the run.

![Survey Events after a run: four accepted events for one student, one of them under an unrecognized survey name, and four rejected requests](images/survey-events-test.png)

---

## 5. When a check fails

The failing line names the check and prints what came back. The usual
causes, with the fix:

| What you see | Cause | Fix |
|--------------|-------|-----|
| `health` is not `200`, or any line shows `HTTP 000` | Wrong host, DNS, or the service is down | Check the URL; `journalctl -u stratahub` on the server |
| `ping with the key` → `401 error=not_configured` | The workspace has no key saved — usually the key was generated but **Save Settings** was not clicked | Settings → Member Status API → Save; rerun |
| `ping with the key` → `401 error=unauthorized` | The key in `MEMBER_STATUS_KEY` is not the saved one (typo, stray whitespace, rotated since) | Copy it again from Settings |
| Any line → `429 error=rate_limited` | 20 failed authentications from this address in 5 minutes (repeated runs with a bad key) | Wait 5 minutes, or fix the key — one success clears it |
| `started` → `404 error=unknown_user` | The id is not an **active member** of **this** workspace: a student from another workspace, a removed or disabled student, or a leader's id | Use a student from Members Report on the same workspace host |
| `started` passes but `known_entity` is `false` | The third argument is not a configured survey name (the `ping` line lists them) | Use one of the listed names |
| `late 'Started'` fails | The status moved or a timestamp changed after `completed` — a contract regression | Compare the two printed lines; report it with the event ids |
| `preflight from … allowed` fails | That origin is not listed on this workspace, or differs from the listed one (scheme, `www.`, port); or the server predates browser support (2026-09-08) | Settings → Member Status API → Allowed browser origins: add the exact origin, Save, rerun |
| `preflight from an unlisted origin refused` fails | The API allowed an origin it should not have — a regression | Review `memberstatusapi/cors.go` and the workspace's list |
| `ping, empty body` shows `not_configured` instead of `unauthorized` | Not possible with the current handler; would mean the check order changed | Review `memberstatusapi/handler.go` |

Everything the provider might see is in the [Provider Guide's error
table](provider-guide.md#errors); the admin-side symptoms are in the
[Admin Guide §6](admin-guide.md#6-troubleshooting).

---

## 6. Choosing the student and survey

- Use a **test student in a test group**, on the test workspace when there is
  one. The student checks are real status: after the run the student shows
  Completed for that survey on the dashboard, and the store keeps the history.
  There is no undo in the UI.
- Repeating the run on the same student and survey still passes (the
  `started` check accepts an already-completed result), but only a survey the
  student has not completed shows the full started → completed ladder. Pass a
  different survey as the third argument for a clean run.
- After go-live, run the script **without** a student id on the production
  workspace. That confirms the key and the survey names without writing
  anything; the student checks belong on the test workspace.

---

## 7. Where things are

| Thing | Where |
|-------|-------|
| The script | `scripts/member-status-api-check.sh` |
| The manual version of the checks | [Rollout To-Do §5](rollout-todo.md#5-verify-end-to-end-curl) |
| What each response means to the provider | [Provider Guide](provider-guide.md) |
| Key setting, Survey Events view, troubleshooting | [Admin Guide](admin-guide.md) |
| The endpoint's behavior | `internal/app/features/memberstatusapi/handler.go` (`authenticate`, `HandleStatus`); browser calls in `cors.go` and `origins.go` |
