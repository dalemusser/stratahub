// internal/features/login/routes.go
package login

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dalemusser/gowebcore/logger"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dalemusser/stratahub/internal/layout"
	"github.com/dalemusser/stratahub/internal/platform/handler"
)

/*──────────────────── embedded templates ────────────────────*/

//go:embed templates/*.html
var views embed.FS

var (
	tplOnce sync.Once
	tpl     *template.Template
)

func parseTemplates() *template.Template {
	t, _ := template.New("").ParseFS(layout.Views, "templates/*.html") // base + menu
	t.ParseFS(views, "templates/*.html")                               // slice views
	return t
}

/*──────────────────── view-model for form ─────────────────────*/

type formData struct {
	Title                 string
	Error                 string
	IsLoggedIn            bool
	Role, UserName, Email string
}

/*──────────────────────── route handler ──────────────────────*/

type LoginHandler struct{ h *handler.Handler }

func MountRoutes(r chi.Router, h *handler.Handler) {
	lh := &LoginHandler{h: h}
	r.Get("/login", lh.showForm)
	r.Post("/login", lh.handlePost)
}

/*────────── GET /login ───────────*/

func (lh *LoginHandler) showForm(w http.ResponseWriter, r *http.Request) {
	lh.render(w, r, formData{
		Title:      "Login to StrataHub",
		IsLoggedIn: lh.h.Session.IsAuth(r),
		Role:       lh.h.Session.Role(r),
		UserName:   lh.h.Session.UserName(r),
	})
}

/*────────── POST /login ──────────*/

func (lh *LoginHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		lh.renderErr(w, r, "Please enter an email address.", email)
		return
	}

	/*────────── lookup user (case-insensitive) ──────────*/

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userColl := lh.h.DB.DB("primary").Collection("users")

	var u struct {
		ID       any    `bson:"_id"`
		FullName string `bson:"full_name"`
		Email    string `bson:"email"`
		Role     string `bson:"role"`
	}

	re := bson.M{"$regex": "^" + regexp.QuoteMeta(email) + "$", "$options": "i"}
	err := userColl.FindOne(ctx,
		bson.M{"email": re},
		options.FindOne().SetProjection(bson.M{
			"full_name": 1, "email": 1, "role": 1,
		}),
	).Decode(&u)

	switch err {
	case mongo.ErrNoDocuments:
		lh.renderErr(w, r, "No account found for that email address.", email)
		return
	case nil:
		// found – continue
	default:
		logger.Error("db find user", "email", email, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	/*────────── create session & redirect ──────────*/

	lh.h.Session.Login(
		w, r,
		u.ID, u.FullName, u.Email,
		u.Role, nil, "", // org fields not used here
	)

	switch strings.ToLower(u.Role) {
	case "admin":
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
	case "leader":
		http.Redirect(w, r, "/leader/dashboard", http.StatusSeeOther)
	default: // player / visitor
		http.Redirect(w, r, "/player/dashboard", http.StatusSeeOther)
	}
}

/*──────────────────── helper renders ───────────────────────*/

func (lh *LoginHandler) render(w http.ResponseWriter, r *http.Request, d formData) {
	tplOnce.Do(func() { tpl = parseTemplates() })
	_ = tpl.ExecuteTemplate(w, "base", d)
}

func (lh *LoginHandler) renderErr(w http.ResponseWriter, r *http.Request, msg, email string) {
	lh.render(w, r, formData{
		Title:      "Login to StrataHub",
		Error:      msg,
		Email:      email,
		IsLoggedIn: lh.h.Session.IsAuth(r),
		Role:       lh.h.Session.Role(r),
		UserName:   lh.h.Session.UserName(r),
	})
}
