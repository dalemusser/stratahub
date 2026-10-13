package mhsactivity

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	activity "github.com/dalemusser/stratahub/internal/app/system/mhsactivity"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/waffle/pantry/templates"
)

// ServeAdmin shows the backfill form and the current backfill's progress.
func (h *Handler) ServeAdmin(w http.ResponseWriter, r *http.Request) {
	if !authz.IsAdmin(r) {
		uierrors.RenderForbidden(w, r, "Administrators only.", "/mhsactivity")
		return
	}
	h.renderAdmin(w, r, "")
}

// ServeAdminStatus is the progress fragment the page polls while a backfill runs.
func (h *Handler) ServeAdminStatus(w http.ResponseWriter, r *http.Request) {
	if !authz.IsAdmin(r) {
		uierrors.RenderForbidden(w, r, "Administrators only.", "/mhsactivity")
		return
	}
	templates.RenderSnippet(w, "mhsactivity_admin_status", h.adminVM(r, ""))
}

// HandleBackfill starts a backfill of a day range for the request's workspace.
func (h *Handler) HandleBackfill(w http.ResponseWriter, r *http.Request) {
	if !authz.IsAdmin(r) {
		uierrors.RenderForbidden(w, r, "Administrators only.", "/mhsactivity")
		return
	}
	if err := r.ParseForm(); err != nil {
		uierrors.RenderBadRequest(w, r, "Bad form.", "/mhsactivity/admin")
		return
	}
	req := activity.BackfillRequest{
		WorkspaceID: workspace.IDFromRequest(r),
		From:        strings.TrimSpace(r.Form.Get("from")),
		To:          strings.TrimSpace(r.Form.Get("to")),
		Stamp:       r.Form.Get("stamp") == "1",
		Force:       r.Form.Get("force") == "1",
	}
	startErr := ""
	if _, err := time.Parse(activity.DayFormat, req.From); err != nil {
		startErr = "The from date must be a day (2006-01-02)."
	} else if _, err := time.Parse(activity.DayFormat, req.To); err != nil {
		startErr = "The to date must be a day (2006-01-02)."
	} else if err := h.Backfill.Start(req); err != nil {
		startErr = err.Error()
	}
	if r.Header.Get("HX-Request") != "" {
		templates.RenderSnippet(w, "mhsactivity_admin_status", h.adminVM(r, startErr))
		return
	}
	h.renderAdmin(w, r, startErr)
}

// HandleBackfillStop cancels the running backfill.
func (h *Handler) HandleBackfillStop(w http.ResponseWriter, r *http.Request) {
	if !authz.IsAdmin(r) {
		uierrors.RenderForbidden(w, r, "Administrators only.", "/mhsactivity")
		return
	}
	h.Backfill.Stop()
	if r.Header.Get("HX-Request") != "" {
		templates.RenderSnippet(w, "mhsactivity_admin_status", h.adminVM(r, ""))
		return
	}
	http.Redirect(w, r, "/mhsactivity/admin", http.StatusSeeOther)
}

func (h *Handler) renderAdmin(w http.ResponseWriter, r *http.Request, startErr string) {
	templates.RenderAutoMap(w, r, "mhsactivity_admin", map[string]string{"mhsactivity-backfill-status": "mhsactivity_admin_status"}, h.adminVM(r, startErr))
}

func (h *Handler) adminVM(r *http.Request, startErr string) adminVM {
	now := h.now()
	yesterday := now.UTC().AddDate(0, 0, -1).Format(activity.DayFormat)
	from := h.Config.BackfillStart
	if from == "" {
		from = yesterday
	}
	vm := adminVM{
		BaseVM:     viewdata.NewBaseVM(r, h.DB, "MHS Activity: stored days", "/mhsactivity"),
		From:       from,
		To:         yesterday,
		StartError: startErr,
	}
	st := h.Backfill.Status()
	vm.Status = backfillVM{
		Running: st.Running,
		Ran:     !st.StartedAt.IsZero(),
		Total:   st.Total,
		Done:    st.Done,
		Skipped: st.Skipped,
		Open:    st.Open,
		Written: st.Written,
		Stamped: st.Stamped,
		Current: st.Current,
		Errors:  st.Errors,
		Stopped: st.Stopped,
	}
	if !st.StartedAt.IsZero() {
		vm.Status.Range = st.Request.From + " to " + st.Request.To
		var opts []string
		if st.Request.Stamp {
			opts = append(opts, "stamping summaries")
		}
		if st.Request.Force {
			opts = append(opts, "recomputing final days")
		}
		vm.Status.Options = strings.Join(opts, ", ")
		vm.Status.StartedAt = st.StartedAt.Format("Jan 2 15:04:05 MST")
		if st.FinishedAt != nil {
			vm.Status.FinishedAt = st.FinishedAt.Format("Jan 2 15:04:05 MST")
		}
		if st.Total > 0 {
			vm.Status.Percent = st.Done * 100 / st.Total
		}
	}
	if st.Running && st.Total > 0 {
		vm.Status.Current = fmt.Sprintf("%s (%d of %d)", st.Current, st.Done, st.Total)
	}
	return vm
}
