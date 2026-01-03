// internal/app/features/uploadcsv/upload.go
package uploadcsv

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/features/uploadcsv/csvutil"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeUploadCSV handles GET /upload_csv.
// Query params:
//   - org: pre-select organization (hex ID)
//   - group: pre-select group (hex ID) - implies group mode with group locked
//   - allowGroup: show group picker even without pre-selected group
//   - return: return URL after completion
func (h *Handler) ServeUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Only admin, coordinator, and leader can upload
	if role != "admin" && role != "coordinator" && role != "leader" {
		uierrors.RenderForbidden(w, r, "You don't have permission to upload members.", "/")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	// Determine mode from query params
	orgHex := query.Get(r, "org")
	groupHex := query.Get(r, "group")
	allowGroup := query.Get(r, "allowGroup") == "true"
	returnURL := query.Get(r, "return")
	if returnURL == "" {
		returnURL = "/members"
	}

	// Group mode if group is specified or allowGroup is true
	groupMode := groupHex != "" || allowGroup

	// Resolve and validate context
	uc, err := h.resolveContext(ctx, r, role, uid, orgHex, groupHex, groupMode)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoOrganization):
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", httpnav.ResolveBackURL(r, returnURL))
		case errors.Is(err, ErrForbiddenOrg):
			uierrors.RenderForbidden(w, r, "You don't have access to this organization.", httpnav.ResolveBackURL(r, returnURL))
		case errors.Is(err, ErrForbiddenGroup):
			uierrors.RenderForbidden(w, r, "You don't have permission to manage this group.", httpnav.ResolveBackURL(r, returnURL))
		case errors.Is(err, ErrOrgNotFound), errors.Is(err, ErrBadOrgID):
			// Org not found - just show page without org selected
			uc = &UploadContext{Role: role, UserID: uid, GroupMode: groupMode}
		case errors.Is(err, ErrGroupNotFound), errors.Is(err, ErrBadGroupID):
			// Group not found - just show page without group selected
			uc = &UploadContext{Role: role, UserID: uid, GroupMode: groupMode, OrgID: uc.OrgID, OrgName: uc.OrgName}
		default:
			h.ErrLog.LogServerError(w, r, "database error resolving context", err, "A database error occurred.", returnURL)
			return
		}
	}

	data := UploadData{
		BaseVM:         viewdata.NewBaseVM(r, h.DB, "Upload CSV", returnURL),
		OrgHex:         uc.OrgID.Hex(),
		OrgName:        uc.OrgName,
		OrgLocked:      uc.OrgLocked,
		GroupMode:      uc.GroupMode,
		GroupID:        uc.GroupID.Hex(),
		GroupName:      uc.GroupName,
		GroupLocked:    uc.GroupLocked,
		ReturnURL:      returnURL,
		CSVAuthMethods: models.GetEnabledAuthMethodsForCSV(),
	}

	// Clear hex if ID is nil
	if uc.OrgID == primitive.NilObjectID {
		data.OrgHex = ""
	}
	if uc.GroupID == primitive.NilObjectID {
		data.GroupID = ""
	}

	templates.Render(w, r, "upload_csv", data)
}

