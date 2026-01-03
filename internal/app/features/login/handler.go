// internal/app/features/login/handler.go
package login

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/emailverify"
	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/mailer"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/gorilla/securecookie"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

type Handler struct {
	DB          *mongo.Database
	Log         *zap.Logger
	SessionMgr  *auth.SessionManager
	ErrLog      *uierrors.ErrorLogger
	Mailer      *mailer.Mailer
	EmailVerify *emailverify.Store
	BaseURL     string // Base URL for magic links (e.g., "https://stratahub.com")
}

/*─────────────────────────────────────────────────────────────────────────────*
| Template-data                                                               |
*─────────────────────────────────────────────────────────────────────────────*/

type loginFormData struct {
	viewdata.BaseVM
	Error     string
	LoginID   string // What the user typed (displayed as "email" in UI for backwards compat)
	ReturnURL string
}

type passwordFormData struct {
	viewdata.BaseVM
	Error     string
	LoginID   string // Display the login ID (read-only)
	ReturnURL string
}

type changePasswordFormData struct {
	viewdata.BaseVM
	Error         string
	LoginID       string // Display the login ID (read-only)
	ReturnURL     string
	PasswordRules string // Rules to display to user
}

type emailVerifyFormData struct {
	viewdata.BaseVM
	Error     string
	LoginID   string // Display the login ID (read-only)
	Email     string // Email where code was sent
	ReturnURL string
}

func NewHandler(db *mongo.Database, sessionMgr *auth.SessionManager, errLog *uierrors.ErrorLogger, mail *mailer.Mailer, baseURL string, logger *zap.Logger) *Handler {
	return &Handler{
		DB:          db,
		Log:         logger,
		SessionMgr:  sessionMgr,
		ErrLog:      errLog,
		Mailer:      mail,
		EmailVerify: emailverify.New(db),
		BaseURL:     baseURL,
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

	// Form field is still named "email" for backwards compatibility with UI
	loginID := strings.TrimSpace(r.FormValue("email"))
	if loginID == "" {
		h.renderFormWithError(w, r, "Please enter your login ID.", loginID)
		return
	}

	/*── look-up user by login_id_ci (case/diacritic-insensitive) ──────────*/

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	var u models.User
	userColl := h.DB.Collection("users")

	proj := options.FindOne().SetProjection(bson.M{
		"full_name":       1,
		"login_id":        1,
		"role":            1,
		"auth_method":     1,
		"organization_id": 1,
		"status":          1, // needed for disabled check
	})

	// Fold input for case/diacritic-insensitive matching via login_id_ci index
	loginIDCI := text.Fold(loginID)
	err := userColl.FindOne(
		ctx,
		bson.M{"login_id_ci": loginIDCI},
		proj,
	).Decode(&u)

	switch err {
	case mongo.ErrNoDocuments:
		h.renderFormWithError(w, r, "No account found for that login ID.", loginID)
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
			loginID,
		)
		return
	}

	/*── check auth method and route accordingly ────────────────────────────*/

	ret := strings.TrimSpace(r.FormValue("return"))
	authMethod := normalize.AuthMethod(u.AuthMethod)

	switch authMethod {
	case "trust":
		// Trust auth: create session immediately (current behavior)
		h.createSessionAndRedirect(w, r, &u, ret)

	case "password":
		// Password auth: store pending login in session, redirect to password page
		h.startPasswordFlow(w, r, &u, ret)

	case "email":
		// Email verification: send code and redirect to verification page
		h.startEmailFlow(w, r, &u, ret)

	case "google", "microsoft", "classlink", "clever", "schoology":
		// SSO methods (future feature)
		h.renderFormWithError(w, r, "This account uses "+authMethod+" sign-in, which is not yet available. Please contact an administrator.", loginID)

	default:
		h.renderFormWithError(w, r, "Unknown authentication method. Please contact an administrator.", loginID)
	}
}

