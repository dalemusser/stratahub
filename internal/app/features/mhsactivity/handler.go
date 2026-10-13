package mhsactivity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/reportpolicy"
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
	DB      *mongo.Database
	Compute activity.Deps
	Config  Config
	Log     *zap.Logger
	ErrLog  *uierrors.ErrorLogger

	cache *dayCache
	now   func() time.Time
}

// NewHandler wires the page to the three databases it reads.
func NewHandler(db, logDB, gradesDB *mongo.Database, errLog *uierrors.ErrorLogger, logger *zap.Logger, cfg Config) *Handler {
	if cfg.MaxLiveDays <= 0 {
		cfg.MaxLiveDays = 7
	}
	return &Handler{
		DB:      db,
		Compute: activity.Deps{DB: db, LogDB: logDB, GradesDB: gradesDB, Log: logger, CloseAfter: cfg.CloseAfter},
		Config:  cfg,
		Log:     logger,
		ErrLog:  errLog,
		cache:   newDayCache(cfg.CacheTTL),
		now:     func() time.Time { return time.Now().UTC() },
	}
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
	})
	return r
}

// ServePage renders the overview, narrowed by the query's org and group.
func (h *Handler) ServePage(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, primitive.NilObjectID)
}

// ServeSchool renders the overview with one school opened (the HTMX
// fragment when the request targets the school panel).
func (h *Handler) ServeSchool(w http.ResponseWriter, r *http.Request) {
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "orgID"))
	if err != nil {
		uierrors.RenderNotFound(w, r, "No such school.", "/mhsactivity")
		return
	}
	h.serve(w, r, id)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request, pathOrg primitive.ObjectID) {
	if _, _, _, ok := authz.UserCtx(r); !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	pol := reportpolicy.CanViewMHSActivity(r)
	if !pol.CanView {
		uierrors.RenderForbidden(w, r, "You do not have access to MHS Activity.", "/dashboard")
		return
	}
	if !h.Config.Enabled {
		uierrors.RenderNotFound(w, r, "MHS Activity is turned off.", "/dashboard")
		return
	}
	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "mhs activity")
	defer cancel()
	wsID := workspace.IDFromRequest(r)

	win, err := ParseWindow(r, h.Config.MaxLiveDays, h.now())
	if err != nil {
		uierrors.RenderBadRequest(w, r, err.Error(), "/mhsactivity")
		return
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
			return
		}
		h.fail(w, r, "resolve scope", err)
		return
	}
	orgOpts, err := scope.Orgs(ctx)
	if err != nil {
		h.fail(w, r, "list organizations", err)
		return
	}
	groupOpts, err := scope.Groups(ctx)
	if err != nil {
		h.fail(w, r, "list groups", err)
		return
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
		return
	}
	now := h.now()
	var docs []models.MHSActivityDay
	var fresh []string
	multiDay := false
	for _, z := range zones {
		days := win.DaysIn(z.Loc, now)
		if len(days) > 1 {
			multiDay = true
		}
		for _, day := range days {
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
				return
			}
			docs = append(docs, got...)
			fresh = append(fresh, fmt.Sprintf("%s (%s): computed %s", prettyDay(day), z.TZ, at.In(z.Loc).Format("3:04:05 PM MST")))
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
			return
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
		return
	}
	rep := BuildReport(docs, lk, Options{SelectedOrg: selectedOrg, SelectedGroup: scope.SelectedGroup, ShowStaff: showStaff, MultiDay: multiDay, Now: now})

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
		Window:        windowVM{Range: win.Range, From: win.From, To: win.To, Label: win.Label(), Custom: win.Range == RangeCustom, MaxDays: h.Config.MaxLiveDays},
		SelectedOrg:   hexOrEmpty(selectedOrg),
		SelectedGroup: hexOrEmpty(scope.SelectedGroup),
		ShowStaff:     showStaff,
		Freshness:     fresh,
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

	templates.RenderAutoMap(w, r, "mhsactivity_page", map[string]string{"mhsactivity-school": "mhsactivity_school"}, vm)
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
