# Member Status API + MHS Dashboard Survey Status — Implementation Plan

_Drafted 2026-08-25; decisions confirmed the same day. Status: approved design, implementation not started. Task 0 (settings-save bug) goes first._

## 1. Goal

Two related deliverables:

1. **Inbound web service.** An external data provider (Abt, for the impact-study
   surveys) calls StrataHub to report that a member (student) has **started** or
   **completed** a named **entity** (a survey). The call carries the member's
   `user_id` (24-char hex ObjectID), the entity name, and the state. StrataHub
   records the state and the date/time it was received.
2. **MHS Dashboard display.** Leaders (teachers) see, per student, the status of
   each survey — *Not started / Started / Completed* — with the timestamps
   available on the entry.

The number, names, and dashboard order of the surveys must be easy to change.
Current names (provisional): `Pre`, `MHS Engagement`, `EWS Engagement`, `Post`.

The API is deliberately generic (member + entity + state) so it can carry other
externally-tracked activities later; only the dashboard presentation is
survey-specific.

## 2. How it fits StrataHub (findings that shape the design)

| Fact | Consequence |
|------|-------------|
| Workspace is resolved from the request `Host` by `workspace.Middleware` (`internal/app/system/workspace/workspace.go:50`); `workspace.IDFromRequest(r)` gives the ID, `NilObjectID` on apex. | The provider calls the **workspace host** (e.g. `https://mhs.adroit.games/api/member-status`). No key→workspace lookup needed. Apex → 400 JSON. |
| Per-workspace admin settings live in `site_settings` (`models.SiteSettings`), edited at `/settings` (admin + superadmin; menu label "Settings", title "Workspace Settings"). The MHS section there already holds `MHSMemberAuthKeyword`, `EnableClaudeSummaries`, `ClaudeModel`. | The shared key goes in `SiteSettings` and is edited on `/settings`, in the existing MHS section. |
| `settingsstore.Save` `$set`s a fixed whitelist of fields from the struct it is handed. `/settings` (`settings/admin.go:298-311`) and `/workspaces/{id}/settings` (`workspaces/settings.go:221-229`) both build a **fresh** struct from form fields, so each save blanks fields the form doesn't carry (today: `/settings` blanks `mhs_active_collection_id`; the apex page blanks nearly everything MHS-related). | **Pre-existing bug.** Must be fixed before adding the key, or the key will be wiped by the next unrelated settings save. See Task 0. |
| `gorilla/csrf` is installed globally (`bootstrap/routes.go:252`) before every mount; no exemption mechanism exists yet. Sub-router middleware runs *after* it. | Add a path-scoped `csrf.UnsafeSkipCheck` middleware immediately **before** `r.Use(csrfMiddleware)`. |
| Maintenance middleware (`system/maintenance/maintenance.go`) returns an HTML 503 to everything not in its exempt list, including `/api/*`. | Add `/api/member-status` to `isExemptPath` so the provider can keep reporting during maintenance (the endpoint is key-gated anyway). |
| `RedirectNonSuperadminFromApex` already exempts `/api/`. | Nothing to do. |
| No inbound API-key auth exists in stratahub. The only constant-time compare is `missionhydrosci/api_manifest.go:192` (`subtle.ConstantTimeCompare`), fronted by a backoff throttle. stratalog's `auth.APIKeyAuth` is the sibling-service pattern (Bearer token). Waffle's `pantry/auth/apikey.Require` takes a static key at construction and uses `!=`, so it can't serve a per-workspace DB-loaded key. | Write a small feature-local middleware: `Authorization: Bearer <key>` (accept `X-API-Key` too), load the workspace's key per request, `subtle.ConstantTimeCompare`, per-IP failure throttle via `internal/app/system/ratelimit`. |
| Dashboard columns are driven by an embedded JSON (`internal/app/resources/mhs_progress_points.json`, loaded once by `mhsdashboard.LoadProgressConfig()`), embedded via `resources.go:16`. | Survey definitions follow the same pattern: `mhs_member_status.json` + `LoadMemberStatusConfig()`. Changing names/count/order = edit JSON, rebuild, deploy. |
| Dashboard keys every per-student lookup on `users._id` hex (`member.ID.Hex()`), batch-loaded per group in `buildProgressRows` (`dashboard.go:740`), which serves both the full page and the HTMX `/grid` refresh. Per-student data stores use `{workspace_id, user_id}` upserts (`mhsdevicestatus.Upsert` is the cleanest template). | New store `memberstatus` keyed `{workspace_id, user_id, entity_key}`; one `ListByUserIDs` call added to `buildProgressRows` feeds both render paths and the 30-second auto-refresh for free. |
| The dashboard's tab system (`switchTab`, per-tab legend `#mhs-legend-<tab>`, per-tab panel in `mhsdashboard_grid.gohtml`) and the Devices tab's plain `<table>` with dot glyphs + `title="…"` timestamps are the closest visual precedent. Status indicators must not be color-only (6 colorblind themes). | Add a **"Surveys" tab** (title from config) modeled on the Devices tab: glyph + word + short date per cell, full timestamps in `title`, click → modal with details. |
| In `abt-deidentified` mode the `__userid` Abt receives **is** `users._id` hex, i.e. exactly the key the API expects. In `abt-identifiable` mode it is the login email. | The API takes `user_id` hex (per the spec and the vocabulary contract). If Abt ends up in identifiable mode, a `login_id` alternative would have to be added (workspace-scoped, and not unique across auth methods) — see Open Questions. |

