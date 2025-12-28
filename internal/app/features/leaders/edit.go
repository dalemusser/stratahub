package leaders

import (
	"context"
	"net/http"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// editLeaderInput defines validation rules for editing a leader.
type editLeaderInput struct {
	FullName string `validate:"required,max=200" label:"Full name"`
	Email    string `validate:"required,email,max=254" label:"Email"`
	Status   string `validate:"required,oneof=active disabled" label:"Status"`
}

// editData is the view model for the edit-leader page.
type editData struct {
	formutil.Base

	ID, FullName, Email string
	OrgID, OrgName      string // Org is read-only display + hidden orgID
	Status, Auth        string
}

// ServeEdit renders the Edit Leader page.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid leader ID.", "/leaders")
		return
	}

	var usr models.User
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "leader"}).Decode(&usr); err != nil {
		uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		return
	}

	orgHex := ""
	orgName := ""
	if usr.OrganizationID != nil {
		orgHex = usr.OrganizationID.Hex()
		var o models.Organization
		if err := h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": *usr.OrganizationID}).Decode(&o); err != nil {
			if err == mongo.ErrNoDocuments {
				orgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization for leader", err, "A database error occurred.", "/leaders")
				return
			}
		} else {
			orgName = o.Name
		}
	}

	data := editData{
		ID:       usr.ID.Hex(),
		FullName: usr.FullName,
		Email:    normalize.Email(usr.Email),
		OrgID:    orgHex,  // hidden field
		OrgName:  orgName, // read-only display
		Status:   usr.Status,
		Auth:     normalize.AuthMethod(usr.AuthMethod),
	}
	formutil.SetBase(&data.Base, r, h.DB, "Edit Leader", "/leaders")

	templates.Render(w, r, "admin_leader_edit", data)
}

// HandleEdit processes the Edit Leader form submission.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form submission.", "/leaders")
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid leader ID.", "/leaders")
		return
	}

	full := normalize.Name(r.FormValue("full_name"))
	email := normalize.Email(r.FormValue("email"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))
	status := normalize.Status(r.FormValue("status"))
	orgHex := normalize.QueryParam(r.FormValue("orgID")) // carried as hidden; not changeable

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// load org name for re-render convenience
	orgName := ""
	if oid, e := primitive.ObjectIDFromHex(orgHex); e == nil {
		var o models.Organization
		if err := h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": oid}).Decode(&o); err != nil {
			if err == mongo.ErrNoDocuments {
				orgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization", err, "A database error occurred.", "/leaders")
				return
			}
		} else {
			orgName = o.Name
		}
	}

	// Normalize empty status to active
	if status == "" {
		status = "active"
	}

	reRender := func(msg string) {
		data := editData{
			ID: uid.Hex(), FullName: full, Email: email, OrgID: orgHex, OrgName: orgName,
			Status: status, Auth: authm,
		}
		formutil.SetBase(&data.Base, r, h.DB, "Edit Leader", "/leaders")
		data.SetError(msg)
		templates.Render(w, r, "admin_leader_edit", data)
	}

	// Validate required fields using struct tags
	input := editLeaderInput{FullName: full, Email: email, Status: status}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	// Early uniqueness check: same email used by a different user?
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{
		"email": email,
		"_id":   bson.M{"$ne": uid},
	}).Err(); err == nil {
		reRender("A user with that email already exists.")
		return
	}

	// Build update doc WITHOUT changing organization_id
	up := bson.M{
		"full_name":    full,
		"full_name_ci": text.Fold(full),
		"email":        email,
		"auth_method":  authm,
		"status":       status,
		"updated_at":   time.Now(),
	}
	if _, err := h.DB.Collection("users").UpdateOne(ctx, bson.M{"_id": uid, "role": "leader"}, bson.M{"$set": up}); err != nil {
		msg := "Database error while updating leader."
		if wafflemongo.IsDup(err) {
			msg = "A user with that email already exists."
		}
		reRender(msg)
		return
	}

	ret := navigation.SafeBackURL(r, navigation.LeadersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