// createSessionAndRedirect creates an authenticated session and redirects to the destination.
func (h *Handler) createSessionAndRedirect(w http.ResponseWriter, r *http.Request, u *models.User, returnURL string) {
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		if scErr, ok := err.(securecookie.Error); ok && scErr.IsDecode() {
			h.Log.Warn("session cookie invalid, using fresh session",
				zap.Error(err),
				zap.String("user_id", u.ID.Hex()))
		} else {
			h.Log.Error("session store error during login, using fresh session",
				zap.Error(err),
				zap.String("user_id", u.ID.Hex()))
		}
	}

	// Clear any pending login state
	delete(sess.Values, "pending_user_id")
	delete(sess.Values, "pending_login_id")

	// Set authenticated state
	sess.Values["is_authenticated"] = true
	sess.Values["user_id"] = u.ID.Hex()

	loginID := ""
	if u.LoginID != nil {
		loginID = *u.LoginID
	}

	if err := sess.Save(r, w); err != nil {
		h.Log.Error("save session failed", zap.Error(err), zap.String("login_id", loginID))
		h.renderFormWithError(w, r, "Unable to create session. Please try again.", loginID)
		return
	}

	// Set theme preference cookie for the layout script to apply on page load.
	// This cookie is read once by JavaScript to set localStorage, then cleared.
	themePref := u.ThemePreference
	if themePref == "" {
		themePref = "system"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "theme_pref",
		Value:    themePref,
		Path:     "/",
		MaxAge:   60, // Short-lived, just needs to survive the redirect
		HttpOnly: false, // JavaScript needs to read it
		SameSite: http.SameSiteLaxMode,
	})

	dest := urlutil.SafeReturn(returnURL, "", "/dashboard")
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// startPasswordFlow stores pending login info in session and redirects to password page.
func (h *Handler) startPasswordFlow(w http.ResponseWriter, r *http.Request, u *models.User, returnURL string) {
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		if scErr, ok := err.(securecookie.Error); ok && scErr.IsDecode() {
			h.Log.Warn("session cookie invalid, using fresh session", zap.Error(err))
		} else {
			h.Log.Error("session store error, using fresh session", zap.Error(err))
		}
	}

	loginID := ""
	if u.LoginID != nil {
		loginID = *u.LoginID
	}

	// Store pending login state (not yet authenticated)
	sess.Values["pending_user_id"] = u.ID.Hex()
	sess.Values["pending_login_id"] = loginID
	sess.Values["pending_return_url"] = returnURL

	// Ensure not authenticated yet
	delete(sess.Values, "is_authenticated")
	delete(sess.Values, "user_id")

	if err := sess.Save(r, w); err != nil {
		h.Log.Error("save session failed", zap.Error(err), zap.String("login_id", loginID))
		h.renderFormWithError(w, r, "Unable to create session. Please try again.", loginID)
		return
	}

	http.Redirect(w, r, "/login/password", http.StatusSeeOther)
}

/*─────────────────────────────────────────────────────────────────────────────*
| helper: render the form with an error                                       |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) renderFormWithError(w http.ResponseWriter, r *http.Request, msg, loginID string) {
	// From POST, "return" will be in the form; from GET, we might rely on the query.
	ret := strings.TrimSpace(r.FormValue("return"))
	if ret == "" {
		ret = query.Get(r, "return")
	}

	templates.Render(w, r, "login", loginFormData{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Login", "/"),
		Error:     msg,
		LoginID:   loginID,
		ReturnURL: ret,
	})
}

/*─────────────────────────────────────────────────────────────────────────────*
| GET /login/password                                                         |
*─────────────────────────────────────────────────────────────────────────────*/

// ServePasswordPage shows the password entry form.
func (h *Handler) ServePasswordPage(w http.ResponseWriter, r *http.Request) {
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Check for pending login
	pendingUserID, ok1 := sess.Values["pending_user_id"].(string)
	pendingLoginID, ok2 := sess.Values["pending_login_id"].(string)
	if !ok1 || !ok2 || pendingUserID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	returnURL, _ := sess.Values["pending_return_url"].(string)

	templates.Render(w, r, "login_password", passwordFormData{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Enter Password", "/login"),
		LoginID:   pendingLoginID,
		ReturnURL: returnURL,
	})
}