// HandleUploadCSV handles POST /upload_csv.
// This parses the CSV and shows a preview. The user must confirm to actually insert.
func (h *Handler) HandleUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Only admin, coordinator, and leader can upload
	if role != "admin" && role != "coordinator" && role != "leader" {
		uierrors.RenderForbidden(w, r, "You don't have permission to upload members.", "/")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, csvutil.MaxUploadSize)

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Batch(), h.Log, "CSV upload preview")
	defer cancel()

	// Get form values
	orgHex := r.FormValue("orgID")
	groupHex := r.FormValue("groupID")
	groupMode := r.FormValue("groupMode") == "true"
	returnURL := r.FormValue("return")
	if returnURL == "" {
		returnURL = "/members"
	}

	// Build URL for error redirects
	uploadURL := "/upload_csv?return=" + url.QueryEscape(returnURL)
	if orgHex != "" {
		uploadURL += "&org=" + orgHex
	}
	if groupMode {
		uploadURL += "&allowGroup=true"
		if groupHex != "" {
			uploadURL += "&group=" + groupHex
		}
	}

	// Validate org is provided
	if orgHex == "" && role != "leader" {
		uierrors.RenderBadRequest(w, r, "Organization is required.", uploadURL)
		return
	}

	// Resolve and validate context
	uc, err := h.resolveContext(ctx, r, role, uid, orgHex, groupHex, groupMode)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoOrganization):
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", uploadURL)
		case errors.Is(err, ErrForbiddenOrg):
			uierrors.RenderForbidden(w, r, "You don't have access to this organization.", uploadURL)
		case errors.Is(err, ErrForbiddenGroup):
			uierrors.RenderForbidden(w, r, "You don't have permission to manage this group.", uploadURL)
		case errors.Is(err, ErrGroupOrgMismatch):
			uierrors.RenderBadRequest(w, r, "Group does not belong to the selected organization.", uploadURL)
		case errors.Is(err, ErrOrgNotFound), errors.Is(err, ErrBadOrgID):
			uierrors.RenderBadRequest(w, r, "Organization not found.", uploadURL)
		case errors.Is(err, ErrGroupNotFound), errors.Is(err, ErrBadGroupID):
			uierrors.RenderBadRequest(w, r, "Group not found.", uploadURL)
		default:
			h.ErrLog.LogServerError(w, r, "database error resolving context", err, "A database error occurred.", uploadURL)
		}
		return
	}

	// Org is required for upload
	if uc.OrgID == primitive.NilObjectID {
		uierrors.RenderBadRequest(w, r, "Organization is required.", uploadURL)
		return
	}

	// Read CSV file
	file, _, err := r.FormFile("csv")
	if err != nil {
		msg := "CSV file is required."
		if strings.Contains(err.Error(), "request body too large") {
			msg = "CSV file is too large. Maximum size is 5 MB."
		}
		uierrors.RenderBadRequest(w, r, msg, uploadURL)
		return
	}
	defer file.Close()

	// Parse and validate CSV
	parsed, parseErr := csvutil.ParseMembersCSV(file, csvutil.DefaultParseOptions())
	if parseErr != nil {
		uierrors.RenderBadRequest(w, r, "CSV file could not be parsed: "+parseErr.Error(), uploadURL)
		return
	}

	// Helper to render error on the form
	renderFormError := func(errHTML interface{}) {
		data := UploadData{
			BaseVM:         viewdata.NewBaseVM(r, h.DB, "Upload CSV", returnURL),
			OrgHex:         uc.OrgID.Hex(),
			OrgName:        uc.OrgName,
			OrgLocked:      uc.OrgLocked,
			GroupMode:      uc.GroupMode,
			GroupID:        uc.GroupID.Hex(),
			GroupName:      uc.GroupName,
			GroupLocked:    uc.GroupLocked,
			ReturnURL:      returnURL,
			CSVAuthMethods: models.GetEnabledAuthMethodsForCSV(),
		}
		if uc.GroupID == primitive.NilObjectID {
			data.GroupID = ""
		}
		switch v := errHTML.(type) {
		case string:
			data.Error = csvutil.FormatParseErrors([]csvutil.RowError{{Reason: v}}, 1)
		case []csvutil.RowError:
			data.Error = csvutil.FormatParseErrors(v, 5)
		case []csvutil.LoginConflict:
			data.Error = csvutil.FormatConflictErrors(v)
		}
		templates.Render(w, r, "upload_csv", data)
	}

	// Check for parsing errors
	if parsed.HasErrors() {
		renderFormError(parsed.Errors)
		return
	}

	// Check for empty file
	if len(parsed.Members) == 0 {
		renderFormError("CSV file contains no valid members.")
		return
	}

	// Check which login IDs already exist in this org (for preview: new vs update)
	loginIDs := make([]string, len(parsed.Members))
	for i, m := range parsed.Members {
		loginIDs[i] = strings.ToLower(m.LoginID)
	}

	existingInOrg := make(map[string]bool)
	existingInOtherOrg := make(map[string]primitive.ObjectID) // login_id -> org_id

	cur, err := h.DB.Collection("users").Find(ctx, bson.M{"login_id": bson.M{"$in": loginIDs}})
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error checking existing users", err, "A database error occurred.", uploadURL)
		return
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var u struct {
			LoginID string              `bson:"login_id"`
			OrgID   *primitive.ObjectID `bson:"organization_id"`
		}
		if err := cur.Decode(&u); err != nil {
			h.ErrLog.LogServerError(w, r, "database error decoding user", err, "A database error occurred.", uploadURL)
			return
		}
		lid := strings.ToLower(u.LoginID)
		if u.OrgID != nil && *u.OrgID == uc.OrgID {
			existingInOrg[lid] = true
		} else if u.OrgID != nil {
			existingInOtherOrg[lid] = *u.OrgID
		}
	}

	// Check for conflicts (login ID exists in different org)
	var conflicts []csvutil.LoginConflict
	conflictOrgIDs := make(map[primitive.ObjectID]bool)
	for _, m := range parsed.Members {
		lid := strings.ToLower(m.LoginID)
		if otherOrgID, ok := existingInOtherOrg[lid]; ok {
			conflicts = append(conflicts, csvutil.LoginConflict{LoginID: m.LoginID, OrgID: otherOrgID})
			conflictOrgIDs[otherOrgID] = true
		}
	}

	if len(conflicts) > 0 {
		// Look up org names for conflicts
		orgIDList := make([]primitive.ObjectID, 0, len(conflictOrgIDs))
		for oid := range conflictOrgIDs {
			orgIDList = append(orgIDList, oid)
		}
		orgNames := make(map[primitive.ObjectID]string)
		orgCur, err := h.DB.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIDList}})
		if err == nil {
			defer orgCur.Close(ctx)
			for orgCur.Next(ctx) {
				var org struct {
					ID   primitive.ObjectID `bson:"_id"`
					Name string             `bson:"name"`
				}
				if orgCur.Decode(&org) == nil {
					orgNames[org.ID] = org.Name
				}
			}
		}
		// Attach org names to conflicts
		for i := range conflicts {
			if name, ok := orgNames[conflicts[i].OrgID]; ok {
				conflicts[i].OrgName = name
			} else {
				conflicts[i].OrgName = "Unknown"
			}
		}
		renderFormError(conflicts)
		return
	}

	// Build preview data
	previewRows := make([]PreviewRow, len(parsed.Members))
	previewMembers := make([]previewMember, len(parsed.Members))
	toCreate := 0
	toUpdate := 0

	for i, m := range parsed.Members {
		lid := strings.ToLower(m.LoginID)
		isNew := !existingInOrg[lid]
		if isNew {
			toCreate++
		} else {
			toUpdate++
		}

		email := ""
		if m.Email != nil {
			email = *m.Email
		}
		authReturnID := ""
		if m.AuthReturnID != nil {
			authReturnID = *m.AuthReturnID
		}

		previewRows[i] = PreviewRow{
			FullName:     m.FullName,
			LoginID:      m.LoginID,
			AuthMethod:   m.AuthMethod,
			Email:        email,
			AuthReturnID: authReturnID,
			IsNew:        isNew,
		}

		previewMembers[i] = previewMember{
			FullName:     m.FullName,
			LoginID:      m.LoginID,
			AuthMethod:   m.AuthMethod,
			Email:        m.Email,
			AuthReturnID: m.AuthReturnID,
			TempPassword: m.TempPassword,
		}
	}

	// Encode preview data as JSON for the confirmation form
	previewJSON, err := json.Marshal(previewMembers)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to encode preview data", err, "A server error occurred.", uploadURL)
		return
	}

	// Build preview page title
	title := "Upload CSV - Preview"

	data := UploadData{
		BaseVM:         viewdata.NewBaseVM(r, h.DB, title, returnURL),
		OrgHex:         uc.OrgID.Hex(),
		OrgName:        uc.OrgName,
		OrgLocked:      true, // Always locked in preview
		GroupMode:      uc.GroupMode,
		GroupID:        uc.GroupID.Hex(),
		GroupName:      uc.GroupName,
		GroupLocked:    true, // Always locked in preview
		ReturnURL:      returnURL,
		CSVAuthMethods: models.GetEnabledAuthMethodsForCSV(),
		ShowPreview:    true,
		PreviewRows:    previewRows,
		PreviewJSON:    string(previewJSON),
		TotalToCreate:  toCreate,
		TotalToUpdate:  toUpdate,
	}

	if uc.GroupID == primitive.NilObjectID {
		data.GroupID = ""
	}

	// Set BackURL to return to upload form
	data.BackURL = uploadURL

	templates.Render(w, r, "upload_csv", data)
}

// RedirectConfirm handles GET /upload_csv/confirm.
// This redirects to the upload form (in case user navigates here via back button).
func (h *Handler) RedirectConfirm(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/upload_csv", http.StatusSeeOther)
}
