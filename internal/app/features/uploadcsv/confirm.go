// internal/app/features/uploadcsv/confirm.go
package uploadcsv

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/app/system/wsauth"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HandleConfirm handles POST /upload_csv/confirm.
// This actually inserts/updates the members after preview confirmation.
func (h *Handler) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Only superadmin, admin, coordinator, and leader can upload
	if role != "superadmin" && role != "admin" && role != "coordinator" && role != "leader" {
		uierrors.RenderForbidden(w, r, "You don't have permission to upload members.", "/")
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Batch(), h.Log, "CSV upload confirm")
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

	// Resolve and validate context (re-validate on confirm!)
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

	// Org is required
	if uc.OrgID == primitive.NilObjectID {
		uierrors.RenderBadRequest(w, r, "Organization is required.", uploadURL)
		return
	}

	// Decode preview data from form
	previewJSON := r.FormValue("preview_data")
	if previewJSON == "" {
		uierrors.RenderBadRequest(w, r, "Preview data is missing. Please start the upload again.", uploadURL)
		return
	}

	var previewMembers []previewMember
	if err := json.Unmarshal([]byte(previewJSON), &previewMembers); err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid preview data. Please start the upload again.", uploadURL)
		return
	}

	if len(previewMembers) == 0 {
		uierrors.RenderBadRequest(w, r, "No members to import.", uploadURL)
		return
	}

	// Convert preview members to MemberEntry for batch upsert
	entries := make([]userstore.MemberEntry, len(previewMembers))
	for i, pm := range previewMembers {
		entries[i] = userstore.MemberEntry{
			FullName:     pm.FullName,
			LoginID:      pm.LoginID,
			AuthMethod:   pm.AuthMethod,
			Email:        pm.Email,
			AuthReturnID: pm.AuthReturnID,
			TempPassword: pm.TempPassword,
		}
	}

	// Batch upsert members in organization
	us := userstore.New(h.DB)
	result, err := us.UpsertMembersInOrgBatch(ctx, uc.OrgID, entries)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error batch upserting members", err, "A database error occurred.", uploadURL)
		return
	}

	// Track group membership stats
	var addedToGroup, alreadyInGroup int

	// If group mode, add members to the group
	if groupMode && uc.GroupID != primitive.NilObjectID {
		// Collect login IDs that were successfully processed (not skipped)
		skippedSet := make(map[string]struct{}, len(result.SkippedMembers))
		for _, sm := range result.SkippedMembers {
			skippedSet[strings.ToLower(sm.LoginID)] = struct{}{}
		}

		// Deduplicate and collect processed login IDs
		seen := make(map[string]struct{})
		var processedLoginIDs []string
		for _, pm := range previewMembers {
			loginIDLower := strings.ToLower(pm.LoginID)
			if _, skipped := skippedSet[loginIDLower]; skipped {
				continue
			}
			if _, dup := seen[loginIDLower]; dup {
				continue
			}
			seen[loginIDLower] = struct{}{}
			processedLoginIDs = append(processedLoginIDs, loginIDLower)
		}

		// Batch fetch users to get their IDs for membership creation
		users, err := h.batchFetchUsersByLoginID(ctx, processedLoginIDs)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error batch loading users", err, "A database error occurred.", uploadURL)
			return
		}

		// Build membership entries from fetched users
		memberships := make([]membershipstore.MembershipEntry, 0, len(users))
		for _, u := range users {
			memberships = append(memberships, membershipstore.MembershipEntry{
				UserID: u.ID,
				Role:   "member",
			})
		}

		// Batch add memberships
		if len(memberships) > 0 {
			msResult, err := membershipstore.New(h.DB).AddBatch(ctx, uc.GroupID, uc.OrgID, memberships)
			if err != nil {
				h.ErrLog.LogServerError(w, r, "database error batch adding memberships", err, "A database error occurred.", uploadURL)
				return
			}
			addedToGroup = msResult.Added
			alreadyInGroup = msResult.Duplicates
		}
	}

	// Show summary
	enabledMethods := wsauth.GetEnabledAuthMethods(ctx, r, h.DB)
	summaryData := UploadData{
		BaseVM:         viewdata.NewBaseVM(r, h.DB, "Upload CSV - Complete", returnURL),
		OrgHex:         uc.OrgID.Hex(),
		OrgName:        uc.OrgName,
		OrgLocked:      true,
		GroupMode:      uc.GroupMode,
		GroupID:        uc.GroupID.Hex(),
		GroupName:      uc.GroupName,
		GroupLocked:    true,
		ReturnURL:      returnURL,
		CSVAuthMethods: models.GetAuthMethodsForCSV(enabledMethods),
		ShowSummary:    true,
		Created:        result.Created,
		Updated:        result.Updated,
		SkippedCount:   result.Skipped,
		AddedToGroup:   addedToGroup,
		AlreadyInGroup: alreadyInGroup,
		CreatedMembers: result.CreatedMembers,
		UpdatedMembers: result.UpdatedMembers,
		SkippedMembers: result.SkippedMembers,
	}

	if uc.GroupID == primitive.NilObjectID {
		summaryData.GroupID = ""
	}

	// Use the return URL for navigation after completion
	summaryData.BackURL = returnURL
	templates.Render(w, r, "upload_csv", summaryData)
}

// batchFetchUsersByLoginID loads all users with the given login IDs in a single query.
func (h *Handler) batchFetchUsersByLoginID(ctx context.Context, loginIDs []string) (map[string]*models.User, error) {
	if len(loginIDs) == 0 {
		return make(map[string]*models.User), nil
	}

	filter := bson.M{"login_id": bson.M{"$in": loginIDs}}
	workspace.FilterCtx(ctx, filter)
	cur, err := h.DB.Collection("users").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	users := make(map[string]*models.User, len(loginIDs))
	for cur.Next(ctx) {
		var u models.User
		if err := cur.Decode(&u); err != nil {
			return nil, err
		}
		loginID := ""
		if u.LoginID != nil {
			loginID = *u.LoginID
		}
		users[strings.ToLower(loginID)] = &u
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}

	return users, nil
}