## 3. Design

### 3.1 API contract (provider-facing)

Base: the workspace host, e.g. `https://mhs.adroit.games`.

**Auth.** Server-to-server over HTTPS; no cookies, no login, no session. The provider's server includes the workspace's shared key **as a string field in the JSON body** (`"key"`). The key is per workspace, set by an admin on the Settings page. Missing/invalid → `401` JSON. No key configured for the workspace → `401` with `"error":"not_configured"`. Repeated failures from one IP are throttled (`429`). (An `Authorization: Bearer <key>` header is accepted as an undocumented alternative; the body field is the contract.)

**`POST /api/member-status`** — report one status event.

```json
{
  "key":         "ms_…",
  "user_id":     "68f138c495cdf54a392b20aa",
  "entity":      "MHS Engagement",
  "state":       "completed",
  "occurred_at": "2026-08-25T14:03:11Z"
}
```

| Field | Required | Rules |
|-------|----------|-------|
| `key` | yes | the workspace's shared key; compared in constant time; never echoed or logged |
| `user_id` | yes | 24-char hex ObjectID; must be an active `member` in this workspace → else `404 unknown_user` |
| `entity` | yes | trimmed, 1–100 chars; matched case-insensitively against configured names; **unknown names are still stored** (so a rename on the provider side never loses data) and flagged `known_entity:false` in the response + a Warn log |
| `state` | yes | `started` or `completed` (case-insensitive) → else `400 invalid_state` |
| `occurred_at` | no | RFC 3339; the provider's own timestamp. StrataHub always records its own receipt time separately |

Response `200`:

```json
{
  "ok": true,
  "user_id": "68f138c495cdf54a392b20aa",
  "entity": "MHS Engagement",
  "known_entity": true,
  "state": "completed",
  "started_at": "2026-08-25T13:41:02Z",
  "completed_at": "2026-08-25T14:03:14Z"
}
```

Errors are JSON `{"ok":false,"error":"<code>","message":"…"}` with codes `bad_json`, `invalid_user_id`, `unknown_user`, `invalid_entity`, `invalid_state`, `unauthorized`, `not_configured`, `workspace_required`, `rate_limited`. Body limit 8 KB.

**Semantics (idempotent, monotonic).** The state ladder is `not_started → opened → started → completed`. `opened` is StrataHub's own signal (the student launched the linked resource, §3.6); the provider reports only `started` and `completed`.
- `started` → sets `started_at` if unset; raises `state` to `started` unless already `completed`.
- `completed` → sets `completed_at` if unset; sets `state=completed`. `started_at` is left unset if never reported (never fabricated).
- Repeats of the same state are accepted (200) and only bump `last_received_at`; timestamps are first-wins. A lower state arriving after a higher one never regresses the state.
- Every accepted event is appended to a bounded `history` array on the document (last 25) for the dashboard's detail view.

**`POST /api/member-status/ping`** — same auth (`{"key":"…"}` body); returns `{"ok":true,"workspace":"mhs","entities":["Pre","MHS Engagement","EWS Engagement","Post"]}` so the provider can verify connectivity, the key, and the exact entity names in use.

Not in v1 (easy later): batch submission (`{"events":[…]}`), a `GET` to read a member's statuses back, `login_id` as an alternative identifier.

### 3.2 Storage

New collection **`member_status`** (`internal/domain/models/memberstatus.go`, store `internal/app/store/memberstatus/`):

