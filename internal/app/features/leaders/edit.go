package leaders

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/db/mongodb"
	"github.com/dalemusser/waffle/toolkit/text/textfold"
	"github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/dalemusser/waffle/toolkit/validate"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	editTimeoutShort = 5 * time.Second
	editTimeoutMed   = 10 * time.Second
)

// editData is the view model for the edit-leader page.
type editData struct {
	Title, Role, UserName string
	IsLoggedIn            bool
	ID, FullName, Email   string
	OrgID, OrgName        string // Org is read-only display + hidden orgID
	Status, Auth          string
	BackURL, CurrentPath  string
	Error                 template.HTML
}

// ServeEdit renders the Edit Leader page.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.CurrentUser(r)

	ctx, cancel := context.WithTimeout(r.Context(), editTimeoutShort)
	defer cancel()

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	var usr models.User
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "leader"}).Decode(&usr); err != nil {
		http.NotFound(w, r)
		return
	}

	orgHex := ""
	orgName := ""
	if usr.OrganizationID != nil {
		orgHex = usr.OrganizationID.Hex()
		var o models.Organization
		_ = h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": *usr.OrganizationID}).Decode(&o)
		orgName = o.Name
	}

	data := editData{
		Title:       "Edit Leader",
		IsLoggedIn:  true,
		Role:        "admin",
		UserName:    u.Name,
		ID:          usr.ID.Hex(),
		FullName:    usr.FullName,
		Email:       strings.ToLower(usr.Email),
		OrgID:       orgHex,  // hidden field
		OrgName:     orgName, // read-only display
		Status:      usr.Status,
		Auth:        strings.ToLower(usr.AuthMethod),
		BackURL:     nav.ResolveBackURL(r, "/leaders"),
		CurrentPath: nav.CurrentPath(r),
	}

	templates.Render(w, r, "admin_leader_edit", data)
}

// HandleEdit processes the Edit Leader form submission.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	full := strings.TrimSpace(r.FormValue("full_name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	authm := strings.ToLower(strings.TrimSpace(r.FormValue("auth_method")))
	status := strings.ToLower(strings.TrimSpace(r.FormValue("status")))
	orgHex := strings.TrimSpace(r.FormValue("orgID")) // carried as hidden; not changeable

	ctx, cancel := context.WithTimeout(r.Context(), editTimeoutMed)
	defer cancel()

	// load org name for re-render convenience
	orgName := ""
	if oid, e := primitive.ObjectIDFromHex(orgHex); e == nil {
		var o models.Organization
		_ = h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": oid}).Decode(&o)
		orgName = o.Name
	}

	// Normalize/validate status: active / disabled only
	switch status {
	case "active", "disabled":
		// ok
	case "":
		status = "active"
	default:
		templates.Render(w, r, "admin_leader_edit", editData{
			Title: "Edit Leader", IsLoggedIn: true, Role: "admin", UserName: currentUserName(r),
			ID: uid.Hex(), FullName: full, Email: email, OrgID: orgHex, OrgName: orgName,
			Status: status, Auth: authm, BackURL: nav.ResolveBackURL(r, "/leaders"),
			CurrentPath: nav.CurrentPath(r), Error: template.HTML("Status must be active or disabled."),
		})
		return
	}

	// Inline validation with specific messages (like Members)
	if full == "" {
		templates.Render(w, r, "admin_leader_edit", editData{
			Title: "Edit Leader", IsLoggedIn: true, Role: "admin", UserName: currentUserName(r),
			ID: uid.Hex(), FullName: full, Email: email, OrgID: orgHex, OrgName: orgName,
			Status: status, Auth: authm, BackURL: nav.ResolveBackURL(r, "/leaders"),
			CurrentPath: nav.CurrentPath(r), Error: template.HTML("Full name is required."),
		})
		return
	}
	if email == "" || !validate.SimpleEmailValid(email) {
		templates.Render(w, r, "admin_leader_edit", editData{
			Title: "Edit Leader", IsLoggedIn: true, Role: "admin", UserName: currentUserName(r),
			ID: uid.Hex(), FullName: full, Email: email, OrgID: orgHex, OrgName: orgName,
			Status: status, Auth: authm, BackURL: nav.ResolveBackURL(r, "/leaders"),
			CurrentPath: nav.CurrentPath(r), Error: template.HTML("A valid email address is required."),
		})
		return
	}

	// Early uniqueness check: same email used by a different user?
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{
		"email": email,
		"_id":   bson.M{"$ne": uid},
	}).Err(); err == nil {
		templates.Render(w, r, "admin_leader_edit", editData{
			Title: "Edit Leader", IsLoggedIn: true, Role: "admin", UserName: currentUserName(r),
			ID: uid.Hex(), FullName: full, Email: email, OrgID: orgHex, OrgName: orgName,
			Status: status, Auth: authm, BackURL: nav.ResolveBackURL(r, "/leaders"),
			CurrentPath: nav.CurrentPath(r), Error: template.HTML("A user with that email already exists."),
		})
		return
	}

	// Build update doc WITHOUT changing organization_id
	up := bson.M{
		"full_name":    full,
		"full_name_ci": textfold.Fold(full),
		"email":        email,
		"auth_method":  authm,
		"status":       status,
		"updated_at":   time.Now(),
	}
	if _, err := h.DB.Collection("users").UpdateOne(ctx, bson.M{"_id": uid, "role": "leader"}, bson.M{"$set": up}); err != nil {
		msg := template.HTML("Database error while updating leader.")
		if mongodb.IsDup(err) {
			msg = template.HTML("A user with that email already exists.")
		}
		templates.Render(w, r, "admin_leader_edit", editData{
			Title: "Edit Leader", IsLoggedIn: true, Role: "admin", UserName: currentUserName(r),
			ID: uid.Hex(), FullName: full, Email: email, OrgID: orgHex, OrgName: orgName,
			Status: status, Auth: authm, BackURL: nav.ResolveBackURL(r, "/leaders"),
			CurrentPath: nav.CurrentPath(r), Error: msg,
		})
		return
	}

	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/leaders", http.StatusSeeOther)
}

// currentUserName returns the current user's name for reuse in error views.
func currentUserName(r *http.Request) string {
	if u, ok := auth.CurrentUser(r); ok && u != nil {
		return u.Name
	}
	return ""
}
