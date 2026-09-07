// internal/app/features/viewers/handler.go
package viewers

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
	basePath        = "/views"
)

// Handler serves every registered viewer.
type Handler struct {
	DB       *mongo.Database
	Registry *Registry
	Log      *zap.Logger
	ErrLog   *uierrors.ErrorLogger
	PageSize int // rows per page (default 50, max 200)
}

// NewHandler constructs the viewers handler.
func NewHandler(db *mongo.Database, reg *Registry, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{DB: db, Registry: reg, Log: logger, ErrLog: errLog, PageSize: defaultPageSize}
}

func (h *Handler) pageSize() int {
	if h.PageSize <= 0 {
		return defaultPageSize
	}
	if h.PageSize > maxPageSize {
		return maxPageSize
	}
	return h.PageSize
}

// --- view models -------------------------------------------------------------

type viewerCard struct {
	Slug, Title, Description string
}

type indexVM struct {
	viewdata.BaseVM
	Viewers []viewerCard
}

type optionVM struct {
	Value    string
	Label    string
	Selected bool
}

type filterVM struct {
	Key         string
	Label       string
	Type        FilterType
	Options     []optionVM
	Placeholder string
	Value       string
	From, To    string
	IsCustom    bool
}

type pageVM struct {
	viewdata.BaseVM
	Slug        string
	Title       string
	Description string

	ShowOrg   bool
	ShowGroup bool
	Orgs      []optionVM
	Groups    []optionVM
	Filters   []filterVM

	Chips         []Chip
	TableURL      string
	ExportURL     string
	ExportJSONURL string // empty unless the viewer implements JSONExporter
	Live          bool
	LiveSeconds int

	Table tableVM
}

type tableVM struct {
	Slug    string
	Columns []ColumnSpec
	ColSpan int
	Rows    []Row
	NextURL string // load-more URL; "" when there are no more rows
	Total   *int64
	IsMore  bool // rendering a continuation (rows only)
	Error   string
}

type detailVM struct {
	Slug    string
	ColSpan int
	Detail  *Detail
}

// --- request resolution ------------------------------------------------------

// request is everything resolved for one viewer request.
type request struct {
	viewer  Viewer
	scope   *viewscope.Scope
	filters Filters
	role    string
	org     string // selected org hex as given ("" = none)
	group   string // selected group hex as given ("" = none)
	live    bool
}

// resolve looks up the viewer, applies the role gate, resolves the scope with
// any org/group selection, and parses the filters. On failure it has written
// the response (a page for page requests, plain text for partials).
func (h *Handler) resolve(w http.ResponseWriter, r *http.Request, partial bool) (*request, bool) {
	fail := func(code int, msg string) {
		if partial {
			http.Error(w, msg, code)
			return
		}
		switch code {
		case http.StatusNotFound:
			uierrors.RenderNotFound(w, r, msg, basePath)
		case http.StatusForbidden:
			uierrors.RenderForbidden(w, r, msg, basePath)
		default:
			uierrors.RenderBadRequest(w, r, msg, basePath)
		}
	}

	slug := chi.URLParam(r, "slug")
	v, ok := h.Registry.Get(slug)
	if !ok {
		fail(http.StatusNotFound, "No such view.")
		return nil, false
	}
	role, _, _, ok := authz.UserCtx(r)
	if !ok || !Allowed(v, role) {
		fail(http.StatusForbidden, "You do not have access to this view.")
		return nil, false
	}

	q := r.URL.Query()
	req := &request{viewer: v, role: role, org: q.Get("org"), group: q.Get("group"), live: q.Get("live") == "1"}
	var opts viewscope.Options
	if req.org != "" {
		id, err := primitive.ObjectIDFromHex(req.org)
		if err != nil {
			fail(http.StatusBadRequest, "Invalid organization.")
			return nil, false
		}
		opts.OrgID = id
	}
	if req.group != "" {
		id, err := primitive.ObjectIDFromHex(req.group)
		if err != nil {
			fail(http.StatusBadRequest, "Invalid group.")
			return nil, false
		}
		opts.GroupID = id
	}

	scope, err := viewscope.Resolve(r.Context(), h.DB, r, opts)
	switch {
	case errors.Is(err, viewscope.ErrOutOfScope):
		fail(http.StatusForbidden, "That organization or group is outside your access.")
		return nil, false
	case errors.Is(err, viewscope.ErrForbidden):
		fail(http.StatusForbidden, "You do not have access to this view.")
		return nil, false
	case errors.Is(err, viewscope.ErrNoWorkspace):
		fail(http.StatusBadRequest, "A workspace is required.")
		return nil, false
	case err != nil:
		h.Log.Error("viewers: scope resolution failed", zap.String("view", slug), zap.Error(err))
		fail(http.StatusInternalServerError, "A database error occurred.")
		return nil, false
	}
	req.scope = scope
	req.filters = ParseFilters(v.Filters(), q)
	return req, true
}

