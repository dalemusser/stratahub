# Data Viewers — Framework Plan

_Drafted 2026-08-26. Status: proposed design, not implemented. First viewer:
Survey Events (see `docs/member-status-api/plan.md` §8)._

## 1. Goal

One StrataHub mechanism for presenting event/log/data lists — role-gated,
scoped to what the viewer may see, filterable, pageable, exportable, with a
per-row detail — so that adding a new list is a matter of writing a small
**viewer** (a query plus row and detail mapping) rather than a new feature
with its own routes, templates, filter parsing, paging, export, and role
scoping every time.

The immediate driver is the Survey Events log (a raw list of Member Status
API events, so the survey provider's developer and StrataHub staff can confirm
events arrived and were stored correctly). It is the first viewer; the
framework is designed against it.

## 2. Why now

StrataHub already has four hand-built lists of exactly this shape:

| Existing | Roles | Shape | Notes |
|----------|-------|-------|-------|
| Audit Log (`/audit`) | admin, coordinator | filters → table → detail | bespoke |
| Activity (`/activity`) | admin, coordinator, leader | filters → table → member detail, export | a live dashboard as much as a log (~1,200 lines) |
| Members Report (`/reports/members`) | admin, analyst | filters → table, CSV | bespoke |
| MHS Dashboard → Debug tab | admin, coordinator | student list → stratalog event timeline | bespoke, inside the dashboard |

Each re-implements the scaffolding, and the role-scoping lookups (leader →
groups → members; coordinator → assigned orgs) exist in three separate
copies. Survey Events would be the fifth instance, with more already
foreseeable: a stratalog game-event viewer outside the dashboard, login
history, an inbound-API request log, mhsgrader grade records. That is the
point at which extracting the common core pays back.

## 3. Principles

1. **Extract the machinery, not the UI.** The framework owns routing, role
   gating, scoping helpers, filter parsing and URL state, cursor paging, live
   refresh, CSV streaming, and the standard table/detail templates. A viewer
   owns its query, its columns, how a row and its detail are built, and any
   custom rendering it needs.
2. **Escape hatches from day one.** A viewer can name its own template
   snippet for any column and its own detail template, and can contribute
   extra controls. Without this the framework becomes a straitjacket and the
   next non-trivial viewer forks it.
3. **Per-viewer menu entries.** Users see "Survey Events" or "Audit Log"
   where they expect them; the shared implementation is invisible. An
   optional `/views` index lists what the current role may open.
4. **Scoping is enforced once.** Every query goes through the resolved
   `Scope`; a viewer cannot forget it. This is a FERPA-posture improvement
   over per-feature enforcement.
5. **DocumentDB first.** Cursor paging uses `{sortKey, _id}`; every viewer
   declares its sort key and ships the matching compound index (the
   single-field-index limitation is already documented for mhsgrader).
6. **Build it against a real viewer, then a second.** The interface is
   finalized by the first viewer and validated by the second; only after that
   is it a candidate for promotion into waffle's pantry.

## 4. Architecture

### 4.1 Packages

```
internal/app/system/viewscope/        shared role scoping (new; replaces 3 copies)
internal/app/features/viewers/        the framework feature: registry, routes, handlers, templates
internal/app/features/viewers/views/  one file per viewer (surveyevents.go, auditlog.go, …)
```

Viewers live inside the framework feature (as sub-package `views`) so the
registry can import them without cycles; each viewer imports only stores and
`viewscope`.

### 4.2 Registry

```go
type Registry struct{ viewers map[string]Viewer; order []string }
func (r *Registry) Register(v Viewer)          // at bootstrap; duplicate slug panics
func (r *Registry) For(role string) []Viewer   // menu / index listing
func (r *Registry) Get(slug string) (Viewer, bool)
```

Bootstrap registers the viewers it wants (`views.SurveyEvents(db, cfg)`,
…) and mounts the feature once. Workspaces without a given data source
(e.g. no survey configuration) can register conditionally.

