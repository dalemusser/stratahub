// internal/app/features/members/upload.go
package members

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/csvutil"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

const (
	membersUploadShortTimeout = 5 * time.Second
	membersUploadLongTimeout  = 60 * time.Second
)

// ServeUploadCSV handles GET /members/upload_csv.
func (h *Handler) ServeUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := userCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersUploadShortTimeout)
	defer cancel()
	db := h.DB

	data := uploadData{
		Title:       "Upload CSV",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		BackURL:     backToMembersURL(r),
		CurrentPath: nav.CurrentPath(r),
	}

	if role == "leader" {
		// Leader must be associated with an organization
		var u models.User
		if err := db.Collection("users").
			FindOne(ctx, bson.M{"_id": uid}).
			Decode(&u); err != nil || u.OrganizationID == nil {

			h.Log.Warn("leader org resolve failed", zap.Error(err))
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", nav.ResolveBackURL(r, "/members"))
			return
		}
		data.OrgLocked = true
		data.OrgHex = u.OrganizationID.Hex()

		var org models.Organization
		_ = db.Collection("organizations").
			FindOne(ctx, bson.M{"_id": *u.OrganizationID}).
			Decode(&org)
		data.OrgName = org.Name
	} else {
		// Admin must select an org before uploading
		orgHex := strings.TrimSpace(r.URL.Query().Get("org"))
		if orgHex == "" || orgHex == "all" {
			uierrors.RenderForbidden(w, r, "Select a specific organization first.", nav.ResolveBackURL(r, "/members"))
			return
		}
		oid, err := primitive.ObjectIDFromHex(orgHex)
		if err != nil {
			uierrors.RenderForbidden(w, r, "Bad organization id.", nav.ResolveBackURL(r, "/members"))
			return
		}
		data.OrgLocked = true
		data.OrgHex = orgHex

		var org models.Organization
		_ = db.Collection("organizations").
			FindOne(ctx, bson.M{"_id": oid}).
			Decode(&org)
		if org.Name == "" {
			uierrors.RenderForbidden(w, r, "Organization not found.", nav.ResolveBackURL(r, "/members"))
			return
		}
		data.OrgName = org.Name
	}

	templates.Render(w, r, "member_upload_csv", data)
}

// HandleUploadCSV handles POST /members/upload_csv.
func (h *Handler) HandleUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := userCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersUploadLongTimeout)
	defer cancel()
	db := h.DB

	// Resolve organization (same as ServeUploadCSV)
	var orgID primitive.ObjectID
	if role == "leader" {
		var me models.User
		if err := db.Collection("users").
			FindOne(ctx, bson.M{"_id": uid}).
			Decode(&me); err != nil || me.OrganizationID == nil {

			h.Log.Warn("leader org resolve failed", zap.Error(err))
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", nav.ResolveBackURL(r, "/members"))
			return
		}
		orgID = *me.OrganizationID
	} else {
		orgHex := strings.TrimSpace(r.FormValue("orgID"))
		if orgHex == "" || orgHex == "all" {
			uierrors.RenderForbidden(w, r, "Organization is required.", nav.ResolveBackURL(r, "/members"))
			return
		}
		oid, err := primitive.ObjectIDFromHex(orgHex)
		if err != nil {
			uierrors.RenderForbidden(w, r, "Bad organization id.", nav.ResolveBackURL(r, "/members"))
			return
		}
		orgID = oid
	}

	// Read CSV file
	file, _, err := r.FormFile("csv")
	if err != nil {
		uierrors.RenderForbidden(w, r, "CSV file is required.", nav.ResolveBackURL(r, "/members"))
		return
	}
	defer file.Close()

	// Shared CSV pre-scan/validation
	rows, htmlErr, _ := csvutil.PreScanMembersCSV(file)
	if htmlErr != "" {
		var org models.Organization
		_ = db.Collection("organizations").
			FindOne(ctx, bson.M{"_id": orgID}).
			Decode(&org)

		templates.Render(w, r, "member_upload_csv", uploadData{
			Title:       "Upload CSV",
			IsLoggedIn:  true,
			Role:        role,
			UserName:    uname,
			BackURL:     backToMembersURL(r),
			CurrentPath: nav.CurrentPath(r),
			OrgLocked:   true,
			OrgHex:      orgID.Hex(),
			OrgName:     org.Name,
			Error:       htmlErr,
		})
		return
	}

	// Upsert using the store helper (do not move users across orgs)
	us := userstore.New(db)
	var created, previously, skipped int
	var skippedEmails []string

	for _, row := range rows {
		updated, conflict, e := us.UpsertMembersInOrg(ctx, orgID, row.FullName, row.Email, row.Auth)
		if e != nil {
			h.Log.Warn("upsert member row failed", zap.String("email", row.Email), zap.Error(e))
			continue
		}
		if conflict != nil {
			skipped++
			skippedEmails = append(skippedEmails, strings.ToLower(strings.TrimSpace(row.Email)))
			continue
		}
		if updated {
			previously++
		} else {
			created++
		}
	}

	// Summary screen
	var org models.Organization
	_ = db.Collection("organizations").
		FindOne(ctx, bson.M{"_id": orgID}).
		Decode(&org)

	templates.Render(w, r, "member_upload_csv", uploadData{
		Title:         "Upload CSV",
		IsLoggedIn:    true,
		Role:          role,
		UserName:      uname,
		BackURL:       backToMembersURL(r),
		CurrentPath:   nav.CurrentPath(r),
		OrgLocked:     true,
		OrgHex:        orgID.Hex(),
		OrgName:       org.Name,
		Created:       created,
		Previously:    previously,
		SkippedCount:  skipped,
		SkippedEmails: skippedEmails,
	})
}
