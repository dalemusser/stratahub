// internal/app/features/workspaces/new.go
package workspaces

import (
	"context"
	"html/template"
	"net/http"
	"regexp"
	"strings"

	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.uber.org/zap"
)

// SubdomainRegex validates subdomain format: lowercase alphanumeric and hyphens only.
var SubdomainRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ReservedSubdomains are subdomains that cannot be used for workspaces.
var ReservedSubdomains = []string{"www", "api", "admin", "app", "mail", "ftp", "localhost"}

// createWorkspaceInput defines validation rules for creating a workspace.
type createWorkspaceInput struct {
	Name      string `validate:"required,max=100" label:"Name"`
	Subdomain string `validate:"required,max=63" label:"Subdomain"`
}

// ServeNew renders the new workspace form.
func (h *Handler) ServeNew(w http.ResponseWriter, r *http.Request) {
	data := newData{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "New Workspace", "/workspaces"),
		Domain: h.PrimaryDomain,
	}
	templates.Render(w, r, "workspace_new", data)
}

// HandleCreate processes the new workspace form submission.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	_, _, actorID, ok := authz.UserCtx(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/workspaces")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	subdomain := strings.ToLower(strings.TrimSpace(r.FormValue("subdomain")))

	reRender := func(msg string) {
		templates.Render(w, r, "workspace_new", newData{
			BaseVM:    viewdata.NewBaseVM(r, h.DB, "New Workspace", "/workspaces"),
			Name:      name,
			Subdomain: subdomain,
			Domain:    h.PrimaryDomain,
			Error:     template.HTML(msg),
		})
	}

	// Validate input
	input := createWorkspaceInput{Name: name, Subdomain: subdomain}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	// Validate subdomain format
	if !SubdomainRegex.MatchString(subdomain) {
		reRender("Subdomain must contain only lowercase letters, numbers, and hyphens. It cannot start or end with a hyphen.")
		return
	}

	// Check reserved subdomains
	for _, r := range ReservedSubdomains {
		if subdomain == r {
			reRender("This subdomain is reserved and cannot be used.")
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	store := workspacestore.New(h.DB)

	// Create workspace
	ws := models.Workspace{
		Name:      name,
		Subdomain: subdomain,
		Status:    "active",
	}

	created, err := store.Create(ctx, ws)
	if err != nil {
		if err == workspacestore.ErrDuplicateName {
			reRender("A workspace with this name already exists.")
			return
		}
		if err == workspacestore.ErrDuplicateSubdomain {
			reRender("A workspace with this subdomain already exists.")
			return
		}
		h.ErrLog.LogServerError(w, r, "database error creating workspace", err, "A database error occurred.", "/workspaces")
		return
	}

	// Audit log
	h.Log.Info("workspace created",
		zap.String("workspace_id", created.ID.Hex()),
		zap.String("name", created.Name),
		zap.String("subdomain", created.Subdomain),
		zap.String("created_by", actorID.Hex()))

	http.Redirect(w, r, "/workspaces", http.StatusSeeOther)
}