/*─────────────────────────────────────────────────────────────────────────────*
| POST /login/password                                                        |
*─────────────────────────────────────────────────────────────────────────────*/

// HandlePasswordSubmit validates the password and completes login.
func (h *Handler) HandlePasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/login")
		return
	}

	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Check for pending login
	pendingUserID, ok1 := sess.Values["pending_user_id"].(string)
	pendingLoginID, ok2 := sess.Values["pending_login_id"].(string)
	returnURL, _ := sess.Values["pending_return_url"].(string)
	if !ok1 || !ok2 || pendingUserID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	password := r.FormValue("password")
	if password == "" {
		h.renderPasswordFormWithError(w, r, "Please enter your password.", pendingLoginID, returnURL)
		return
	}

	// Load user from database to verify password
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	oid, err := parseObjectID(pendingUserID)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	var u models.User
	err = h.DB.Collection("users").FindOne(ctx, bson.M{"_id": oid}).Decode(&u)
	if err != nil {
		h.Log.Error("failed to load user for password check", zap.Error(err), zap.String("user_id", pendingUserID))
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Check password
	if u.PasswordHash == nil || *u.PasswordHash == "" {
		h.renderPasswordFormWithError(w, r, "No password set for this account. Please contact an administrator.", pendingLoginID, returnURL)
		return
	}

	if !authutil.CheckPassword(password, *u.PasswordHash) {
		h.renderPasswordFormWithError(w, r, "Incorrect password. Please try again.", pendingLoginID, returnURL)
		return
	}

	// Password correct - check if it's temporary
	if u.PasswordTemp != nil && *u.PasswordTemp {
		// Need to change password
		http.Redirect(w, r, "/login/change-password", http.StatusSeeOther)
		return
	}

	// Password correct and not temporary - complete login
	h.createSessionAndRedirect(w, r, &u, returnURL)
}

func (h *Handler) renderPasswordFormWithError(w http.ResponseWriter, r *http.Request, msg, loginID, returnURL string) {
	templates.Render(w, r, "login_password", passwordFormData{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Enter Password", "/login"),
		Error:     msg,
		LoginID:   loginID,
		ReturnURL: returnURL,
	})
}

/*─────────────────────────────────────────────────────────────────────────────*
| GET /login/change-password                                                  |
*─────────────────────────────────────────────────────────────────────────────*/

// ServeChangePassword shows the change password form.
func (h *Handler) ServeChangePassword(w http.ResponseWriter, r *http.Request) {
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Check for pending login
	pendingUserID, ok1 := sess.Values["pending_user_id"].(string)
	pendingLoginID, ok2 := sess.Values["pending_login_id"].(string)
	if !ok1 || !ok2 || pendingUserID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	returnURL, _ := sess.Values["pending_return_url"].(string)

	templates.Render(w, r, "login_change_password", changePasswordFormData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Change Password", "/login"),
		LoginID:       pendingLoginID,
		ReturnURL:     returnURL,
		PasswordRules: authutil.PasswordRules(),
	})
}

/*─────────────────────────────────────────────────────────────────────────────*
| POST /login/change-password                                                 |
*─────────────────────────────────────────────────────────────────────────────*/

