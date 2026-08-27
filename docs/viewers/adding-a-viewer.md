# Adding a Data Viewer

A **viewer** is one role-gated, scoped, filterable, pageable list served by
the viewers framework at `/views/{slug}`. The framework owns the routes, the
role gate, scope resolution, the filter bar and URL state, cursor paging, the
Live toggle, CSV export, and the templates. A viewer owns three things: its
query, how a row is built, and how a row's detail is built.

Two viewers are live and are the reference implementations:

| Viewer | File | Roles | Notes |
|--------|------|-------|-------|
| Survey Events (`/views/survey-events`) | `internal/app/features/viewers/views/surveyevents.go` | admin, analyst, coordinator, leader | summary chips, Live refresh, name search |
| Audit Log (`/views/audit-log`) | `internal/app/features/viewers/views/auditlog.go` | admin, coordinator | summary chips, actor/target mapping, per-event time zone |

Framework code: `internal/app/features/viewers/` (`viewer.go` is the
contract; `handler.go` the routes; `filters.go`, `cursor.go`, `export.go`).
Scoping: `internal/app/system/viewscope/`. Design and delivery record:
[plan.md](plan.md).

Budget: a viewer over an existing collection is about half a day, most of it
the store query and the tests.

## 1. Give the store a viewer query

The framework pages newest-first with a cursor of `{sort key, _id}`, so the
store needs a `List` that accepts a position and returns `limit + 1` rows to
learn whether there is more, plus a `Count` for summary chips. Pattern (from
`internal/app/store/audit/store.go`):

```go
type Position struct {
    Timestamp time.Time
    ID        primitive.ObjectID
}

type ListQuery struct {
    WorkspaceID primitive.ObjectID
    Scope       bson.M       // from viewscope.Scope.Filter; nil = unrestricted
    From, To    time.Time    // zero = open
    // ... one field per filter the viewer offers ...
    After       *Position    // cursor; nil = first page
    Limit       int
}

func (q ListQuery) filter() bson.M {
    f := bson.M{"workspace_id": q.WorkspaceID}
    var and []bson.M
    if q.Scope != nil {
        and = append(and, q.Scope)
    }
    // ... filters ...
    if q.After != nil {
        and = append(and, bson.M{"$or": []bson.M{
            {"timestamp": bson.M{"$lt": q.After.Timestamp.UTC()}},
            {"timestamp": q.After.Timestamp.UTC(), "_id": bson.M{"$lt": q.After.ID}},
        }})
    }
    if len(and) > 0 {
        f["$and"] = and
    }
    return f
}

func (s *Store) List(ctx context.Context, q ListQuery) (rows []Event, more bool, err error) {
    opts := options.Find().
        SetSort(bson.D{{Key: "timestamp", Value: -1}, {Key: "_id", Value: -1}}).
        SetLimit(int64(q.Limit + 1))
    // ... find, decode, then:
    if len(rows) > q.Limit {
        return rows[:q.Limit], true, nil
    }
    return rows, false, nil
}

func (s *Store) Count(ctx context.Context, q ListQuery) (int64, error)
```

