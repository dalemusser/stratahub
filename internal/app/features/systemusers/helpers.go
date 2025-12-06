// internal/app/features/systemusers/helpers.go
package systemusers

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/templates"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

/*
backToSystemUsersURL computes a “safe” return URL for System Users pages.

It honors ?return=... / form return only if it points to /system-users and
is not one of the sub-actions (new, edit, delete). Otherwise it falls back
to the canonical list page (/system-users).
*/
func backToSystemUsersURL(r *http.Request) string {
	ret := strings.TrimSpace(r.URL.Query().Get("return"))
	if ret == "" {
		ret = strings.TrimSpace(r.FormValue("return"))
	}
	if strings.HasPrefix(ret, "/system-users") &&
		!strings.Contains(ret, "/edit") &&
		!strings.Contains(ret, "/delete") &&
		!strings.Contains(ret, "/new") {
		return ret
	}
	return "/system-users"
}

/*
requireAdmin gates an endpoint to admins only.

It returns (role, userName, userID, ok). If ok is false, it has already
written an HTTP error (401/403) to the response.
*/
func requireAdmin(w http.ResponseWriter, r *http.Request) (string, string, primitive.ObjectID, bool) {
	role, uname, uid, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", "", primitive.NilObjectID, false
	}
	if !authz.IsAdmin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return "", "", primitive.NilObjectID, false
	}
	return role, uname, uid, true
}

/*
countActiveAdmins returns the number of users with role=admin and status=active.

Callers pass in the DB and a context with an appropriate timeout.
*/
func countActiveAdmins(ctx context.Context, db *mongo.Database) (int64, error) {
	return db.Collection("users").CountDocuments(ctx, bson.M{
		"role":   "admin",
		"status": "active",
	})
}

/*
renderEditForm centralizes rendering of the Edit System User form
with the posted values and an optional error message.

This keeps HandleEdit and Delete-guard paths from duplicating
the same form wiring.
*/
func renderEditForm(
	w http.ResponseWriter,
	r *http.Request,
	role, uname, idHex, full, email, uRole, authm, status string,
	errMsg template.HTML,
) {
	templates.Render(w, r, "system_user_edit", formData{
		Title:       "Edit System User",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		ID:          idHex,
		FullName:    full,
		Email:       email,
		URole:       uRole,
		UserRole:    uRole,
		Auth:        authm,
		Status:      status,
		Error:       errMsg,
		BackURL:     backToSystemUsersURL(r),
		CurrentPath: nav.CurrentPath(r),
	})
}