// HandleChangePassword validates and saves the new password.
func (h *Handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/login")
		return
	}

	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Check for pending login
	pendingUserID, ok1 := sess.Values["pending_user_id"].(string)
	pendingLoginID, ok2 := sess.Values["pending_login_id"].(string)
	returnURL, _ := sess.Values["pending_return_url"].(string)
	if !ok1 || !ok2 || pendingUserID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate passwords match
	if newPassword != confirmPassword {
		h.renderChangePasswordFormWithError(w, r, "Passwords do not match.", pendingLoginID, returnURL)
		return
	}

	// Validate password requirements
	if err := authutil.ValidatePassword(newPassword); err != nil {
		h.renderChangePasswordFormWithError(w, r, err.Error(), pendingLoginID, returnURL)
		return
	}

	// Hash the new password
	hash, err := authutil.HashPassword(newPassword)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "hash password failed", err, "A server error occurred.", "/login")
		return
	}

	// Update the user's password in the database
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	oid, err := parseObjectID(pendingUserID)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	_, err = h.DB.Collection("users").UpdateOne(
		ctx,
		bson.M{"_id": oid},
		bson.M{
			"$set": bson.M{
				"password_hash": hash,
				"password_temp": false,
			},
		},
	)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "update password failed", err, "A server error occurred.", "/login")
		return
	}

	// Load user for session creation
	var u models.User
	err = h.DB.Collection("users").FindOne(ctx, bson.M{"_id": oid}).Decode(&u)
	if err != nil {
		h.Log.Error("failed to load user after password change", zap.Error(err), zap.String("user_id", pendingUserID))
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Complete login
	h.createSessionAndRedirect(w, r, &u, returnURL)
}

func (h *Handler) renderChangePasswordFormWithError(w http.ResponseWriter, r *http.Request, msg, loginID, returnURL string) {
	templates.Render(w, r, "login_change_password", changePasswordFormData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Change Password", "/login"),
		Error:         msg,
		LoginID:       loginID,
		ReturnURL:     returnURL,
		PasswordRules: authutil.PasswordRules(),
	})
}

// parseObjectID parses a hex string into a MongoDB ObjectID.
func parseObjectID(hex string) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(hex)
}

/*─────────────────────────────────────────────────────────────────────────────*
| Email verification flow                                                      |
*─────────────────────────────────────────────────────────────────────────────*/

// startEmailFlow creates a verification code/token and sends the email.
func (h *Handler) startEmailFlow(w http.ResponseWriter, r *http.Request, u *models.User, returnURL string) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get email from user's login_id (for email auth, login_id is the email)
	email := ""
	if u.LoginID != nil {
		email = *u.LoginID
	}
	if email == "" {
		h.renderFormWithError(w, r, "No email address found for this account.", "")
		return
	}

	// Create verification record
	result, err := h.EmailVerify.Create(ctx, u.ID, email)
	if err != nil {
		h.Log.Error("failed to create email verification", zap.Error(err), zap.String("email", email))
		h.renderFormWithError(w, r, "Failed to send verification email. Please try again.", email)
		return
	}

	// Get site name from settings
	siteName := "StrataHub" // default
	settings, err := settingsstore.New(h.DB).Get(ctx)
	if err == nil && settings.SiteName != "" {
		siteName = settings.SiteName
	}

	// Build magic link
	magicLink := fmt.Sprintf("%s/login/verify-email?token=%s", h.BaseURL, result.Token)

	// Build and send email
	emailData := mailer.VerificationEmailData{
		SiteName:  siteName,
		Code:      result.Code,
		MagicLink: magicLink,
		ExpiresIn: "10 minutes",
	}
	mailMsg := mailer.BuildVerificationEmail(emailData)
	mailMsg.To = email

	if err := h.Mailer.Send(mailMsg); err != nil {
		h.Log.Error("failed to send verification email", zap.Error(err), zap.String("email", email))
		h.renderFormWithError(w, r, "Failed to send verification email. Please try again.", email)
		return
	}

	h.Log.Info("verification email sent", zap.String("email", email), zap.String("user_id", u.ID.Hex()))

	// Store pending email login in session
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		if scErr, ok := err.(securecookie.Error); ok && scErr.IsDecode() {
			h.Log.Warn("session cookie invalid, using fresh session", zap.Error(err))
		} else {
			h.Log.Error("session store error, using fresh session", zap.Error(err))
		}
	}

	loginID := ""
	if u.LoginID != nil {
		loginID = *u.LoginID
	}

	// Store pending login state
	sess.Values["pending_user_id"] = u.ID.Hex()
	sess.Values["pending_login_id"] = loginID
	sess.Values["pending_email"] = email
	sess.Values["pending_return_url"] = returnURL

	// Ensure not authenticated yet
	delete(sess.Values, "is_authenticated")
	delete(sess.Values, "user_id")

	if err := sess.Save(r, w); err != nil {
		h.Log.Error("save session failed", zap.Error(err), zap.String("login_id", loginID))
		h.renderFormWithError(w, r, "Unable to create session. Please try again.", loginID)
		return
	}

	http.Redirect(w, r, "/login/verify-email", http.StatusSeeOther)
}