**Index.** The sort must be backed by a compound index
`{workspace_id: 1, <sortField>: -1, _id: -1}` — DocumentDB will not use a
single-field index for a `{field, _id}` sort. Add it in
`internal/app/system/indexes/indexes.go` (or the store's `EnsureIndexes`) and
add filter-leading indexes for the filters you expect to be used alone (the
member-status log has `{workspace_id, resolved.user_id, received_at}` for the
student filter, for example). Name indexes (`idx_<coll>_ws_<field>`).

Put scope and filter fields in the filter in the order the index can use;
keep `workspace_id` first.

## 2. Write the viewer

One file in `internal/app/features/viewers/views/`. Implement
`viewers.Viewer`; add `Summarizer`, `Liveable`, or `Exporter` if wanted.

```go
package views

type Widgets struct {
    db    *mongo.Database
    store *widgets.Store
}

func NewWidgets(db *mongo.Database) *Widgets { return &Widgets{db: db, store: widgets.New(db)} }

func (v *Widgets) Slug() string        { return "widgets" }         // URL segment: /views/widgets
func (v *Widgets) Title() string       { return "Widgets" }         // menu label and heading
func (v *Widgets) Description() string { return "One line for the index page and the subheading." }
func (v *Widgets) Roles() []string     { return []string{"admin", "coordinator"} } // superadmin is implied
```

### Filters

Declare the controls; the framework renders them, parses the query string,
applies `Default` when a parameter is absent, and keeps every value in the
URL (`HX-Replace-Url`), so a view can be bookmarked or pasted to a colleague.

```go
const (
    fWhen   = "when"
    fKind   = "kind"
    fPerson = "person"
    fID     = "id"
)

func (v *Widgets) Filters() []viewers.FilterSpec {
    return []viewers.FilterSpec{
        {Key: fWhen, Label: "When", Type: viewers.FilterDateRange, Default: viewers.RangeWeek},
        {Key: fKind, Label: "Kind", Type: viewers.FilterSelect, Options: []viewers.Option{{Value: "a", Label: "Kind A"}}},
        {Key: fPerson, Label: "Person", Type: viewers.FilterText, Placeholder: "name or 24-char id"},
        {Key: fID, Label: "Widget id", Type: viewers.FilterID, Placeholder: "24-char id"},
    }
}
```

Types: `FilterSelect` (an "any" option is added for you), `FilterText`
(the viewer decides what the text means), `FilterID` (24-char hex),
`FilterDateRange` (presets last hour / 24 h / 7 d / 30 d / custom UTC
range; read it with `f.DateRange(key, time.Now().UTC())`). Defaults: a log
that grows without bound should default to `RangeWeek`; a list people scan
for something specific (the Audit Log) defaults to any time.

**Do not declare Organization or Group filters.** The framework supplies
them from `viewscope` — an organization selector when the scope spans more
than one organization, a group selector within the scope — and the viewer
receives the choice already folded into the `Scope`.

### Columns

```go
func (v *Widgets) Columns() []viewers.ColumnSpec {
    return []viewers.ColumnSpec{
        {Key: "time", Label: "Time", Class: "whitespace-nowrap"},
        {Key: "kind", Label: "Kind"},
        {Key: "who", Label: "Person"},
        {Key: "id", Label: "Id", Mono: true},
    }
}
```

`Key` is the CSV header (falls back to `Label`); `Class` is added to the
header and every cell; `Mono` renders the column in a monospace face. Every
row must have exactly `len(Columns())` cells.

### Query

Translate scope + filters into the store query, apply the cursor, build
rows. Two conventions matter:

- **Scope first.** `scope.Filter(ctx, orgField, userField)` returns the
  Mongo constraint for the caller's reach — `nil` for an unrestricted
  admin/analyst, an `$in` on the organization field for coordinators and
  org selections, an `$in` on the user field for leaders and group
  selections, and a match-nothing filter for an empty reach. Name the two
  fields as they are stored in *your* collection (`"organization_id"`,
  `"user_id"`; `"resolved.organization_id"` for nested documents). Pass
  `""` as `orgField` if your documents have no organization and everything
  must go through users.
- **A filter that can match nothing returns no rows, not an error.** A
  person search with no hits, or an id filter that is not valid hex, should
  produce an empty page (the reference viewers do this with a `nil` query).

```go
func (v *Widgets) listQuery(ctx context.Context, scope *viewscope.Scope, f viewers.Filters) (*widgets.ListQuery, error) {
    sf, err := scope.Filter(ctx, "organization_id", "user_id")
    if err != nil {
        return nil, err
    }
    q := &widgets.ListQuery{WorkspaceID: scope.WorkspaceID, Scope: sf, Kind: f.Get(fKind)}
    if from, to, ok := f.DateRange(fWhen, time.Now().UTC()); ok {
        q.From, q.To = from, to
    }
    if id, ok := f.ID(fID); ok {
        q.ID = &id
    } else if f.Has(fID) {
        return nil, nil // not a valid id: nothing can match
    }
    if s := f.Get(fPerson); s != "" {
        // hex → exact id; otherwise a folded name-prefix search on users.full_name_ci
        // (see searchMembersByName in surveyevents.go / searchPeople in auditlog.go)
    }
    return q, nil
}

func (v *Widgets) Query(ctx context.Context, scope *viewscope.Scope, f viewers.Filters, after *viewers.Cursor, limit int) (viewers.Page, error) {
    q, err := v.listQuery(ctx, scope, f)
    if err != nil || q == nil {
        return viewers.Page{}, err
    }
    if after != nil {
        at, err := viewers.ParseTimeKey(after.Key)
        if err != nil {
            return viewers.Page{}, err
        }
        q.After = &widgets.Position{Timestamp: at, ID: after.ID}
    }
    q.Limit = limit

    rows, more, err := v.store.List(ctx, *q)
    if err != nil {
        return viewers.Page{}, err
    }
    rc := v.lookups(ctx, rows) // one $in query per referenced collection: names, org time zones
    page := viewers.Page{}
    for _, r := range rows {
        page.Rows = append(page.Rows, v.row(r, rc))
    }
    if more && len(rows) > 0 {
        last := rows[len(rows)-1]
        page.Next = &viewers.Cursor{Key: viewers.TimeKey(last.Timestamp), ID: last.ID}
    }
    return page, nil
}
```

The cursor `Key` is a string the viewer formats and parses itself;
`viewers.TimeKey`/`ParseTimeKey` cover the common case of a time sort. Leave
`Page.Next` nil when the page is not full. The framework asks for 50 rows
(max 200) and renders a "Load more" row when `Next` is set.

**Lookups.** Never query per row. Collect the ids a page references (users,
organizations) and resolve them with one `$in` query each; a missing name
(deleted user) renders as blank text with the id in the tooltip rather than
a raw id in the column.

**Times.** Show times in the organization's time zone (`organizations.time_zone`,
`time.LoadLocation`), fall back to UTC, and put the RFC 3339 UTC value in the
cell's tooltip.

