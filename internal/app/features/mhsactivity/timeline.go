package mhsactivity

import (
	"net/http"
	"sort"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	"github.com/dalemusser/stratahub/internal/app/policy/reportpolicy"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicetests"
	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
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
)

// timelineVM is the student-day page.
type timelineVM struct {
	viewdata.BaseVM
	UserIDHex   string
	Name        string
	OrgName     string
	TZ          string
	Day         string
	DayLabel    string
	Records     int
	Devices     []string // device id tails, in order of first appearance
	Entries     []timelineEntry
	Prev        string // the day before, for the link
	Next        string
	BackHref    string
	RecordsHref string
}

type timelineEntry struct {
	Time   string // local clock
	Device string // tail of the device id
	Step   string
	State  string
	Class  string
	Msg    string
	Detail string
}

// ServeStudentDay shows one student's merged step log for one day: every
// entry from every load record stored for that day (and the six hours after
// it), de-duplicated across the overlapping records one tab posts, in time
// order, with the device each came from.
func (h *Handler) ServeStudentDay(w http.ResponseWriter, r *http.Request) {
	if _, _, _, ok := authz.UserCtx(r); !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if !reportpolicy.CanViewMHSActivity(r).CanView {
		uierrors.RenderForbidden(w, r, "You do not have access to MHS Activity.", "/dashboard")
		return
	}
	userID, err := primitive.ObjectIDFromHex(chi.URLParam(r, "userID"))
	if err != nil {
		uierrors.RenderNotFound(w, r, "No such member.", "/mhsactivity")
		return
	}
	day := chi.URLParam(r, "day")
	if _, err := time.Parse(activity.DayFormat, day); err != nil {
		uierrors.RenderNotFound(w, r, "No such day.", "/mhsactivity")
		return
	}
	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "mhs activity timeline")
	defer cancel()
	wsID := workspace.IDFromRequest(r)

	users, err := userstore.New(h.DB).GetByIDs(ctx, []primitive.ObjectID{userID})
	if err != nil {
		h.fail(w, r, "timeline user", err)
		return
	}
	if len(users) == 0 || users[0].OrganizationID == nil || (users[0].WorkspaceID != nil && *users[0].WorkspaceID != wsID) {
		uierrors.RenderNotFound(w, r, "No such member.", "/mhsactivity")
		return
	}
	u := users[0]
	// The member must be inside the viewer's scope (a coordinator's organizations).
	scope, err := viewscope.Resolve(ctx, h.DB, r, viewscope.Options{})
	if err != nil {
		uierrors.RenderForbidden(w, r, "You do not have access to MHS Activity.", "/dashboard")
		return
	}
	if !scope.Unrestricted() {
		allowed := false
		for _, id := range scope.OrgIDs {
			if id == *u.OrganizationID {
				allowed = true
			}
		}
		if !allowed {
			uierrors.RenderForbidden(w, r, "That member is outside your scope.", "/mhsactivity")
			return
		}
	}
	orgs, err := organizationstore.New(h.DB).GetByIDs(ctx, []primitive.ObjectID{*u.OrganizationID})
	if err != nil {
		h.fail(w, r, "timeline organization", err)
		return
	}
	loc, orgName := time.UTC, "Organization …"+tail(u.OrganizationID.Hex())
	if len(orgs) > 0 {
		loc, _ = activity.LocationFor(orgs[0].TimeZone)
		orgName = orgs[0].Name
	}
	start, end, _ := activity.DayBounds(day, loc)
	store := mhsdevicetests.New(h.DB)
	recs, err := store.ListMemberBetween(ctx, wsID, start, end.Add(h.Compute.EffectiveCloseAfter()))
	if err != nil {
		h.fail(w, r, "timeline records", err)
		return
	}
	var ids []primitive.ObjectID
	deviceOf := map[primitive.ObjectID]string{}
	for _, rec := range recs {
		if rec.UserID == nil || *rec.UserID != userID {
			continue
		}
		ids = append(ids, rec.ID)
		deviceOf[rec.ID] = rec.DeviceID
	}
	steps, err := store.StepsByIDs(ctx, wsID, ids)
	if err != nil {
		h.fail(w, r, "timeline steps", err)
		return
	}
	entries, devices := mergeSteps(ids, deviceOf, steps, day, loc)

	prev, _ := activity.AddDays(day, -1)
	next, _ := activity.AddDays(day, 1)
	vm := timelineVM{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "MHS Activity: "+u.FullName, "/mhsactivity"),
		UserIDHex: userID.Hex(),
		Name:      u.FullName,
		OrgName:   orgName,
		TZ:        loc.String(),
		Day:       day,
		DayLabel:  prettyDay(day),
		Records:   len(ids),
		Devices:   devices,
		Entries:   entries,
		Prev:      prev,
		Next:      next,
		BackHref:  "/mhsactivity?range=custom&from=" + day + "&to=" + day + "&org=" + u.OrganizationID.Hex(),
	}
	if authz.IsAdmin(r) || authz.IsAnalyst(r) {
		vm.RecordsHref = DeviceTestsHref(userID)
	}
	templates.Render(w, r, "mhsactivity_timeline", vm)
}

// mergeSteps joins the records' step logs into one list for the day: an
// entry is one (device, time, step, message); the overlapping records of one
// tab contribute it once. Devices are numbered in order of first appearance.
func mergeSteps(ids []primitive.ObjectID, deviceOf map[primitive.ObjectID]string, steps map[primitive.ObjectID][]models.MHSDeviceTestStep, day string, loc *time.Location) ([]timelineEntry, []string) {
	type keyed struct {
		at  time.Time
		dev string
		st  models.MHSDeviceTestStep
	}
	seen := map[string]bool{}
	var all []keyed
	for _, id := range ids {
		dev := deviceOf[id]
		for _, st := range steps[id] {
			if st.At.IsZero() || activity.LocalDay(st.At, loc) != day {
				continue
			}
			k := dev + "|" + st.At.UTC().Format(time.RFC3339Nano) + "|" + st.Step + "|" + st.Msg
			if seen[k] {
				continue
			}
			seen[k] = true
			all = append(all, keyed{st.At, dev, st})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
	var devices []string
	label := map[string]string{}
	entries := make([]timelineEntry, 0, len(all))
	for _, k := range all {
		if _, ok := label[k.dev]; !ok {
			devices = append(devices, tail(k.dev))
			label[k.dev] = tail(k.dev)
		}
		var detail []string
		for dk, dv := range k.st.Detail {
			detail = append(detail, dk+"="+dv)
		}
		sort.Strings(detail)
		entries = append(entries, timelineEntry{
			Time:   k.at.In(loc).Format("3:04:05 PM"),
			Device: label[k.dev],
			Step:   k.st.Step,
			State:  k.st.State,
			Class:  stateClass(k.st.State),
			Msg:    k.st.Msg,
			Detail: strings.Join(detail, " · "),
		})
	}
	return entries, devices
}

func stateClass(state string) string {
	switch state {
	case "fail":
		return viewers.PillAmber
	case "warn":
		return viewers.PillGray
	case "ok":
		return viewers.PillGreen
	}
	return viewers.TextMuted
}
