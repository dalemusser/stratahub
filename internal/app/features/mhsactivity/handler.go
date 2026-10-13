package mhsactivity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/reportpolicy"
	"github.com/dalemusser/stratahub/internal/app/store/mhsactivitydays"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicetests"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	activity "github.com/dalemusser/stratahub/internal/app/system/mhsactivity"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler serves the MHS Activity page.
type Handler struct {
	DB       *mongo.Database
	Compute  activity.Deps
	Store    *mhsactivitydays.Store // stored (rolled-up) days; nil disables the stored path
	Backfill *activity.Backfiller
	Config   Config
	Log      *zap.Logger
	ErrLog   *uierrors.ErrorLogger

	cache *dayCache
	now   func() time.Time
}

// NewHandler wires the page to the three databases it reads.
func NewHandler(db, logDB, gradesDB *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger, cfg Config) *Handler {
	if cfg.MaxLiveDays <= 0 {
		cfg.MaxLiveDays = 7
	}
	if cfg.MaxDays < cfg.MaxLiveDays {
		cfg.MaxDays = cfg.MaxLiveDays
	}
	deps := activity.Deps{DB: db, LogDB: logDB, GradesDB: gradesDB, Log: logger, CloseAfter: cfg.CloseAfter}
	store := mhsactivitydays.New(db)
	h := &Handler{
		DB:       db,
		Compute:  deps,
		Store:    store,
		Backfill: activity.NewBackfiller(deps, store, mhsdevicetests.New(db)),
		Config:   cfg,
		Log:      logger,
		ErrLog:   errLog,
		cache:    newDayCache(cfg.CacheTTL),
	}
	h.setNow(func() time.Time { return time.Now().UTC() })
	return h
}

// setNow installs the clock the page and the backfill use (tests).
func (h *Handler) setNow(now func() time.Time) {
	h.now = now
	h.Backfill.Now = now
}

// Routes mounts the page. Staff only: superadmin, admin and analyst see the
// workspace, coordinators their organizations (viewscope); the handlers
// check the policy again so the gate holds without the middleware too.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()
	r.Group(func(g chi.Router) {
		g.Use(sm.RequireSignedIn)
		g.Use(sm.RequireRole("superadmin", "admin", "analyst", "coordinator"))
		g.Get("/", h.ServePage)
		g.Get("/school/{orgID}", h.ServeSchool)
		g.Get("/schools.csv", h.ServeSchoolsCSV)
		g.Get("/students.csv", h.ServeStudentsCSV)
		g.Get("/student/{userID}/day/{day}", h.ServeStudentDay)
	})
	r.Group(func(a chi.Router) {
		a.Use(sm.RequireSignedIn)
		a.Use(sm.RequireRole("superadmin", "admin"))
		a.Get("/admin", h.ServeAdmin)
		a.Get("/admin/status", h.ServeAdminStatus)
		a.Post("/admin/backfill", h.HandleBackfill)
		a.Post("/admin/stop", h.HandleBackfillStop)
	})
	return r
}

// ServePage renders the overview, narrowed by the query's org and group.
func (h *Handler) ServePage(w http.ResponseWriter, r *http.Request) {
	vm, ok := h.load(w, r, primitive.NilObjectID)
	if !ok {
		return
	}
	h.render(w, r, vm)
}

// ServeSchool renders the overview with one school opened (the HTMX
// fragment when the request targets the school panel).
func (h *Handler) ServeSchool(w http.ResponseWriter, r *http.Request) {
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "orgID"))
	if err != nil {
		uierrors.RenderNotFound(w, r, "No such school.", "/mhsactivity")
		return
	}
	vm, ok := h.load(w, r, id)
	if !ok {
		return
	}
	h.render(w, r, vm)
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, vm *pageVM) {
	templates.RenderAutoMap(w, r, "mhsactivity_page", map[string]string{"mhsactivity-school": "mhsactivity_school"}, vm)
}