```go
type MemberStatus struct {
    ID             primitive.ObjectID `bson:"_id,omitempty"`
    WorkspaceID    primitive.ObjectID `bson:"workspace_id"`
    UserID         primitive.ObjectID `bson:"user_id"`
    Entity         string             `bson:"entity"`      // name as first received / config title (display)
    EntityKey      string             `bson:"entity_key"`  // canonical key, see below
    State          string             `bson:"state"`       // "opened" | "started" | "completed"
    OpenedAt       *time.Time         `bson:"opened_at,omitempty"`    // first launch of the linked resource (StrataHub)
    StartedAt      *time.Time         `bson:"started_at,omitempty"`   // receipt time of first "started" (provider)
    CompletedAt    *time.Time         `bson:"completed_at,omitempty"` // receipt time of first "completed" (provider)
    Source         string             `bson:"source"`      // "api" | "launch" — who wrote the latest event
    FirstReceivedAt time.Time         `bson:"first_received_at"`
    LastReceivedAt  time.Time         `bson:"last_received_at"`
    History        []MemberStatusEvent `bson:"history"`    // capped at 25 via $push/$each/$slice
    CreatedAt      time.Time          `bson:"created_at"`
    UpdatedAt      time.Time          `bson:"updated_at"`
}
type MemberStatusEvent struct {
    State      string     `bson:"state"`
    ReceivedAt time.Time  `bson:"received_at"`
    OccurredAt *time.Time `bson:"occurred_at,omitempty"`
    RemoteIP   string     `bson:"remote_ip,omitempty"`
}
```

**Canonical `entity_key`.** Both writers resolve to the same key so the dashboard reads one document per (student, survey): if the name the provider sends matches a configured item's `api_names` (case-insensitive) → `entity_key = item.id` (e.g. `pre`); a resource launch writes `item.id` directly; an unrecognized provider name → `entity_key = "name:" + text.Fold(name)` so it is stored, flagged, and never collides with an item id. When an alias is added to the config later, the dashboard also checks the folded-name key as a fallback so earlier events surface without a migration.

Store methods: `Record(ctx, wsID, userID, entityKey, displayName, state, source, occurredAt *time.Time, remoteIP string) (MemberStatus, error)` — a single `FindOneAndUpdate` upsert with `$setOnInsert` for the first-wins timestamps, a state raise that never regresses, `$push`/`$each`/`$slice` for history, plus the duplicate-key retry from `mhsdevicestatus.Upsert`; `ListByUserIDs(ctx, wsID, userIDs) (map[userHex]map[entityKey]MemberStatus, error)`; `ListForUser(ctx, wsID, userID)`. Indexes (in `system/indexes/indexes.go`, new `ensureMemberStatus`): unique `{workspace_id, user_id, entity_key}`; `{workspace_id, entity_key, state}` for future roll-ups. Only `$match/$sort/$in` are used — DocumentDB-safe; `$push` with `$each`+`$slice` is supported on DocumentDB.

### 3.3 Shared key (workspace settings)