### 4.3 The Viewer interface

```go
type Viewer interface {
    Slug() string                 // URL segment, e.g. "survey-events"
    Title() string                // menu label / page heading
    Description() string          // one line for the index page
    Roles() []string              // who may open it (superadmin implied)
    SortKey() string              // field paired with _id for cursor paging

    Filters() []FilterSpec        // rendered by the framework; parsed into Filters
    Columns() []ColumnSpec        // header text, width hint, optional cell snippet name

    Scope(ctx, *http.Request) (viewscope.Scope, error)          // usually viewscope.Resolve
    Query(ctx, viewscope.Scope, Filters, Cursor, limit int) (Page, error)
    Detail(ctx, viewscope.Scope, id string) (Detail, error)
}

// Optional capabilities, detected by type assertion:
type Exporter interface { Export(ctx, viewscope.Scope, Filters, w io.Writer) error }   // CSV
type Summarizer interface { Summary(ctx, viewscope.Scope, Filters) ([]Chip, error) }   // header chips
type Liveable interface { LiveIntervalSeconds() int }                                  // enables the Live toggle
```

Row and detail shapes are generic so the standard templates can render any
viewer, with per-cell overrides:

```go
type Cell   struct { Text, Class, Title, Href string; Snippet string; Data any } // Snippet+Data → custom template
type Row    struct { ID string; Cells []Cell; Class string }
type Page   struct { Rows []Row; Next Cursor; Total *int64 }
type Detail struct { Title, Subtitle string; Fields []Field; RawJSON string; Snippet string; Data any }
```

### 4.4 Filters

`FilterSpec{Key, Label, Type, Options, Placeholder, Default}` with types
`select`, `text`, `id` (exact ObjectID hex), `date-range` (with presets:
last hour / 24 h / 7 d / 30 d / custom), `bool`. The framework renders the
bar (HTMX `keyup changed delay:300ms` / `change`), parses the query string
into a typed `Filters` map, and reflects every value in the URL so a view can
be bookmarked or pasted to someone. Viewers translate `Filters` into their
Mongo filter inside `Query`.

Two filters are framework-supplied because they map to scope, not data:
**Organization** (offered when the scope spans more than one org) and
**Group** (always, within scope). Viewers receive the chosen ids in `Scope`.

### 4.5 Scoping — `viewscope`

```go
type Scope struct {
    WorkspaceID primitive.ObjectID
    Role        string
    All         bool                  // admin / analyst / superadmin: whole workspace
    OrgIDs      []primitive.ObjectID  // coordinator: assigned orgs (or the one chosen from them)
    GroupIDs    []primitive.ObjectID  // leader: own groups (or the one chosen)
    UserIDs     []primitive.ObjectID  // resolved members of GroupIDs (lazily)
}
func Resolve(ctx, db, r *http.Request, opts) (Scope, error)
func (s Scope) UserFilter(field string) bson.M   // "" when All; {$in} otherwise
func (s Scope) OrgFilter(field string) bson.M
```

This consolidates the group/org lookups that Activity, the MHS Dashboard, and
the audit feature each implement today. Existing features can adopt it
opportunistically; nothing forces a rewrite.

### 4.6 Routes and templates

Mounted once inside the workspace group, behind `RequireSignedIn`; the role
gate is per viewer from `Roles()`:

```
GET /views                       index of viewers available to this role (optional)
GET /views/{slug}                page: header chips, filter bar, table, paging
GET /views/{slug}/table          HTMX partial (filters + cursor in query string)
GET /views/{slug}/{id}           detail partial (drawer under the row)
GET /views/{slug}/export.csv     streamed CSV (viewers implementing Exporter)
```

Templates: `viewers_page`, `viewers_table`, `viewers_detail`, `viewers_index`,
plus a default cell snippet and default detail snippet. Light and dark
styles follow the app conventions; status pills reuse the glyph-first rule
from the dashboard (meaning never carried by color alone). A **Live** toggle
polls `/table` on the viewer's interval while on the first page.