// load checks access, resolves the scope and the window, gathers the days
// (stored or computed) and builds the report. When it returns false it has
// already written the response (an error page).
func (h *Handler) load(w http.ResponseWriter, r *http.Request, pathOrg primitive.ObjectID) (*pageVM, bool) {
	if _, _, _, ok := authz.UserCtx(r); !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return nil, false
	}
	pol := reportpolicy.CanViewMHSActivity(r)
	if !pol.CanView {
		uierrors.RenderForbidden(w, r, "You do not have access to MHS Activity.", "/dashboard")
		return nil, false
	}
	if !h.Config.Enabled {
		uierrors.RenderNotFound(w, r, "MHS Activity is turned off.", "/dashboard")
		return nil, false
	}
	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "mhs activity")
	defer cancel()
	wsID := workspace.IDFromRequest(r)

	win, err := ParseWindow(r, h.Config.MaxDays, h.now())
	if err != nil {
		uierrors.RenderBadRequest(w, r, err.Error(), "/mhsactivity")
		return nil, false
	}
	q := r.URL.Query()
	orgID := pathOrg
	if orgID.IsZero() {
		orgID, _ = primitive.ObjectIDFromHex(strings.TrimSpace(q.Get("org")))
	}
	groupID, _ := primitive.ObjectIDFromHex(strings.TrimSpace(q.Get("group")))
	showStaff := q.Get("staff") == "1"

	scope, err := viewscope.Resolve(ctx, h.DB, r, viewscope.Options{OrgID: orgID, GroupID: groupID})
	if err != nil {
		if errors.Is(err, viewscope.ErrOutOfScope) || errors.Is(err, viewscope.ErrForbidden) {
			uierrors.RenderForbidden(w, r, "That school or group is outside your scope.", "/mhsactivity")
			return nil, false
		}
		h.fail(w, r, "resolve scope", err)
		return nil, false
	}
	orgOpts, err := scope.Orgs(ctx)
	if err != nil {
		h.fail(w, r, "list organizations", err)
		return nil, false
	}
	groupOpts, err := scope.Groups(ctx)
	if err != nil {
		h.fail(w, r, "list groups", err)
		return nil, false
	}

	// Which organizations the page shows: the selected group's, the
	// selected one, the scope's, or all. Every zone-day is computed for all
	// of its organizations and cached once (the key has no scope in it), so
	// one computation serves every staff user; the narrowing happens on the
	// result below.
	selectedOrg := scope.SelectedOrg
	if !scope.SelectedGroup.IsZero() {
		for _, g := range groupOpts {
			if g.ID == scope.SelectedGroup {
				selectedOrg = g.OrgID
				break
			}
		}
	}
	var allowedOrgs map[primitive.ObjectID]bool // nil = every organization
	switch {
	case scope.Empty():
		allowedOrgs = map[primitive.ObjectID]bool{}
	case !selectedOrg.IsZero():
		allowedOrgs = map[primitive.ObjectID]bool{selectedOrg: true}
	case scope.OrgIDs != nil:
		allowedOrgs = map[primitive.ObjectID]bool{}
		for _, id := range scope.OrgIDs {
			allowedOrgs[id] = true
		}
	}

	zones, err := activity.ZonesFor(ctx, h.DB, wsID, nil)
	if err != nil {
		h.fail(w, r, "zones", err)
		return nil, false
	}
	now := h.now()
	var docs []models.MHSActivityDay
	fresh := &freshness{}
	multiDay := false
	live := 0
	for _, z := range zones {
		days := win.DaysIn(z.Loc, now)
		if len(days) > 1 {
			multiDay = true
		}
		for _, day := range days {
			// A closed day whose organizations are all rolled up comes from
			// the store; anything else is computed live (and cached).
			if h.Store != nil && activity.Closed(day, z.Loc, h.Compute.EffectiveCloseAfter(), now) {
				stored, at, partial, err := h.loadStored(ctx, wsID, day, z)
				if err != nil {
					h.fail(w, r, "stored "+day, err)
					return nil, false
				}
				if stored != nil {
					docs = append(docs, stored...)
					fresh.stored(day, at, partial)
					continue
				}
			}
			live++
			if live > h.Config.MaxLiveDays {
				uierrors.RenderBadRequest(w, r, fmt.Sprintf("This range needs more than %d days computed live; the stored history for it is still being built. Try a shorter range, or run the backfill from the admin page.", h.Config.MaxLiveDays), "/mhsactivity")
				return nil, false
			}
			zone, dd := z, day
			key := wsID.Hex() + "|" + z.TZ + "|" + day
			got, at, err := h.cache.Get(key, now, func() ([]models.MHSActivityDay, error) {
				// Detached from the request: a cancelled request must not
				// poison the entry other requests are waiting on.
				cctx, ccancel := context.WithTimeout(context.Background(), timeouts.Long())
				defer ccancel()
				return activity.ComputeDay(cctx, h.Compute, wsID, dd, zone)
			})
			if err != nil {
				h.fail(w, r, "compute "+day, err)
				return nil, false
			}
			docs = append(docs, got...)
			fresh.live(day, z, at)
		}
	}

	if allowedOrgs != nil {
		kept := docs[:0]
		for _, d := range docs {
			if allowedOrgs[d.OrganizationID] {
				kept = append(kept, d)
			}
		}
		docs = kept
	}
	// A group restriction (a group selection) narrows to its members.
	if scope.GroupIDs != nil {
		ids, err := scope.UserIDs(ctx)
		if err != nil {
			h.fail(w, r, "scope members", err)
			return nil, false
		}
		allowed := make(map[primitive.ObjectID]bool, len(ids))
		for _, id := range ids {
			allowed[id] = true
		}
		kept := docs[:0]
		for _, d := range docs {
			if allowed[d.UserID] || d.Role == models.MHSActivityRoleStaff {
				kept = append(kept, d)
			}
		}
		docs = kept
	}

	lk, err := loadLookups(ctx, h.DB, docs, orgOpts, groupOpts)
	if err != nil {
		h.fail(w, r, "lookups", err)
		return nil, false
	}
	canOpenRecords := authz.IsAdmin(r) || authz.IsAnalyst(r) // the Device Tests viewer's roles
	rep := BuildReport(docs, lk, Options{SelectedOrg: selectedOrg, SelectedGroup: scope.SelectedGroup, ShowStaff: showStaff, MultiDay: multiDay, Now: now, CanOpenDeviceTests: canOpenRecords})

	missing := map[string]bool{}
	for _, d := range docs {
		for _, m := range d.Missing {
			missing[m] = true
		}
	}
	var missingList []string
	for m := range missing {
		missingList = append(missingList, missingLabel(m))
	}

	wq := win.Query()
	if showStaff {
		wq.Set("staff", "1")
	}
	vm := pageVM{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "MHS Activity", "/dashboard"),
		Window:        windowVM{Range: win.Range, From: win.From, To: win.To, Label: win.Label(), Custom: win.Range == RangeCustom, MaxDays: h.Config.MaxDays},
		IsAdmin:       authz.IsAdmin(r),
		SelectedOrg:   hexOrEmpty(selectedOrg),
		SelectedGroup: hexOrEmpty(scope.SelectedGroup),
		ShowStaff:     showStaff,
		Freshness:     fresh.lines(),
		Missing:       missingList,
		Report:        rep,
		WindowQuery:   wq.Encode(),
	}
	for _, o := range orgOpts {
		vm.Orgs = append(vm.Orgs, optionVM{ID: o.ID.Hex(), Name: o.Name, Selected: o.ID == selectedOrg})
	}
	for _, g := range groupOpts {
		if !selectedOrg.IsZero() && g.OrgID != selectedOrg {
			continue
		}
		vm.Groups = append(vm.Groups, optionVM{ID: g.ID.Hex(), Name: g.Name, OrgID: g.OrgID.Hex(), Selected: g.ID == scope.SelectedGroup})
	}

	vm.docs = docs
	vm.lk = lk
	return &vm, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, what string, err error) {
	const msg = "The activity report could not be built right now."
	if h.ErrLog != nil {
		h.ErrLog.LogServerError(w, r, "mhs activity: "+what, err, msg, "/dashboard")
		return
	}
	if h.Log != nil {
		h.Log.Error("mhs activity: "+what, zap.Error(err))
	}
	uierrors.RenderServerError(w, r, msg, "/dashboard")
}

