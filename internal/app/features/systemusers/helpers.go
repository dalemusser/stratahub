// internal/app/features/systemusers/helpers.go
package systemusers

import (
	"context"
	"html/template"
	"net/http"

	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// backToSystemUsersURL returns a safe return URL for System Users pages.
func backToSystemUsersURL(r *http.Request) string {
	return navigation.SafeBackURL(r, navigation.SystemUsersBackURL)
}

// userContext returns the current user's context (role, name, userID, ok).
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach handlers calling this.
// This function only retrieves context; it does not perform authorization checks.
func userContext(r *http.Request) (string, string, primitive.ObjectID, bool) {
	role, name, userID, ok := authz.UserCtx(r)
	return role, name, userID, ok
}

/*
countActiveAdmins returns the number of users with role=admin and status=active.

Callers pass in the DB and a context with an appropriate timeout.
*/
func countActiveAdmins(ctx context.Context, db *mongo.Database) (int64, error) {
	return userstore.New(db).CountActiveAdmins(ctx)
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
	db *mongo.Database,
	idHex, full, email, uRole, authm, status string,
	isSelf bool,
	errMsg template.HTML,
) {
	templates.Render(w, r, "system_user_edit", formData{
		BaseVM:   viewdata.NewBaseVM(r, db, "Edit System User", backToSystemUsersURL(r)),
		ID:       idHex,
		FullName: full,
		Email:    email,
		URole:    uRole,
		UserRole: uRole,
		Auth:     authm,
		Status:   status,
		IsSelf:   isSelf,
		Error:    errMsg,
	})
}
