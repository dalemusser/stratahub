// internal/app/features/login/handler.go
package login

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/http/webutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

const loginTimeout = 5 * time.Second

type Handler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

/*─────────────────────────────────────────────────────────────────────────────*
| Template-data                                                               |
*─────────────────────────────────────────────────────────────────────────────*/

type loginFormData struct {
	Title      string
	Error      string
	IsLoggedIn bool
	Role       string
	UserName   string
	Email      string
	ReturnURL  string
}

func NewHandler(db *mongo.Database, logger *zap.Logger) *Handler {
	return &Handler{
		DB:  db,
		Log: logger,
	}
}

/*─────────────────────────────────────────────────────────────────────────────*
| GET /login                                                                  |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) ServeLogin(w http.ResponseWriter, r *http.Request) {
	user, logged := auth.CurrentUser(r)

	role, userName := "visitor", ""
	if logged {
		role = strings.ToLower(user.Role)
		userName = user.Name
	}

	ret := strings.TrimSpace(r.URL.Query().Get("return"))

	templates.Render(w, r, "login", loginFormData{
		Title:      "Login to Adroit Games",
		IsLoggedIn: logged,
		Role:       role,
		UserName:   userName,
		ReturnURL:  ret,
	})
}

/*─────────────────────────────────────────────────────────────────────────────*
| POST /login                                                                 |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) HandleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		h.renderFormWithError(w, r, "Please enter an email address.", email)
		return
	}

	/*── look-up user (case-insensitive) ────────────────────────────────────*/

	ctx, cancel := context.WithTimeout(r.Context(), loginTimeout)
	defer cancel()

	var u models.User
	userColl := h.DB.Collection("users")

	proj := options.FindOne().SetProjection(bson.M{
		"full_name":       1,
		"email":           1,
		"role":            1,
		"auth_method":     1,
		"organization_id": 1,
		"status":          1, // needed for disabled check
	})

	//   ^<email>$   anchored, quoted, case-insensitive
	re := fmt.Sprintf("^%s$", regexp.QuoteMeta(email))
	err := userColl.FindOne(
		ctx,
		bson.M{"email": bson.M{"$regex": re, "$options": "i"}},
		proj,
	).Decode(&u)

	switch err {
	case mongo.ErrNoDocuments:
		h.renderFormWithError(w, r, "No account found for that email address.", email)
		return
	case nil:
		// found – continue
	default:
		h.Log.Error("DB find user", zap.String("email", email), zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	/*── check status: disabled users cannot log in ────────────────────────*/

	if strings.ToLower(strings.TrimSpace(u.Status)) == "disabled" {
		h.renderFormWithError(
			w,
			r,
			"Your account is currently disabled. Please contact an administrator.",
			email,
		)
		return
	}

	/*── create session ─────────────────────────────────────────────────────*/

	// NOTE: assumes auth.Store has been initialized via InitSessionStore.
	sess, _ := auth.Store.Get(r, auth.SessionName)
	sess.Values["is_authenticated"] = true
	sess.Values["user_id"] = u.ID.Hex()
	sess.Values["user_name"] = u.FullName
	sess.Values["user_email"] = u.Email

	role := strings.ToLower(u.Role)
	sess.Values["user_role"] = role

	orgIDHex := ""
	if u.OrganizationID != nil {
		orgIDHex = u.OrganizationID.Hex()
	}
	sess.Values["organization_id"] = orgIDHex

	orgName := ""
	if orgIDHex != "" {
		orgColl := h.DB.Collection("organizations")
		var org models.Organization
		if err := orgColl.FindOne(ctx, bson.M{"_id": u.OrganizationID}).Decode(&org); err == nil {
			orgName = org.Name
		}
	}
	sess.Values["organization_name"] = orgName

	if err := sess.Save(r, w); err != nil {
		h.Log.Error("save session", zap.Error(err))
	}

	/*── redirect to return URL or dashboard ──────────────────────────────*/

	ret := strings.TrimSpace(r.FormValue("return"))
	// SafeReturn ensures we only navigate to a safe, local path.
	// Fallback is the unified /dashboard entrypoint.
	dest := webutil.SafeReturn(ret, "", "/dashboard")
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

/*─────────────────────────────────────────────────────────────────────────────*
| helper: render the form with an error                                       |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) renderFormWithError(w http.ResponseWriter, r *http.Request, msg, email string) {
	user, logged := auth.CurrentUser(r)

	role, userName := "visitor", ""
	if logged {
		role = strings.ToLower(user.Role)
		userName = user.Name
	}

	// From POST, "return" will be in the form; from GET, we might rely on the query.
	ret := strings.TrimSpace(r.FormValue("return"))
	if ret == "" {
		ret = strings.TrimSpace(r.URL.Query().Get("return"))
	}

	templates.Render(w, r, "login", loginFormData{
		Title:      "Login to Adroit Games",
		IsLoggedIn: logged,
		Role:       role,
		UserName:   userName,
		Error:      msg,
		Email:      email,
		ReturnURL:  ret,
	})
}
