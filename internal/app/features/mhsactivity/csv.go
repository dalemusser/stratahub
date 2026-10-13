package mhsactivity

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ServeSchoolsCSV writes the schools table of the current window.
func (h *Handler) ServeSchoolsCSV(w http.ResponseWriter, r *http.Request) {
	vm, ok := h.load(w, r, primitive.NilObjectID)
	if !ok {
		return
	}
	cw := h.startCSV(w, "mhs-activity-schools-"+fileSlug(vm.Window.Label)+".csv")
	defer cw.Flush()
	if err := cw.Write([]string{"window", "organization", "time_zone", "students", "students_opened", "students_played", "students_blocked", "student_days",
		"downloads_complete", "downloads_held_then_done", "downloads_held", "downloads_failed", "downloads_cached", "downloads_unfinished",
		"launches_ok", "launches_slow", "launches_slow_recovered", "launches_failed", "crashes_real", "crash_notices",
		"devices_seen", "devices_new", "new_profile_every_day", "bgfetch_refused_students", "platforms", "last_activity"}); err != nil {
		return
	}
	for _, s := range vm.Report.Schools {
		if err := cw.Write(cleanRow([]string{vm.Window.Label, s.Name, s.TZ, n(s.Students), n(s.Opened), n(s.Played), n(s.Blocked), n(s.StudentDays),
			n(s.DlComplete), n(s.DlHeldDone), n(s.DlHeld), n(s.DlFailed), n(s.DlCached), n(s.DlInProgress),
			n(s.LaunchOK), n(s.LaunchStalled), n(s.LaunchRecovered), n(s.LaunchFailed), n(s.CrashReal), n(s.CrashNotice),
			n(s.DevicesSeen), n(s.DevicesNew), yesNo(s.Churn), n(s.BgfetchRefused), s.Platforms, s.LastActivity})); err != nil {
			h.Log.Warn("mhs activity csv", zap.Error(err))
			return
		}
	}
}

// ServeStudentsCSV writes one row per student-day and download episode of
// the current window (a student-day without downloads is one row with the
// unit columns empty). Staff rows are included when the page shows them.
func (h *Handler) ServeStudentsCSV(w http.ResponseWriter, r *http.Request) {
	vm, ok := h.load(w, r, primitive.NilObjectID)
	if !ok {
		return
	}
	cw := h.startCSV(w, "mhs-activity-students-"+fileSlug(vm.Window.Label)+".csv")
	defer cw.Flush()
	if err := cw.Write([]string{"day", "time_zone", "organization", "groups", "user_id", "name", "role", "opened", "played", "blocked",
		"unit", "unit_state", "device_id", "total_mb", "max_mb", "drops", "stalls", "retries", "fails", "last_error_class", "complete_mb", "complete_seconds", "held_at_mb", "hold_seconds", "bgfetch_refused",
		"launches_ok", "launches_slow", "launches_slow_recovered", "launches_failed", "crashes_real", "crash_notices",
		"devices", "new_devices", "platforms", "current_unit", "first_at_utc", "last_at_utc", "page_views", "load_records", "partial"}); err != nil {
		return
	}
	for _, d := range vm.docs {
		if d.Role == models.MHSActivityRoleStaff && !vm.ShowStaff {
			continue
		}
		base := h.studentDayRow(vm, d)
		if len(d.Units) == 0 {
			if err := cw.Write(cleanRow(append(base.head, append(make([]string, 15), base.tail...)...))); err != nil {
				return
			}
			continue
		}
		for _, u := range d.Units {
			unit := []string{u.Unit, u.State, u.DeviceID, n(u.TotalMB), n(u.MaxMB), n(u.Drops), n(u.Stalls), n(u.Retries), n(u.Fails), u.LastErrorClass, n(u.CompleteMB), n(u.CompleteSeconds), n(u.HeldAtMB), n(u.HoldSeconds), yesNo(u.BgfetchRefused)}
			if err := cw.Write(cleanRow(append(base.head, append(unit, base.tail...)...))); err != nil {
				return
			}
		}
	}
}

type csvRowParts struct {
	head []string
	tail []string
}

func (h *Handler) studentDayRow(vm *pageVM, d models.MHSActivityDay) csvRowParts {
	groups := make([]string, 0, len(d.GroupIDs))
	for _, g := range d.GroupIDs {
		groups = append(groups, vm.lk.GroupName(g))
	}
	platforms := map[string]bool{}
	newDevices := 0
	for _, dev := range d.Devices {
		if dev.Platform != "" {
			platforms[dev.Platform] = true
		}
		if dev.New {
			newDevices++
		}
	}
	plats := make([]string, 0, len(platforms))
	for p := range platforms {
		plats = append(plats, p)
	}
	first, last := "", ""
	if d.FirstAt != nil {
		first = d.FirstAt.UTC().Format("2006-01-02 15:04:05")
	}
	if d.LastAt != nil {
		last = d.LastAt.UTC().Format("2006-01-02 15:04:05")
	}
	return csvRowParts{
		head: []string{d.Day, d.TZ, vm.lk.OrgName(d.OrganizationID), strings.Join(groups, "; "), d.UserID.Hex(), vm.lk.UserName(d.UserID), d.Role, yesNo(d.Opened), yesNo(d.Played), yesNo(d.Blocked)},
		tail: []string{n(d.Launch.OK), n(d.Launch.Stalled), n(d.Launch.StalledRecovered), n(d.Launch.Failed), n(d.Crashes.Real), n(d.Crashes.Notice),
			n(len(d.Devices)), n(newDevices), strings.Join(plats, "; "), d.CurrentUnit, first, last, n(d.PageViews), n(d.Records), yesNo(d.Partial)},
	}
}

func (h *Handler) startCSV(w http.ResponseWriter, filename string) *csv.Writer {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM for Excel
	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	return cw
}

var fileSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

func fileSlug(s string) string {
	return strings.Trim(fileSlugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func n(i int) string { return strconv.Itoa(i) }

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// cleanRow keeps a cell from being read as a formula by a spreadsheet.
func cleanRow(cells []string) []string {
	for i, c := range cells {
		if len(c) > 0 {
			switch c[0] {
			case '=', '+', '-', '@':
				cells[i] = "'" + c
			}
		}
	}
	return cells
}
