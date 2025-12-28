// internal/app/features/login/handler.go
package login

import (
	"context"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/gorilla/securecookie"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

type Handler struct {
	DB         *mongo.Database
	Log        *zap.Logger
	SessionMgr *auth.SessionManager
	ErrLog     *uierrors.ErrorLogger
}

/*─────────────────────────────────────────────────────────────────────────────*
| Template-data                                                               |
*─────────────────────────────────────────────────────────────────────────────*/

type loginFormData struct {
	viewdata.BaseVM
	Error     string
	Email     string
	ReturnURL string
}

func NewHandler(db *mongo.Database, sessionMgr *auth.SessionManager, errLog *uierrors.ErrorLogger, logger *zap.Logger) *Handler {
	return &Handler{
		DB:         db,
		Log:        logger,
		SessionMgr: sessionMgr,
		ErrLog:     errLog,
	}
}

/*─────────────────────────────────────────────────────────────────────────────*
| GET /login                                                                  |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) ServeLogin(w http.ResponseWriter, r *http.Request) {
	ret := query.Get(r, "return")

	templates.Render(w, r, "login", loginFormData{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Login", "/"),
		ReturnURL: ret,
	})
}

/*─────────────────────────────────────────────────────────────────────────────*
| POST /login                                                                 |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) HandleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/login")
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		h.renderFormWithError(w, r, "Please enter an email address.", email)
		return
	}

	/*── look-up user (case-insensitive) ────────────────────────────────────*/

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
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

	// Emails are stored lowercase - normalize input and do direct match (index-friendly)
	emailNorm := normalize.Email(email)
	err := userColl.FindOne(
		ctx,
		bson.M{"email": emailNorm},
		proj,
	).Decode(&u)

	switch err {
	case mongo.ErrNoDocuments:
		h.renderFormWithError(w, r, "No account found for that email address.", email)
		return
	case nil:
		// found – continue
	default:
		h.ErrLog.LogServerError(w, r, "DB find user", err, "A server error occurred.", "/login")
		return
	}

	/*── check status: disabled users cannot log in ────────────────────────*/

	if normalize.Status(u.Status) == "disabled" {
		h.renderFormWithError(
			w,
			r,
			"Your account is currently disabled. Please contact an administrator.",
			email,
		)
		return
	}

	/*── create session ─────────────────────────────────────────────────────*/
	// We only store the authentication flag and user ID in the session.
	// Fresh user data (name, email, role, org) is fetched from the database
	// on each request by the LoadSessionUser middleware. This ensures that
	// role changes, disabled accounts, and profile updates take effect immediately.

	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		// Distinguish between expected cookie errors and unexpected store failures.
		// Cookie decode errors (corrupted, tampered, expired) are normal and expected.
		// Other errors could indicate session store backend issues.
		if scErr, ok := err.(securecookie.Error); ok && scErr.IsDecode() {
			h.Log.Warn("session cookie invalid, using fresh session",
				zap.Error(err),
				zap.String("email", email))
		} else {
			// Unexpected error - could be store backend failure or usage error.
			// Log as error but continue with fresh session to avoid blocking login.
			h.Log.Error("session store error during login, using fresh session",
				zap.Error(err),
				zap.String("email", email))
		}
	}
	sess.Values["is_authenticated"] = true
	sess.Values["user_id"] = u.ID.Hex()

	if err := sess.Save(r, w); err != nil {
		h.Log.Error("save session failed", zap.Error(err), zap.String("email", email))
		h.renderFormWithError(w, r, "Unable to create session. Please try again.", email)
		return
	}

	/*── redirect to return URL or dashboard ──────────────────────────────*/

	ret := strings.TrimSpace(r.FormValue("return"))
	// SafeReturn ensures we only navigate to a safe, local path.
	// Fallback is the unified /dashboard entrypoint.
	dest := urlutil.SafeReturn(ret, "", "/dashboard")
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

/*─────────────────────────────────────────────────────────────────────────────*
| helper: render the form with an error                                       |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) renderFormWithError(w http.ResponseWriter, r *http.Request, msg, email string) {
	// From POST, "return" will be in the form; from GET, we might rely on the query.
	ret := strings.TrimSpace(r.FormValue("return"))
	if ret == "" {
		ret = query.Get(r, "return")
	}

	templates.Render(w, r, "login", loginFormData{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Login", "/"),
		Error:     msg,
		Email:     email,
		ReturnURL: ret,
	})
}
