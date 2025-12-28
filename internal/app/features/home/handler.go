package home

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
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

/*─────────────────────────────────────────────────────────────────────────────*
| GET / – landing                                                             |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) ServeRoot(w http.ResponseWriter, r *http.Request) {
	data := struct {
		viewdata.BaseVM
	}{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "Welcome", "/"),
	}

	templates.Render(w, r, "home", data)
}