- `SiteSettings.MemberStatusAPIKey string \`bson:"member_status_api_key,omitempty" json:"-"\`` + `MemberStatusAPIKeySetAt *time.Time`.
- Stored **in the clear**, displayed masked. Rationale: the key guards a write-only endpoint whose worst-case abuse is falsifying survey status; admins need to re-copy it to send to the provider; the same document already stores `mhs_member_auth_keyword` the same way. (Hashed storage with show-once, as stratalog's `apikeys` feature does, is a drop-in swap if wanted.)
- UI on `/settings`, MHS section, under a "Member Status API" heading: masked input with Show/Hide + Copy, a **Generate** button (client-side `crypto.getRandomValues`, 32 bytes → base64url, prefixed `ms_` so it is recognizable in logs), "Set on <date>" caption, and a one-line note of the endpoint URL for this workspace. Empty = API disabled.
- Store: add to `settingsstore.Save`'s `$set` whitelist **and** add a narrow `SetMemberStatusAPIKey(ctx, wsID, key)`; Task 0 makes the two settings handlers non-destructive.
- Audit: new `audit.EventMemberStatusKeyChanged`, logged via a new `auditlog.Logger` wrapper when the key changes/clears (the settings handler doesn't hold an `AuditLog` today — add it in `routes.go:571`).

### 3.4 Dashboard

Config file `internal/app/resources/mhs_member_status.json` (embedded alongside `mhs_progress_points.json`):

```json
{
  "tab_title": "Surveys",
  "items": [
    { "id": "pre",  "title": "Pre-Survey",     "short_name": "Pre",  "api_names": ["Pre"],            "description": "Taken before Unit 1." },
    { "id": "mhs",  "title": "MHS Engagement", "short_name": "MHS",  "api_names": ["MHS Engagement"], "description": "…" },
    { "id": "ews",  "title": "EWS Engagement", "short_name": "EWS",  "api_names": ["EWS Engagement"], "description": "…" },
    { "id": "post", "title": "Post-Survey",    "short_name": "Post", "api_names": ["Post"],           "description": "Taken after the final unit." }
  ]
}
```

Order in the file = column order. `api_names` lists every name the provider may send for that column (so a rename is one edit; old data keeps matching). `id` is stable and internal.

Presentation — a new **Surveys tab** (title from `tab_title`):
- Tab button + `switchTab` maps + legend row, following the Devices tab exactly.
- Panel = `<table>`: `Student | <one column per item>`; cell shows a glyph + word + short date: `○ Not started`, `◔ Opened · Aug 25`, `◐ Started · Aug 25`, `✓ Completed · Aug 25`. Glyph carries the meaning; color is secondary (works in all 6 themes and dark mode via the existing `.dark #mhs-dashboard …` overrides).
- `title="Opened Aug 25, 2026 1:39 PM CDT · Started 1:41 PM · Completed 2:03 PM"` using the org's time zone (`formatTimeInOrgTimezone` precedent).
- Click a cell → modal (clone of `#mhs-review-modal`) with student, survey, all timestamps, and the recent history.
- Data plumbing: `SurveyHeader` + `SurveyCell` types; `SurveyHeaders` on **both** `DashboardData` and `GridData`; `Surveys []SurveyCell` on `MemberRow`; one `memberstatus.ListByUserIDs` call in `buildProgressRows` (graceful empty map on error, like grades), matched by `item.id` with the folded-name fallback. Unknown entity names from the API are stored but not shown unless added to the config.
- The config loader lives in a small neutral package, `internal/app/system/memberstatuscfg` (`Load()`, `Items()`, `Resolve(name) (item, ok)`), because three features need it — the dashboard, the API, and the resource forms — and `resources` must not import `mhsdashboard`.
- Optional (cheap): append a "Surveys: …" line to the AI summary digest in `summary.go:154` so Claude summaries can mention survey completion.

This is a third data source for the dashboard (StrataHub DB, alongside mhsgrader and stratalog) — a deliberate departure from the 2026-03 "mhsgrader is the single source" note, recorded in `docs/mhsdashboard-planning031226.md` as part of Task 6.

### 3.5 Security posture

- Key compared with `subtle.ConstantTimeCompare`; failures logged at Warn with IP and path; per-IP throttle (e.g. 20 failed auths / 5 min) using `internal/app/system/ratelimit`.
- Endpoint is write-only and returns no PII (only echoes the IDs/timestamps it was given). `known_entity`/`unknown_user` responses do not reveal names.
- Body limit 8 KB, `middleware.Timeout(timeouts.Short())`, JSON only.
- **Server-to-server over HTTPS** (confirmed). The key travels in the POST body, which TLS protects in transit; it never appears in a URL, so it never lands in access logs or proxies. No CORS, no cookies, no session — the endpoint ignores `LoadSessionUser` state entirely.
- The key is never written to logs, audit details, or responses; auth failures log only IP, path, and reason.

### 3.6 Linking a survey resource to a tracked survey ("Opened")

A survey is a `models.Resource` with `Type == "survey"` and nothing else tying it to Pre/MHS/EWS/Post. To let StrataHub record that a student **opened** a survey (before the provider reports `started`), each resource gets an optional link to one configured item:

- `Resource.TrackedEntityID string \`bson:"tracked_entity_id,omitempty"\`` — the config item `id` (`pre`, `mhs`, …); empty = not tracked.
- Resource **new/edit** forms: a "Survey tracking" `<select>` — *None* plus one option per configured item (title) — following the `URLIdentityMode` precedent exactly: options built in `admincommon.go`, read/validated/saved in `adminnew.go` and `adminedit.go` (validation = must be empty or a configured id), label shown on `adminview.go` and the list. Store `Create`/`Update` carry the field. Only meaningful for URL resources; the form hides the select when a file is chosen.
- **Launch hook:** in the two member launch paths in `resources/memberview.go` (where `RecordResourceLaunch` already fires), if `res.TrackedEntityID != ""` call `memberstatus.Record(ws, user, item.id, item.title, "opened", "launch", nil, "")`. First-wins on `opened_at`; a later launch never changes anything except history. Failure to record is logged and never blocks the launch redirect.
- Several resources may point at the same item (e.g. two link variants for the same survey); they all raise the same document.
- Renaming or reordering surveys in the JSON does not touch resources; removing an item leaves resources pointing at an id the select no longer offers — the edit form shows it as "(no longer configured)" and the launch hook skips it.

## 4. Work breakdown

Each task is independently committable; order matters only where noted.

**Task 0 — Make SiteSettings saves non-destructive (prerequisite, bug fix — first, and shipped on its own).** ✅ Done: commit `ca323ce`, deployed to production 2026-08-26, verified (a Settings save no longer clears the active MHS collection).
`settings/admin.go HandleSettings` and `workspaces/settings.go HandleSettings`: load `current` (already done for the logo), copy it, overlay only the fields the form carries, then `Save`. Tests: saving from each page preserves `mhs_active_collection_id`, Claude settings, MHS member-auth settings, and landing content. (Also fixes the apex page wiping the workspace logo via `workspacestore.Update` — same class; one-line carry-forward.) Committed and deployable independently of everything below.

**Task 1 — Model, store, indexes.** ✅ Done 2026-08-26 (uncommitted at time of writing).
`models/memberstatus.go`, `store/memberstatus/store.go` (+ `_test.go` against local Mongo: first-wins timestamps, no regression from completed, history cap, concurrent upsert), `indexes.go: ensureMemberStatus` wired into `EnsureAll`. Implementation note: the state ladder is stored as `state_rank` (raised with `$max`) alongside the `state` label, which is re-synced with a rank-conditioned `$set` — no update pipelines, DocumentDB-safe. `Record` takes a `RecordInput`; `Get`, `ListForUser`, `ListByUserIDs`, `DeleteByUser` are the readers (`DeleteByUser` is provided for parity with `mhsdevicestatus` but, like it, is not yet wired into member deletion).

**Task 2 — Settings: shared key.** ✅ Done 2026-08-26.
`SiteSettings.MemberStatusAPIKey` + `MemberStatusAPIKeySetAt`; `settingsstore.Save` whitelist + `SetMemberStatusAPIKey` setter; `/settings` form field `member_status_api_key` (masked input with Show/Copy/Generate, "Set on" caption, endpoint URL for the workspace), validated 16–128 chars/no whitespace, empty = disabled; set-at refreshed and `audit.EventMemberStatusKeyChanged` logged only when the key actually changes (the form round-trips the current key). The settings handler now takes an `*auditlog.Logger`. Tests: handler set/round-trip/clear, validation, survival across unrelated saves, endpoint URL, plus an in-package render test that boots the real template engine and executes `settings.gohtml` with and without a key.

**Task 3 — Member Status API feature.** ✅ Done 2026-08-26.
`internal/app/features/memberstatusapi/`: `handler.go` (auth + both endpoints; deps: settings/users/memberstatus stores, `memberstatuscfg`, failed-auth limiter), `routes.go` (`MountRoutes` + `CSRFExempt`), `json.go`, `types.go`, `handler_test.go` (15 cases: apex/no workspace, not configured, missing/wrong key, bad JSON, Bearer alternative, throttle + reset on success, ping, field validation incl. `opened` rejected from the provider, wrong-workspace/leader/disabled users → 404, started→completed→late started, unknown entity stored + flagged, one doc per entity, 405, CSRF exemption proven against real `csrf.Protect`). Bootstrap: `r.Use(memberstatusapifeature.CSRFExempt)` immediately before `csrf.Protect`; `/api/member-status` added to the maintenance exemptions; mounted on the root router next to the other `/api/*` routes. **Pulled forward from Task 4:** `internal/app/resources/mhs_member_status.json` (embedded) and `internal/app/system/memberstatuscfg` (`Load`/`Parse`/`Resolve`/`KeyFor`/`Find`/`APINames`, with tests), because the API needs name → key resolution. Verified end-to-end against the running dev server with curl: auth failures, ping, the state ladder, unknown entity/user, GET → 405, and ping succeeding during maintenance mode while `/dashboard` returned 503.

**Task 4 — Dashboard config + data.** ✅ Done 2026-08-26. (Config file and `memberstatuscfg` package were delivered under Task 3.)
`mhsdashboard/surveys.go`: `surveyTabTitle`/`surveyHeaders` from the config, `loadSurveyCells` (one `memberstatus.ListByUserIDs` per group, matched by item id with the unknown-name-key fallback), and the pure `buildSurveyCell` formatter (glyph + label + CSS class per state, short date and full timestamps in the org's zone, ladder-ordered tooltip). Types: `SurveyHeader`, `SurveyCell`; `MemberRow.Surveys`; `SurveyTabTitle`/`SurveyHeaders` on `DashboardData` and `GridData` (including the empty-group render). `Handler` gains `MemberStatusStore` and `SurveyConfig` (a bad config is logged and hides the tab rather than failing the dashboard). `formatTimeInOrgTimezone` now also returns the `*time.Location`, split out as `orgLocation`, and `buildProgressRows` takes it so survey timestamps use the same zone as "Last updated". Tests: `surveys_test.go` (cell formatting incl. America/Chicago, fallback key matching, headers from a parsed and the embedded config, Mongo-backed `loadSurveyCells` with workspace isolation). Templates untouched — Task 5 renders these fields.

**Task 5 — Dashboard UI.**
Tab button, `switchTab` maps, legend (four states), panel table, cell glyph/text/date, `title` tooltips, detail modal + delegated click handler, dark-mode check across the 6 themes, name-column `iconSpace` untouched (no new icon in the name column). Optional AI-summary line.

**Task 5b — Resource link + "Opened" hook (§3.6).**
`Resource.TrackedEntityID`; resource store create/update; `admincommon.go` options; `adminnew.go`/`adminedit.go` read + validate + save; `adminview.go`/`adminlist.go` label; `resources_new.gohtml`/`resources_edit.gohtml`/`resources_view.gohtml` select + label; launch hook in `memberview.go` (both launch paths) writing `opened` via `memberstatus.Record`, non-blocking on error; handler tests for validation and for the hook firing only on tracked resources. Depends on Tasks 1 and 4.

**Task 6 — Docs & context.**
`docs/member-status-api/provider-guide.md` (for Abt: endpoint, auth header, payload, entity names, error codes, curl examples — placeholder hostnames only, never real survey URLs), `docs/member-status-api/admin-guide.md` (set/rotate key, change survey names via JSON, link a survey resource to a tracked survey), note in `docs/mhsdashboard-planning031226.md`, update `ai/context.md` (feature + store tables), `docs/docs_index.md`.

**Task 7 — Verification & rollout.**
`go build ./... && go vet ./... && make test-safe`; `make css-prod` (new Tailwind classes in templates); run locally, exercise with curl against `localhost:8080` (single-workspace mode) and check the tab in light/dark; deploy via `stratahub_update/aws_update.sh`; set the key on the production workspace; send Abt the provider guide + key out of band; confirm with a `ping` and one test event against a test member.

Rough effort: Task 0 ≈ 2 h; Tasks 1–3 ≈ 1 day; Tasks 4–5 ≈ 1 day; Task 5b ≈ ½ day; Tasks 6–7 ≈ ½ day.

## 5. Decisions (confirmed 2026-08-25)

1. **Task 0 first.** The destructive settings-save bug is fixed and shipped before any of the new work lands.
2. **Caller.** Server-to-server over HTTPS. The provider's server has no cookie and never logs in; it authenticates by including the shared key as a string in the posted JSON (`"key"`).
3. **Survey config.** Embedded JSON (`mhs_member_status.json`); no admin-editable list.
4. **"Opened" is in scope**, not v2 — implemented as the resource link in §3.6 (`Resource.TrackedEntityID` + select on the resource forms + launch hook).

Still assumed (no objection raised): `user_id` hex is the only identifier; plaintext-masked key storage; unknown entity names are stored and flagged rather than rejected.

5. **Entity strings.** The provider sends exactly `Pre`, `MHS Engagement`, `EWS Engagement`, `Post` for now; these are the `api_names` in `mhs_member_status.json`. If Abt changes a name, the change is one edit to that file (add the new name as an alias; keep the old one so earlier events still match).
6. **States.** Abt sends both `started` and `completed` (confirmed), so all four dashboard states are live: Opened (StrataHub launch) → Started (provider) → Completed (provider).
7. **Config package.** `internal/app/system/memberstatuscfg` is the single loader for the survey list, shared by the dashboard, the API, and the resource forms.