### Rows and cells

```go
func (v *Widgets) row(w widgets.Widget, rc rowContext) viewers.Row {
    when := viewers.Cell{Text: w.Timestamp.In(rc.loc(w)).Format("Jan 2, 2006 3:04:05 PM MST"),
        Title: w.Timestamp.UTC().Format(time.RFC3339) + " UTC"}
    kind := viewers.Cell{Text: "Kind A", Class: viewers.PillBlue}
    who := viewers.Cell{Text: rc.names[w.UserID], Title: w.UserID.Hex(),
        Href: "/views/widgets?person=" + w.UserID.Hex()} // a link that filters to this person
    return viewers.Row{ID: w.ID.Hex(), Cells: []viewers.Cell{when, kind, who, {Text: w.ID.Hex()}}}
}
```

`Cell.Text` is the default rendering and the CSV value; `Class` wraps it in
a styled span; `Href` makes it a link; `Title` is the tooltip; `HTML` is the
escape hatch for custom, already-safe markup. Use the `Pill*` and `Text*`
constants from `viewer.go` for status colors — they are defined in Go so
Tailwind's content scan compiles them, and each has a dark variant. Meaning
must not be carried by color alone: the pill text says what the state is.

`Row.ID` must be unique within the viewer; it is the DOM id and the detail
request's `{id}`.

### Detail

`Detail` is the expanded view under a row (the "is it what I sent?" view).
It **must re-apply scope** — fetch the row with the scope filter and the id,
and return `viewers.ErrNotFound` when nothing matches, so a leader cannot
open another group's row by guessing an id. Return the fields in reading
order, the raw stored document as indented JSON, and omit fields that are
empty rather than printing blanks.

```go
func (v *Widgets) Detail(ctx context.Context, scope *viewscope.Scope, id string) (*viewers.Detail, error) {
    oid, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return nil, viewers.ErrNotFound
    }
    sf, err := scope.Filter(ctx, "organization_id", "user_id")
    if err != nil {
        return nil, err
    }
    rows, _, err := v.store.List(ctx, widgets.ListQuery{WorkspaceID: scope.WorkspaceID, Scope: sf, ID: &oid, Limit: 1})
    if err != nil {
        return nil, err
    }
    if len(rows) == 0 {
        return nil, viewers.ErrNotFound
    }
    w := rows[0]
    raw, _ := json.MarshalIndent(w, "", "  ")
    return &viewers.Detail{
        Title:    "Widget " + w.ID.Hex(),
        Subtitle: "Kind A",
        Fields:   []viewers.Field{{Label: "Id", Value: w.ID.Hex(), Mono: true} /* ... */},
        RawJSON:  string(raw),
    }, nil
}
```

`Detail.HTML` exists for a custom body when fields are not enough.

### Optional capabilities

- **`Summary(ctx, scope, f) ([]viewers.Chip, error)`** — header chips for
  the current filter (counts, last-received). Use the store's `Count` /
  aggregate; reuse `listQuery` so chips and rows always agree. Return a
  single `{Events 0}` chip when the query can match nothing.
- **`LiveIntervalSeconds() int`** — enables the Live toggle, which re-polls
  the table on that interval while on the first page. Use it for feeds
  someone watches while testing (Survey Events: 10 s); leave it off for
  reference lists.