func hexOrEmpty(id primitive.ObjectID) string {
	if id.IsZero() {
		return ""
	}
	return id.Hex()
}

func missingLabel(m string) string {
	switch m {
	case activity.MissingLogData:
		return "the game log service (played is estimated from launch records)"
	case activity.MissingGrader:
		return "the grader (no current unit)"
	case activity.MissingDeviceStatus:
		return "device status rows"
	case activity.MissingPageViews:
		return "page views"
	}
	return m
}

// SchoolHref links to this page narrowed to a school, keeping the window.
func SchoolHref(orgID string, windowQuery string) string {
	v, _ := url.ParseQuery(windowQuery)
	v.Set("org", orgID)
	return "/mhsactivity?" + v.Encode()
}

// loadStored returns the stored documents of a closed zone-day when every
// organization of the zone has a final mark of the current version; nil
// otherwise (the caller computes live). It also returns the oldest
// computed-at of those marks and whether any is partial.
func (h *Handler) loadStored(ctx context.Context, wsID primitive.ObjectID, day string, z activity.Zone) ([]models.MHSActivityDay, time.Time, bool, error) {
	marks, err := h.Store.GetMarks(ctx, wsID, day, day, z.OrgIDs)
	if err != nil {
		return nil, time.Time{}, false, err
	}
	var at time.Time
	partial := false
	for _, org := range z.OrgIDs {
		m, ok := marks[mhsactivitydays.MarkKey(org, day)]
		if !ok || !m.Final || m.Version != models.MHSActivityVersion {
			return nil, time.Time{}, false, nil
		}
		if at.IsZero() || m.ComputedAt.Before(at) {
			at = m.ComputedAt
		}
		partial = partial || m.Partial
	}
	docs, err := h.Store.ListDays(ctx, mhsactivitydays.DaysQuery{WorkspaceID: wsID, From: day, To: day, OrgIDs: z.OrgIDs})
	if err != nil {
		return nil, time.Time{}, false, err
	}
	if docs == nil {
		docs = []models.MHSActivityDay{}
	}
	return docs, at, partial, nil
}