// stateQuery is the query string that reproduces the current view: filters,
// org/group selection, and the live flag. Used for the page URL, the table
// URL, export, and load-more (which adds the cursor).
func (req *request) stateQuery() url.Values {
	q := req.filters.Values()
	if req.org != "" {
		q.Set("org", req.org)
	}
	if req.group != "" {
		q.Set("group", req.group)
	}
	if req.live {
		q.Set("live", "1")
	}
	return q
}

func withQuery(path string, q url.Values) string {
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// --- handlers ----------------------------------------------------------------

// ServeIndex lists the viewers the current user may open.
func (h *Handler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	vm := indexVM{BaseVM: viewdata.NewBaseVM(r, h.DB, "Data Views", "/dashboard")}
	for _, v := range h.Registry.ForRole(role) {
		vm.Viewers = append(vm.Viewers, viewerCard{Slug: v.Slug(), Title: v.Title(), Description: v.Description()})
	}
	templates.Render(w, r, "viewers_index", vm)
}

// ServePage renders a viewer's page with its first page of rows inline.
func (h *Handler) ServePage(w http.ResponseWriter, r *http.Request) {
	req, ok := h.resolve(w, r, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	v := req.viewer

	vm := pageVM{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, v.Title(), basePath),
		Slug:        v.Slug(),
		Title:       v.Title(),
		Description: v.Description(),
		TableURL:    basePath + "/" + v.Slug() + "/table",
		ExportURL:   withQuery(basePath+"/"+v.Slug()+"/export.csv", req.stateQuery()),
		Live:        req.live,
	}
	if l, ok := v.(Liveable); ok && l.LiveIntervalSeconds() > 0 {
		vm.LiveSeconds = l.LiveIntervalSeconds()
	}
	if _, ok := v.(JSONExporter); ok {
		vm.ExportJSONURL = withQuery(basePath+"/"+v.Slug()+"/export.json", req.stateQuery())
	}

	// Organization / group selectors from the scope.
	orgs, err := req.scope.Orgs(ctx)
	if err != nil {
		h.Log.Warn("viewers: org options", zap.Error(err))
	}
	if len(orgs) > 1 {
		vm.ShowOrg = true
		for _, o := range orgs {
			vm.Orgs = append(vm.Orgs, optionVM{Value: o.ID.Hex(), Label: o.Name, Selected: o.ID.Hex() == req.org})
		}
	}
	groups, err := req.scope.Groups(ctx)
	if err != nil {
		h.Log.Warn("viewers: group options", zap.Error(err))
	}
	if len(groups) > 0 {
		vm.ShowGroup = true
		for _, g := range groups {
			vm.Groups = append(vm.Groups, optionVM{Value: g.ID.Hex(), Label: g.Name, Selected: g.ID.Hex() == req.group})
		}
	}

	vm.Filters = buildFilterVMs(v.Filters(), req.filters)

	if s, ok := v.(Summarizer); ok {
		chips, err := s.Summary(ctx, req.scope, req.filters)
		if err != nil {
			h.Log.Warn("viewers: summary failed", zap.String("view", v.Slug()), zap.Error(err))
		} else {
			vm.Chips = chips
		}
	}

	vm.Table = h.buildTable(ctx, req, nil)
	templates.Render(w, r, "viewers_page", vm)
}

// ServeTable renders the table partial (HTMX): the full table for a filter
// change, or a rows-only continuation when a cursor is given.
func (h *Handler) ServeTable(w http.ResponseWriter, r *http.Request) {
	req, ok := h.resolve(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	after, err := ParseCursor(r.URL.Query().Get("after"))
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}
	vm := h.buildTable(ctx, req, after)
	if after == nil {
		// Keep the address bar in step with the filters so the view can be
		// bookmarked or pasted to someone.
		w.Header().Set("HX-Replace-Url", withQuery(basePath+"/"+req.viewer.Slug(), req.stateQuery()))
		templates.RenderSnippet(w, "viewers_table", vm)
		return
	}
	vm.IsMore = true
	templates.RenderSnippet(w, "viewers_rows", vm)
}

// ServeDetail renders one row's detail as a table row partial.
func (h *Handler) ServeDetail(w http.ResponseWriter, r *http.Request) {
	req, ok := h.resolve(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	id := chi.URLParam(r, "id")
	d, err := req.viewer.Detail(ctx, req.scope, id)
	switch {
	case errors.Is(err, ErrNotFound) || (err == nil && d == nil):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		h.Log.Error("viewers: detail failed", zap.String("view", req.viewer.Slug()), zap.String("id", id), zap.Error(err))
		http.Error(w, "detail failed", http.StatusInternalServerError)
		return
	}
	templates.RenderSnippet(w, "viewers_detail", detailVM{Slug: req.viewer.Slug(), ColSpan: len(req.viewer.Columns()) + 1, Detail: d})
}

// ServeExport streams the current filter within scope as CSV.
func (h *Handler) ServeExport(w http.ResponseWriter, r *http.Request) {
	req, ok := h.resolve(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()
	v := req.viewer

	filename := fmt.Sprintf("%s-%s.csv", v.Slug(), time.Now().UTC().Format("20060102-1504"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")

	h.Log.Info("viewers: export",
		zap.String("view", v.Slug()),
		zap.String("role", req.role),
		zap.String("query", req.stateQuery().Encode()))

	var err error
	if e, ok := v.(Exporter); ok {
		err = e.Export(ctx, req.scope, req.filters, w)
	} else {
		err = exportGeneric(ctx, v, req.scope, req.filters, w)
	}
	if err != nil {
		// Headers are already sent; log and stop the stream.
		h.Log.Error("viewers: export failed", zap.String("view", v.Slug()), zap.Error(err))
	}
}

// ServeExportJSON streams the current filter within scope as JSON, for
// viewers that implement JSONExporter; others get 404.
func (h *Handler) ServeExportJSON(w http.ResponseWriter, r *http.Request) {
	req, ok := h.resolve(w, r, true)
	if !ok {
		return
	}
	e, ok := req.viewer.(JSONExporter)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()

	filename := fmt.Sprintf("%s-%s.json", req.viewer.Slug(), time.Now().UTC().Format("20060102-1504"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")

	h.Log.Info("viewers: export json",
		zap.String("view", req.viewer.Slug()),
		zap.String("role", req.role),
		zap.String("query", req.stateQuery().Encode()))

	if err := e.ExportJSON(ctx, req.scope, req.filters, w); err != nil {
		h.Log.Error("viewers: export json failed", zap.String("view", req.viewer.Slug()), zap.Error(err))
	}
}

// ServeDetailJSON returns one row as a downloadable JSON document, for
// viewers that implement JSONDetailer; others get 404.
func (h *Handler) ServeDetailJSON(w http.ResponseWriter, r *http.Request) {
	req, ok := h.resolve(w, r, true)
	if !ok {
		return
	}
	d, ok := req.viewer.(JSONDetailer)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	id := chi.URLParam(r, "id")
	body, err := d.DetailJSON(ctx, req.scope, id)
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil:
		h.Log.Error("viewers: detail json failed", zap.String("view", req.viewer.Slug()), zap.String("id", id), zap.Error(err))
		http.Error(w, "detail failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+req.viewer.Slug()+"-"+id+`.json"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// --- helpers -----------------------------------------------------------------

// buildTable runs the viewer's query and shapes the table view model. A query
// error is shown in the table area rather than failing the page.
func (h *Handler) buildTable(ctx context.Context, req *request, after *Cursor) tableVM {
	v := req.viewer
	vm := tableVM{Slug: v.Slug(), Columns: v.Columns(), ColSpan: len(v.Columns()) + 1}
	page, err := v.Query(ctx, req.scope, req.filters, after, h.pageSize())
	if err != nil {
		h.Log.Error("viewers: query failed", zap.String("view", v.Slug()), zap.Error(err))
		vm.Error = "The data could not be loaded. Please try again."
		return vm
	}
	vm.Rows = page.Rows
	vm.Total = page.Total
	if page.Next != nil && len(page.Rows) > 0 {
		q := req.stateQuery()
		q.Set("after", page.Next.String())
		vm.NextURL = withQuery(basePath+"/"+v.Slug()+"/table", q)
	}
	return vm
}

func buildFilterVMs(specs []FilterSpec, f Filters) []filterVM {
	out := make([]filterVM, 0, len(specs))
	for _, s := range specs {
		fv := filterVM{Key: s.Key, Label: s.Label, Type: s.Type, Placeholder: s.Placeholder, Value: f.Get(s.Key)}
		switch s.Type {
		case FilterSelect:
			for _, o := range s.Options {
				fv.Options = append(fv.Options, optionVM{Value: o.Value, Label: o.Label, Selected: o.Value == fv.Value})
			}
		case FilterDateRange:
			for _, o := range DateRangeOptions {
				fv.Options = append(fv.Options, optionVM{Value: o.Value, Label: o.Label, Selected: o.Value == fv.Value})
			}
			fv.IsCustom = fv.Value == RangeCustom
			fv.From = f.Get(s.Key + "_from")
			fv.To = f.Get(s.Key + "_to")
		}
		out = append(out, fv)
	}
	return out
}
