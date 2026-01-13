// internal/app/features/systemusers/helpers.go
package systemusers

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"html/template"
	"net/http"

	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
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

// editFormParams holds parameters for renderEditForm.
type editFormParams struct {
	ID           string
	FullName     string
	LoginID      string
	Email        string
	AuthReturnID string
	TempPassword string
	Role         string
	Auth         string
	Status       string
	IsSelf       bool
	ErrMsg       template.HTML
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
	p editFormParams,
) {
	templates.Render(w, r, "system_user_edit", formData{
		BaseVM:       viewdata.NewBaseVM(r, db, "Edit System User", backToSystemUsersURL(r)),
		AuthMethods:  models.AllAuthMethods,
		ID:           p.ID,
		FullName:     p.FullName,
		LoginID:      p.LoginID,
		Email:        p.Email,
		AuthReturnID: p.AuthReturnID,
		TempPassword: p.TempPassword,
		URole:        p.Role,
		UserRole:     p.Role,
		Auth:         p.Auth,
		Status:       p.Status,
		IsEdit:       true,
		IsSelf:       p.IsSelf,
		Error:        p.ErrMsg,
	})
}