/*─────────────────────────────────────────────────────────────────────────────*
| GET /login/verify-email                                                      |
*─────────────────────────────────────────────────────────────────────────────*/

// ServeVerifyEmail handles both magic link verification and showing the code entry form.
func (h *Handler) ServeVerifyEmail(w http.ResponseWriter, r *http.Request) {
	// Check for magic link token in query params
	token := query.Get(r, "token")
	if token != "" {
		h.handleMagicLink(w, r, token)
		return
	}

	// No token - show code entry form
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Check for pending email login
	pendingUserID, ok1 := sess.Values["pending_user_id"].(string)
	pendingLoginID, ok2 := sess.Values["pending_login_id"].(string)
	pendingEmail, ok3 := sess.Values["pending_email"].(string)
	if !ok1 || !ok2 || !ok3 || pendingUserID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	returnURL, _ := sess.Values["pending_return_url"].(string)

	templates.Render(w, r, "login_verify_email", emailVerifyFormData{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Verify Email", "/login"),
		LoginID:   pendingLoginID,
		Email:     pendingEmail,
		ReturnURL: returnURL,
	})
}

// handleMagicLink verifies a magic link token and completes login.
func (h *Handler) handleMagicLink(w http.ResponseWriter, r *http.Request, token string) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	v, err := h.EmailVerify.VerifyToken(ctx, token)
	if err != nil {
		h.Log.Warn("invalid magic link token", zap.Error(err))
		// Render the verify form with an error
		templates.Render(w, r, "login_verify_email", emailVerifyFormData{
			BaseVM: viewdata.NewBaseVM(r, h.DB, "Verify Email", "/login"),
			Error:  "This verification link is invalid or has expired. Please request a new one.",
		})
		return
	}

	// Load user
	var u models.User
	err = h.DB.Collection("users").FindOne(ctx, bson.M{"_id": v.UserID}).Decode(&u)
	if err != nil {
		h.Log.Error("failed to load user after magic link verification", zap.Error(err), zap.String("user_id", v.UserID.Hex()))
		templates.Render(w, r, "login_verify_email", emailVerifyFormData{
			BaseVM: viewdata.NewBaseVM(r, h.DB, "Verify Email", "/login"),
			Error:  "Failed to complete login. Please try again.",
		})
		return
	}

	// Get return URL from session if available
	returnURL := ""
	sess, err := h.SessionMgr.GetSession(r)
	if err == nil {
		returnURL, _ = sess.Values["pending_return_url"].(string)
	}

	h.Log.Info("user logged in via magic link", zap.String("user_id", u.ID.Hex()), zap.String("email", v.Email))

	// Complete login
	h.createSessionAndRedirect(w, r, &u, returnURL)
}

/*─────────────────────────────────────────────────────────────────────────────*
| POST /login/verify-email                                                     |
*─────────────────────────────────────────────────────────────────────────────*/