### 4.7 Paging

Cursor = `{sortKey value, _id}` encoded opaquely; `limit` 50 (max 200).
Viewers must have a compound index `{workspace_id, sortKey, _id}` (plus any
filter-leading indexes they need). The framework never sorts on a field the
viewer didn't declare.

### 4.8 Security

- One role gate and one scope resolution path for all viewers.
- Detail and export re-apply the scope (a leader cannot fetch another
  group's row by guessing an id).
- Exports are streamed, scoped, and logged at Info with the viewer slug and
  filter summary; no secrets are ever part of a row (viewers must redact at
  mapping time — e.g. the Member Status API key is never stored anyway).

## 5. First viewer: Survey Events

Mapped onto the framework (full data design in
`docs/member-status-api/plan.md` §8):

- **Data:** new append-only `member_status_log` collection (one document per
  authenticated API request, accepted or rejected, and per launch-hook
  write), written by the API and `recordSurveyOpened`; the API returns
  `event_id`.
- **Roles:** admin, analyst, coordinator, leader. Sort key `received_at`.
- **Filters:** date range, source, survey (config items + "unrecognized"),
  state sent, outcome (accepted / rejected / error code), student (name or
  hex id), event id; Organization and Group from the framework.
- **Columns:** Received · Source · Student · Survey · State sent · Result
  (custom pill snippet) · detail expander.
- **Detail:** definition list + raw JSON of the stored entry.
- **Capabilities:** Exporter (CSV), Summarizer (received / accepted /
  rejected / last received), Liveable (10 s).
- **Navigation:** menu entry "Survey Events" for the four roles; cross-links
  from the MHS Dashboard Surveys tab, the survey modal (pre-filtered to the
  student), and the Settings page's API section.

## 6. Second viewer and beyond

1. **Audit Log** — same shape as today's feature; porting it validates the
   interface with a second real case and retires one bespoke implementation.
   Keep the old route working (redirect) until the port is verified.
2. Candidates after that, in likely order of value: stratalog game events
   (the Debug tab's timeline, outside the dashboard), login history, inbound
   API request log, mhsgrader grade records.
3. **Not** Activity: it is a live presence dashboard, not a log; porting it
   would be work without user benefit. It should adopt `viewscope` only.
4. **Waffle promotion:** after two viewers have proven the interface, the
   framework (registry, filter specs, cursor paging, CSV streaming, generic
   templates) is a candidate for `waffle/pantry/listview`, with `viewscope`
   staying in StrataHub (it encodes StrataHub's roles).

## 7. Task breakdown

| Task | Scope | Effort |
|------|-------|--------|
| V1 ✅ (2026-08-26) | `viewscope`: `Scope`, `Resolve`, filters; tests per role. Delivered as `internal/app/system/viewscope`: `Resolve(ctx, db, r, Options{OrgID, GroupID})` → `*Scope`; one `Filter(ctx, orgField, userField)` that picks the right constraint for the role (org `$in` for coordinators/org selections, member-id `$in` for leaders/group selections, match-nothing for an empty reach, nil when unrestricted); `UserIDs` (cached), `Orgs`/`Groups` option lists, `ErrForbidden`/`ErrNoWorkspace`/`ErrOutOfScope`. Coordinators are scoped to assigned orgs (as Activity does; the MHS Dashboard currently shows coordinators every group — a discrepancy to reconcile when it adopts `viewscope`). Leaders' inactive groups are excluded, matching the dashboards. | ½ day |
| V2 ✅ (2026-08-26) | `viewers` feature: registry, interface, filter specs + parsing + URL state, cursor paging, routes, page/table/detail/index templates (light/dark), Live toggle, CSV streaming; render test; handler tests (role gate, scope on detail/export). Delivered as `internal/app/features/viewers` mounted at `/views` (bootstrap creates the registry; viewers register there). Interface as designed (`Viewer` + optional `Exporter`/`Summarizer`/`Liveable`); rows/details are generic with `Cell.HTML`/`Detail.HTML` escape hatches and `Pill*` class constants; cursor = `{Key, ID}` with `TimeKey` helpers; filters `select/text/id/daterange` (presets + custom UTC) parsed by `ParseFilters`, org/group selectors supplied from `viewscope`; the table partial sets `HX-Replace-Url` so the address bar tracks the filters; load-more replaces its own row; generic CSV export pages `Query` (500/page, 20k cap) unless the viewer implements `Exporter`. Tests run through the real template engine with an in-memory fake viewer: index/page rendering, every filter type, org/group/coordinator/leader scoping and rejections, URL state, cursor paging, detail (incl. out-of-scope 404), CSV export and paging. | 1½ days |
| V3 ✅ (2026-08-27) | Survey Events data: `member_status_log` model/store/indexes/TTL; API and launch-hook writers; `event_id` in API responses; tests; provider + admin guide updates. Delivered: `models.MemberStatusLogEntry` (request as sent incl. `state_norm`, resolved user/org/entity/state, outcome), `store/memberstatuslog` (`Append` with clipping, `Get`, `List` with every viewer filter + `(received_at,_id)` cursor, `Count`, `Summarize`), indexes `idx_mslog_ws_received` / `_ws_user_received` / `_ws_entity_received` + `ttl_mslog_received` (400 d). The API logs every authenticated request (accepted or rejected) and returns `event_id` on both; unauthenticated failures and pings are not logged. `recordSurveyOpened` logs each tracked launch (source `launch`, with the resource id and the resulting state). Guides updated. | 1 day |
| V4 ✅ (2026-08-27) | Survey Events viewer on the framework; menu entries + cross-links; tests. Delivered: `features/viewers/views/surveyevents.go` (`NewSurveyEvents(db)`; roles admin/analyst/coordinator/leader; filters received-range (default 7 d) / source / survey incl. unrecognized / state sent / result incl. specific error codes / student by name-prefix or hex id / event id; columns Received (student's org zone) · Source · Student (link filters to them) · Survey · State sent · Result pills; detail with every stored field + JSON; Summarizer chips; Live 10 s; generic CSV) registered in bootstrap; menu "Survey Events" for admin, coordinator, leader, analyst; links from the Surveys tab legend, the survey modal (pre-filtered to the student), and the Settings API section; `memberstatuslog.ListQuery.UserIDs` added for name search. Tests: meta, rows/cells/scoping (leader, coordinator, admin), every filter, cursor paging, detail incl. out-of-scope 404, summary. Verified in the browser on the dev server: events generated through the real API and a real launch; leader saw only their students' events (rejected rows hidden), admin saw all incl. rejected; filters drive the URL; detail toggle; dark mode; zero console errors. | ½ day |
| V5 | Audit Log ported as the second viewer; old route redirected; tests | ½–1 day |
| V6 | Docs: this plan ticked, "adding a viewer" how-to, `ai/context.md`, docs index | ½ day |

V1–V4 deliver the Survey Events log (≈3½ days, versus ≈2 days standalone);
V5 is the validation step and first payback; each later viewer is ≈½ day.

## 8. Decisions to confirm

1. Framework first (this plan) vs. standalone Survey Events now and extract
   later — the difference is ≈1½ days up front.
2. Port Audit Log as the second viewer (recommended) or pick another.
3. Survey Events data decisions carried over from
   `docs/member-status-api/plan.md` §8.6: log rejected-but-authenticated
   requests (recommended), 400-day retention, provider-developer access via
   an analyst account against a test group, names shown to analysts as in
   the Members Report.
4. Menu placement: per-viewer entries only (recommended) or also a "Data
   Views" index entry.