- **`Export(ctx, scope, f, w io.Writer) error`** — only if the generic CSV
  is not right. Without it the framework exports by paging `Query` (500 rows
  per page, 20,000-row cap) and writing `Cell.Text` under the column keys;
  that is usually what you want.

### What not to put in a row

No secrets, ever — redact at mapping time. Nothing a role should not see:
the scope filter decides *which* rows, but the viewer decides *what* is in
them (analysts see names in Survey Events because the Members Report already
shows them names; a viewer for a role that must not see names would render
the hex id instead).

## 3. Register it

In `internal/app/bootstrap/routes.go`, inside the workspace router where the
registry is built:

```go
viewerRegistry.Register(views.NewWidgets(deps.StrataHubMongoDatabase))
```

Registration order is display order on `/views`. A duplicate or empty slug
panics at startup. A viewer that needs configuration that may be absent
(Survey Events needs the survey configuration) is constructed with an error
return and registered conditionally, so the rest of the app still starts.

## 4. Menu and cross-links

Menu entries are hand-placed in `internal/app/resources/templates/menu.gohtml`
— one `<a>` per role block that may open the viewer, next to related
entries (Survey Events sits after MHS Dashboard for staff and after Members
Report for analysts). `/views` lists every viewer the role may open, but the
menu is how people find things.

Link to the viewer from wherever a question arises, with the filter in the
URL: the Surveys tab links to `/views/survey-events?student=<hex>`, the
Settings API section to `/views/survey-events`. Old addresses that people
may have bookmarked get a redirect, as `/audit` → `/views/audit-log` does.

## 5. Tests

Viewer tests live in `internal/app/features/viewers/views/` in package
`views_test` and run against the local MongoDB through
`testutil.SetupTestDB` (indexes are created for you). Follow
`surveyevents_test.go` / `auditlog_test.go`:

- a `world` fixture seeding two organizations (one with a time zone), a
  group, users of each relevant role, the coordinator assignment and group
  memberships, and a handful of rows chosen so every filter and both sides
  of every scope rule have a distinct answer;
- `scope(t, userID, role)` builds a request with `workspace.WithTestWorkspace`
  and `auth.WithTestUser` and resolves it with `viewscope.Resolve`;
- `filters(v, "key", "value", ...)` builds `Filters` through
  `viewers.ParseFilters` so defaults apply exactly as in production;
- `expectIDs` compares row ids in order.

Cover: metadata and roles (`viewers.Allowed`), rows and cell content for
each distinct row shape, scoping per role (including that an out-of-scope
detail returns `ErrNotFound`), every filter alone and one combination,
cursor paging across a page boundary, detail fields and raw JSON, and
summary chips (including the match-nothing case).

The framework's own handler tests (`viewers/handler_test.go`) exercise the
routes, role gate, URL state, paging, and export with an in-memory fake
viewer — you do not need to repeat those.

Run: `go test ./internal/app/features/viewers/...` (or `make test-safe` for
the whole suite, sequentially).

## 6. Finish

- `gofmt`, `go vet ./...`, tests.
- `make css-prod` if you introduced Tailwind classes that did not exist
  before (the `Pill*` constants are already compiled).
- Add the viewer to the table at the top of this document and to the
  "Data Viewers" section of `docs/docs_index.md`; mention it in
  `ai/context.md`'s viewers row.
- Verify in the browser as each role that may open it: rows, a filter or
  two, the detail expander, CSV, and dark mode. Templates are embedded, so
  rebuild and restart the dev server to see template changes.

## Checklist

- [ ] Store `List` (limit+1, cursor) and `Count`; compound index
      `{workspace_id, sortField, _id}`; filter-leading indexes as needed
- [ ] Viewer file in `views/`: `Slug`, `Title`, `Description`, `Roles`,
      `Filters`, `Columns`, `Query`, `Detail` (+ `Summary` / `LiveIntervalSeconds` if wanted)
- [ ] `scope.Filter` applied in `Query` **and** `Detail`; empty match → empty page
- [ ] Batched lookups; org-zone times with UTC tooltip; no secrets, no raw ids where a name belongs
- [ ] Registered in bootstrap; menu entries per role; cross-links; redirect from any old address
- [ ] Tests: meta, rows, scope per role, filters, paging, detail, summary
- [ ] Docs: this table, `docs_index.md`, `ai/context.md`
