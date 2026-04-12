// internal/app/features/workspaces/maintenance.go
package workspaces

import (
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"golang.org/x/net/context"
)

type maintenanceVM struct {
	viewdata.BaseVM
	MaintenanceEnabled bool
	Message            string
	Error              string
	Success            string
}

// ServeMaintenance renders the maintenance mode settings page.
// GET /workspaces/maintenance
func (h *Handler) ServeMaintenance(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	gs, err := h.GlobalSettingsStore.Get(ctx)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "load global settings", err, "Failed to load settings.", "/workspaces")
		return
	}

	vm := maintenanceVM{
		BaseVM:             viewdata.NewBaseVM(r, h.DB, "Maintenance Mode", "/workspaces"),
		MaintenanceEnabled: gs.MaintenanceMode,
		Message:            gs.MaintenanceMessage,
	}
	templates.Render(w, r, "workspace_maintenance", vm)
}

// HandleMaintenance processes the maintenance mode form submission.
// POST /workspaces/maintenance
func (h *Handler) HandleMaintenance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form", err, "Invalid form data.", "/workspaces/maintenance")
		return
	}

	enabled := r.FormValue("maintenance_mode") == "on"
	message := strings.TrimSpace(r.FormValue("maintenance_message"))

	_, userName, _, _ := authz.UserCtx(r)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	gs := models.GlobalSettings{
		MaintenanceMode:    enabled,
		MaintenanceMessage: message,
		UpdatedByName:      userName,
	}

	if err := h.GlobalSettingsStore.Save(ctx, gs); err != nil {
		h.ErrLog.LogServerError(w, r, "save maintenance settings", err, "Failed to save settings.", "/workspaces/maintenance")
		return
	}

	// Re-render with success message
	vm := maintenanceVM{
		BaseVM:             viewdata.NewBaseVM(r, h.DB, "Maintenance Mode", "/workspaces"),
		MaintenanceEnabled: enabled,
		Message:            message,
		Success:            "Maintenance settings saved.",
	}
	templates.Render(w, r, "workspace_maintenance", vm)
}
