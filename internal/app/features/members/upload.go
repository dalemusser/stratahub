// internal/app/features/members/upload.go
package members

import (
	"context"
	"errors"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/memberpolicy"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/csvutil"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeUploadCSV handles GET /members/upload_csv.
func (h *Handler) ServeUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Check authorization using policy layer
	listScope := memberpolicy.CanListMembers(r)
	if !listScope.CanList {
		uierrors.RenderForbidden(w, r, "You don't have permission to upload members.", httpnav.ResolveBackURL(r, "/"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	var data uploadData
	formutil.SetBase(&data.Base, r, "Upload CSV", "/members")

	var orgID primitive.ObjectID
	var orgName string
	var err error

	if role == "leader" {
		orgID, orgName, err = orgutil.ResolveLeaderOrg(ctx, db, uid)
		if errors.Is(err, orgutil.ErrUserNotFound) || errors.Is(err, orgutil.ErrNoOrganization) {
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error resolving leader org", err, "A database error occurred.", "/members")
			return
		}
	} else {
		orgHex := normalize.OrgID(query.Get(r, "org"))
		if orgHex == "" {
			uierrors.RenderForbidden(w, r, "Select a specific organization first.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		orgID, orgName, err = orgutil.ResolveOrgFromHex(ctx, db, orgHex)
		if errors.Is(err, orgutil.ErrBadOrgID) {
			uierrors.RenderForbidden(w, r, "Bad organization id.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if errors.Is(err, orgutil.ErrOrgNotFound) {
			uierrors.RenderForbidden(w, r, "Organization not found.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error loading organization", err, "A database error occurred.", "/members")
			return
		}
	}
	data.OrgLocked = true
	data.OrgHex = orgID.Hex()
	data.OrgName = orgName

	templates.Render(w, r, "member_upload_csv", data)
}

// HandleUploadCSV handles POST /members/upload_csv.
func (h *Handler) HandleUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Check authorization using policy layer
	listScope := memberpolicy.CanListMembers(r)
	if !listScope.CanList {
		uierrors.RenderForbidden(w, r, "You don't have permission to upload members.", httpnav.ResolveBackURL(r, "/"))
		return
	}

	// Limit request body size to prevent memory exhaustion from large uploads
	r.Body = http.MaxBytesReader(w, r.Body, csvutil.MaxUploadSize)

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Batch(), h.Log, "members CSV upload")
	defer cancel()
	db := h.DB

	// Resolve organization
	var orgID primitive.ObjectID
	var orgName string
	if role == "leader" {
		var err error
		orgID, orgName, err = orgutil.ResolveLeaderOrg(ctx, db, uid)
		if errors.Is(err, orgutil.ErrUserNotFound) || errors.Is(err, orgutil.ErrNoOrganization) {
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error resolving leader org", err, "A database error occurred.", "/members")
			return
		}
	} else {
		orgHex := normalize.OrgID(r.FormValue("orgID"))
		if orgHex == "" {
			uierrors.RenderForbidden(w, r, "Organization is required.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		var err error
		orgID, orgName, err = orgutil.ResolveOrgFromHex(ctx, db, orgHex)
		if errors.Is(err, orgutil.ErrBadOrgID) {
			uierrors.RenderForbidden(w, r, "Bad organization id.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if errors.Is(err, orgutil.ErrOrgNotFound) {
			uierrors.RenderForbidden(w, r, "Organization not found.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error loading organization", err, "A database error occurred.", "/members")
			return
		}
	}

	// Read CSV file
	file, _, err := r.FormFile("csv")
	if err != nil {
		msg := "CSV file is required."
		if strings.Contains(err.Error(), "request body too large") {
			msg = "CSV file is too large. Maximum size is 5 MB."
		}
		uierrors.RenderForbidden(w, r, msg, httpnav.ResolveBackURL(r, "/members"))
		return
	}
	defer file.Close()

	// Parse and validate CSV
	parsed, parseErr := csvutil.ParseMemberCSV(file, csvutil.DefaultParseOptions())
	if parseErr != nil {
		uierrors.RenderForbidden(w, r, "CSV file could not be parsed: "+parseErr.Error(), httpnav.ResolveBackURL(r, "/members"))
		return
	}
	if parsed.HasErrors() {
		data := uploadData{
			OrgLocked: true,
			OrgHex:    orgID.Hex(),
			OrgName:   orgName,
		}
		formutil.SetBase(&data.Base, r, "Upload CSV", "/members")
		data.Error = parsed.FormatErrorsHTML(5)
		templates.Render(w, r, "member_upload_csv", data)
		return
	}

	// Convert parsed rows to member entries
	entries := make([]userstore.MemberEntry, len(parsed.Rows))
	for i, row := range parsed.Rows {
		entries[i] = userstore.MemberEntry{
			FullName:   row.FullName,
			Email:      row.Email,
			AuthMethod: row.Auth,
		}
	}

	// Batch upsert (do not move users across orgs)
	us := userstore.New(db)
	result, err := us.UpsertMembersInOrgBatch(ctx, orgID, entries)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error batch upserting members", err, "A database error occurred.", "/members")
		return
	}

	// Summary screen
	summaryData := uploadData{
		OrgLocked:     true,
		OrgHex:        orgID.Hex(),
		OrgName:       orgName,
		Created:       result.Created,
		Previously:    result.Updated,
		SkippedCount:  result.Skipped,
		SkippedEmails: result.SkippedEmails,
	}
	formutil.SetBase(&summaryData.Base, r, "Upload CSV", "/members")
	templates.Render(w, r, "member_upload_csv", summaryData)
}