// freshness collects where each zone-day came from and says it in a line or
// two instead of one entry per zone-day.
type freshness struct {
	storedDays   map[string]bool
	storedOldest time.Time
	storedNewest time.Time
	partial      bool
	liveDays     map[string][]string // day → "zone h:mm:ss PM MST"
}

func (f *freshness) stored(day string, at time.Time, partial bool) {
	if f.storedDays == nil {
		f.storedDays = map[string]bool{}
	}
	f.storedDays[day] = true
	if f.storedOldest.IsZero() || at.Before(f.storedOldest) {
		f.storedOldest = at
	}
	if at.After(f.storedNewest) {
		f.storedNewest = at
	}
	f.partial = f.partial || partial
}

func (f *freshness) live(day string, z activity.Zone, at time.Time) {
	if f.liveDays == nil {
		f.liveDays = map[string][]string{}
	}
	f.liveDays[day] = append(f.liveDays[day], z.TZ+" "+at.In(z.Loc).Format("3:04:05 PM MST"))
}

func (f *freshness) lines() []string {
	var out []string
	if len(f.storedDays) > 0 {
		days := make([]string, 0, len(f.storedDays))
		for d := range f.storedDays {
			days = append(days, d)
		}
		sort.Strings(days)
		span := prettyDay(days[0])
		if len(days) > 1 {
			span += " – " + prettyDay(days[len(days)-1])
		}
		line := fmt.Sprintf("Stored: %s (closed days, stored once; last stored %s UTC)", span, f.storedNewest.UTC().Format("Jan 2 15:04"))
		if f.partial {
			line += "; some of it was computed with a source unavailable"
		}
		out = append(out, line)
	}
	if len(f.liveDays) > 0 {
		days := make([]string, 0, len(f.liveDays))
		for d := range f.liveDays {
			days = append(days, d)
		}
		sort.Strings(days)
		for _, d := range days {
			out = append(out, fmt.Sprintf("Live: %s, computed %s (reused for a minute)", prettyDay(d), strings.Join(f.liveDays[d], " · ")))
		}
	}
	return out
}
