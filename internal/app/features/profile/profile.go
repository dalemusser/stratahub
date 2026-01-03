// internal/app/features/profile/profile.go
package profile

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/mongo"
)

// profileData is the view model for the profile page.
type profileData struct {
	viewdata.BaseVM

	// User info (read-only display)
	FullName   string
	Email      string
	AuthMethod string

	// Password section (only shown for password auth)
	ShowPasswordSection bool
	PasswordRules       string

	// Preferences section
	ThemePreference string // "light", "dark", "system"

	// Form state
	Error   template.HTML
	Success template.HTML
}

// ServeProfile renders the user's profile page.
func (h *Handler) ServeProfile(w http.ResponseWriter, r *http.Request) {
	_, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	usrStore := userstore.New(h.DB)
	user, err := usrStore.GetByID(ctx, uid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "User not found.", "/")
		return
	}

	email := ""
	if user.Email != nil {
		email = *user.Email
	}

	data := profileData{
		BaseVM:              viewdata.NewBaseVM(r, h.DB, "Profile", "/"),
		FullName:            user.FullName,
		Email:               email,
		AuthMethod:          formatAuthMethod(user.AuthMethod),
		ShowPasswordSection: user.AuthMethod == "password",
		PasswordRules:       authutil.PasswordRules(),
		ThemePreference:     user.ThemePreference,
	}

	// Default to "system" if empty
	if data.ThemePreference == "" {
		data.ThemePreference = "system"
	}

	// Check for success message in query params
	if r.URL.Query().Get("success") == "password" {
		data.Success = "Password changed successfully."
	} else if r.URL.Query().Get("success") == "preferences" {
		data.Success = "Preferences saved."
	}

	templates.Render(w, r, "profile", data)
}

// HandleChangePassword processes the password change form.
func (h *Handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	_, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/profile")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	usrStore := userstore.New(h.DB)
	user, err := usrStore.GetByID(ctx, uid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "User not found.", "/")
		return
	}

	// Only allow password change for password auth users
	if user.AuthMethod != "password" {
		renderProfileWithError(w, r, h.DB, user, "Password change is only available for password authentication.")
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	// Verify current password
	if user.PasswordHash == nil || !authutil.CheckPassword(currentPassword, *user.PasswordHash) {
		renderProfileWithError(w, r, h.DB, user, "Current password is incorrect.")
		return
	}

	// Validate new password
	if err := authutil.ValidatePassword(newPassword); err != nil {
		renderProfileWithError(w, r, h.DB, user, err.Error())
		return
	}

	// Check passwords match
	if newPassword != confirmPassword {
		renderProfileWithError(w, r, h.DB, user, "New passwords do not match.")
		return
	}

	// Don't allow reusing the current password
	if authutil.CheckPassword(newPassword, *user.PasswordHash) {
		renderProfileWithError(w, r, h.DB, user, "New password cannot be the same as your current password.")
		return
	}

	// Hash and save the new password
	hash, err := authutil.HashPassword(newPassword)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "hash password failed", err, "Failed to update password.", "/profile")
		return
	}

	if err := usrStore.UpdatePassword(ctx, uid, hash); err != nil {
		h.ErrLog.LogServerError(w, r, "update password failed", err, "Failed to update password.", "/profile")
		return
	}

	http.Redirect(w, r, "/profile?success=password", http.StatusSeeOther)
}

// HandleUpdatePreferences processes the preferences form.
func (h *Handler) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	_, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/profile")
		return
	}

	theme := strings.TrimSpace(r.FormValue("theme_preference"))

	// Validate theme value
	switch theme {
	case "light", "dark", "system":
		// valid
	default:
		theme = "system"
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	usrStore := userstore.New(h.DB)
	if err := usrStore.UpdateThemePreference(ctx, uid, theme); err != nil {
		h.ErrLog.LogServerError(w, r, "update theme preference failed", err, "Failed to save preferences.", "/profile")
		return
	}

	// Set theme preference cookie so the new theme applies immediately on redirect
	http.SetCookie(w, &http.Cookie{
		Name:     "theme_pref",
		Value:    theme,
		Path:     "/",
		MaxAge:   60,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/profile?success=preferences", http.StatusSeeOther)
}

// renderProfileWithError re-renders the profile page with an error message.
func renderProfileWithError(w http.ResponseWriter, r *http.Request, db *mongo.Database, user *models.User, errMsg string) {
	email := ""
	if user.Email != nil {
		email = *user.Email
	}

	data := profileData{
		BaseVM:              viewdata.NewBaseVM(r, db, "Profile", "/"),
		FullName:            user.FullName,
		Email:               email,
		AuthMethod:          formatAuthMethod(user.AuthMethod),
		ShowPasswordSection: user.AuthMethod == "password",
		PasswordRules:       authutil.PasswordRules(),
		ThemePreference:     user.ThemePreference,
		Error:               template.HTML(errMsg),
	}

	if data.ThemePreference == "" {
		data.ThemePreference = "system"
	}

	templates.Render(w, r, "profile", data)
}

// formatAuthMethod returns a human-readable label for the auth method.
func formatAuthMethod(method string) string {
	switch method {
	case "password":
		return "Password"
	case "email":
		return "Email"
	case "google":
		return "Google"
	case "microsoft":
		return "Microsoft"
	case "clever":
		return "Clever"
	case "classlink":
		return "ClassLink"
	case "schoology":
		return "Schoology"
	case "trust":
		return "Trusted"
	default:
		return method
	}
}