// HandleVerifyEmailSubmit validates the verification code and completes login.
func (h *Handler) HandleVerifyEmailSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/login")
		return
	}

	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Check for pending email login
	pendingUserID, ok1 := sess.Values["pending_user_id"].(string)
	pendingLoginID, ok2 := sess.Values["pending_login_id"].(string)
	pendingEmail, ok3 := sess.Values["pending_email"].(string)
	returnURL, _ := sess.Values["pending_return_url"].(string)
	if !ok1 || !ok2 || !ok3 || pendingUserID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	code := strings.TrimSpace(r.FormValue("code"))
	if code == "" {
		h.renderVerifyEmailFormWithError(w, r, "Please enter the verification code.", pendingLoginID, pendingEmail, returnURL)
		return
	}

	// Verify the code
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	oid, err := parseObjectID(pendingUserID)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	_, err = h.EmailVerify.VerifyCode(ctx, oid, code)
	if err != nil {
		h.Log.Warn("invalid verification code", zap.Error(err), zap.String("user_id", pendingUserID))
		h.renderVerifyEmailFormWithError(w, r, "Invalid or expired verification code. Please try again.", pendingLoginID, pendingEmail, returnURL)
		return
	}

	// Load user
	var u models.User
	err = h.DB.Collection("users").FindOne(ctx, bson.M{"_id": oid}).Decode(&u)
	if err != nil {
		h.Log.Error("failed to load user after code verification", zap.Error(err), zap.String("user_id", pendingUserID))
		h.renderVerifyEmailFormWithError(w, r, "Failed to complete login. Please try again.", pendingLoginID, pendingEmail, returnURL)
		return
	}

	h.Log.Info("user logged in via verification code", zap.String("user_id", u.ID.Hex()), zap.String("email", pendingEmail))

	// Complete login
	h.createSessionAndRedirect(w, r, &u, returnURL)
}

func (h *Handler) renderVerifyEmailFormWithError(w http.ResponseWriter, r *http.Request, msg, loginID, email, returnURL string) {
	templates.Render(w, r, "login_verify_email", emailVerifyFormData{
		BaseVM:    viewdata.NewBaseVM(r, h.DB, "Verify Email", "/login"),
		Error:     msg,
		LoginID:   loginID,
		Email:     email,
		ReturnURL: returnURL,
	})
}

/*─────────────────────────────────────────────────────────────────────────────*
| POST /login/resend-code                                                      |
*─────────────────────────────────────────────────────────────────────────────*/

// HandleResendCode resends the verification email.
func (h *Handler) HandleResendCode(w http.ResponseWriter, r *http.Request) {
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Check for pending email login
	pendingUserID, ok1 := sess.Values["pending_user_id"].(string)
	pendingEmail, ok2 := sess.Values["pending_email"].(string)
	returnURL, _ := sess.Values["pending_return_url"].(string)
	if !ok1 || !ok2 || pendingUserID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	oid, err := parseObjectID(pendingUserID)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Create new verification record (this deletes any existing one)
	result, err := h.EmailVerify.Create(ctx, oid, pendingEmail)
	if err != nil {
		h.Log.Error("failed to create email verification for resend", zap.Error(err), zap.String("email", pendingEmail))
		h.renderVerifyEmailFormWithError(w, r, "Failed to resend verification email. Please try again.", "", pendingEmail, returnURL)
		return
	}

	// Get site name from settings
	siteName := "StrataHub"
	settings, err := settingsstore.New(h.DB).Get(ctx)
	if err == nil && settings.SiteName != "" {
		siteName = settings.SiteName
	}

	// Build magic link
	magicLink := fmt.Sprintf("%s/login/verify-email?token=%s", h.BaseURL, result.Token)

	// Build and send email
	emailData := mailer.VerificationEmailData{
		SiteName:  siteName,
		Code:      result.Code,
		MagicLink: magicLink,
		ExpiresIn: "10 minutes",
	}
	mailMsg := mailer.BuildVerificationEmail(emailData)
	mailMsg.To = pendingEmail

	if err := h.Mailer.Send(mailMsg); err != nil {
		h.Log.Error("failed to resend verification email", zap.Error(err), zap.String("email", pendingEmail))
		h.renderVerifyEmailFormWithError(w, r, "Failed to resend verification email. Please try again.", "", pendingEmail, returnURL)
		return
	}

	h.Log.Info("verification email resent", zap.String("email", pendingEmail), zap.String("user_id", pendingUserID))

	// Redirect back to verify page with a success indication
	// (we can't easily flash messages without additional infrastructure, so just redirect)
	http.Redirect(w, r, "/login/verify-email", http.StatusSeeOther)
}
