package home

import (
	"context"
	"html/template"
	"net/http"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler holds dependencies needed to serve the home page.
type Handler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

func NewHandler(db *mongo.Database, logger *zap.Logger) *Handler {
	return &Handler{
		DB:  db,
		Log: logger,
	}
}

// homeVM is the view model for the landing page.
type homeVM struct {
	viewdata.BaseVM
	LandingTitle string        // Title for landing page
	Content      template.HTML // Landing page content (HTML)
	CanEdit      bool          // True if user can edit the landing page
}

/*─────────────────────────────────────────────────────────────────────────────*
| GET / – landing                                                             |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) ServeRoot(w http.ResponseWriter, r *http.Request) {
	baseVM := viewdata.NewBaseVM(r, h.DB, "Welcome", "/")

	// Check if user can edit (admin or superadmin)
	role, _, _, _ := authz.UserCtx(r)
	canEdit := role == "admin" || role == "superadmin"

	// Get landing page title and content from settings
	var landingTitle string
	var content template.HTML

	wsID := workspace.IDFromRequest(r)
	if wsID.IsZero() {
		// No workspace context - use defaults
		landingTitle = models.DefaultLandingTitle
		content = template.HTML(models.DefaultLandingContent)
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
		defer cancel()

		store := settingsstore.New(h.DB)
		settings, err := store.Get(ctx, wsID)
		if err != nil {
			h.Log.Warn("failed to load settings for landing page", zap.Error(err))
			landingTitle = models.DefaultLandingTitle
			content = template.HTML(models.DefaultLandingContent)
		} else {
			// Settings store returns defaults if no document exists
			landingTitle = settings.LandingTitle
			if landingTitle == "" {
				landingTitle = models.DefaultLandingTitle
			}
			if settings.LandingContent == "" {
				content = template.HTML(models.DefaultLandingContent)
			} else {
				content = template.HTML(settings.LandingContent)
			}
		}
	}

	data := homeVM{
		BaseVM:       baseVM,
		LandingTitle: landingTitle,
		Content:      content,
		CanEdit:      canEdit,
	}

	templates.Render(w, r, "home", data)
}
