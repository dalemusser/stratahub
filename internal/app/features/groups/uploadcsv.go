// internal/app/features/groups/uploadcsv.go
package groups

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/csvutil"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// groupCSVData is the view model for the CSV upload page.
type groupCSVData struct {
	viewdata.BaseVM

	GroupID   string
	GroupName string
	OrgName   string

	Error   template.HTML
	Success template.HTML

	// Summary
	Created        int // new users created in org (and added to this group)
	Previously     int // users already in the org
	AddedToGroup   int // among 'Previously', how many we just added to the group
	AlreadyInGroup int // among 'Previously', how many were already in the group
	SkippedCount   int
	SkippedEmails  []string
}

// batchFetchUsersByEmail loads all users with the given emails in a single query.
func (h *Handler) batchFetchUsersByEmail(ctx context.Context, emails []string) (map[string]*models.User, error) {
	if len(emails) == 0 {
		return make(map[string]*models.User), nil
	}

	cur, err := h.DB.Collection("users").Find(ctx, bson.M{"email": bson.M{"$in": emails}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	users := make(map[string]*models.User, len(emails))
	for cur.Next(ctx) {
		var u models.User
		if err := cur.Decode(&u); err != nil {
			return nil, err
		}
		users[strings.ToLower(u.Email)] = &u
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// ServeUploadCSV handles GET /groups/{id}/upload_csv.
func (h *Handler) ServeUploadCSV(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid group ID.", "/groups")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
		return
	}
	if err != nil {
		uierrors.RenderServerError(w, r, "A database error occurred.", "/groups")
		return
	}

	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You don't have permission to manage this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	orgName, err := orgutil.GetOrgName(ctx, db, group.OrganizationID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error loading organization for group", err, "A database error occurred.", "/groups")
		return
	}

	templates.Render(w, r, "group_upload_csv", groupCSVData{
		BaseVM:    viewdata.NewBaseVM(r, db, "Upload CSV", "/groups/"+group.ID.Hex()+"/manage"),
		GroupID:   group.ID.Hex(),
		GroupName: group.Name,
		OrgName:   orgName,
	})
}

// HandleUploadCSV handles POST /groups/{id}/upload_csv.
func (h *Handler) HandleUploadCSV(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, csvutil.MaxUploadSize)

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid group ID.", "/groups")
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Batch(), h.Log, "group CSV upload")
	defer cancel()

	group, err := groupstore.New(h.DB).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
		return
	}
	if err != nil {
		uierrors.RenderServerError(w, r, "A database error occurred.", "/groups")
		return
	}

	canManage, policyErr := grouppolicy.CanManageGroup(ctx, h.DB, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You don't have permission to manage this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	// Get uploaded file
	file, _, err := r.FormFile("csv")
	if err != nil {
		msg := "CSV file is required."
		if strings.Contains(err.Error(), "request body too large") {
			msg = "CSV file is too large. Maximum size is 5 MB."
		}
		uierrors.RenderBadRequest(w, r, msg, "/groups/"+gid+"/upload_csv")
		return
	}
	defer file.Close()

	// Parse and validate CSV
	parsed, parseErr := csvutil.ParseMemberCSV(file, csvutil.ParseOptions{MaxRows: csvutil.MaxRows})
	if parseErr == csvutil.ErrTooManyRows {
		uierrors.RenderBadRequest(w, r, "CSV contains too many rows.", "/groups/"+gid+"/upload_csv")
		return
	}
	if parseErr != nil {
		uierrors.RenderBadRequest(w, r, parseErr.Error(), "/groups/"+gid+"/upload_csv")
		return
	}

	// Return validation errors if any
	if parsed.HasErrors() {
		templates.Render(w, r, "group_upload_csv", groupCSVData{
			BaseVM:    viewdata.NewBaseVM(r, h.DB, "Upload CSV", "/groups/"+group.ID.Hex()+"/manage"),
			GroupID:   group.ID.Hex(),
			GroupName: group.Name,
			Error:     parsed.FormatErrorsHTML(3),
		})
		return
	}

	// Convert parsed rows to member entries for batch upsert
	entries := make([]userstore.MemberEntry, len(parsed.Rows))
	for i, row := range parsed.Rows {
		entries[i] = userstore.MemberEntry{
			FullName:   row.FullName,
			Email:      row.Email,
			AuthMethod: row.Auth,
		}
	}

	// Batch upsert users in the group's organization
	us := userstore.New(h.DB)
	upsertResult, err := us.UpsertMembersInOrgBatch(ctx, group.OrganizationID, entries)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error batch upserting users", err, "A database error occurred.", "/groups/"+gid+"/upload_csv")
		return
	}

	// Collect emails that were successfully processed (not skipped)
	skippedSet := make(map[string]struct{}, len(upsertResult.SkippedEmails))
	for _, email := range upsertResult.SkippedEmails {
		skippedSet[strings.ToLower(email)] = struct{}{}
	}

	// Deduplicate and collect processed emails
	seen := make(map[string]struct{})
	var processedEmails []string
	for _, row := range parsed.Rows {
		emailLower := strings.ToLower(row.Email)
		if _, skipped := skippedSet[emailLower]; skipped {
			continue
		}
		if _, dup := seen[emailLower]; dup {
			continue
		}
		seen[emailLower] = struct{}{}
		processedEmails = append(processedEmails, emailLower)
	}

	// Batch fetch users to get their IDs for membership creation
	users, err := h.batchFetchUsersByEmail(ctx, processedEmails)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error batch loading users", err, "A database error occurred.", "/groups/"+gid+"/upload_csv")
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
	var addedToGroup, alreadyInGroup int
	if len(memberships) > 0 {
		result, err := membershipstore.New(h.DB).AddBatch(ctx, group.ID, group.OrganizationID, memberships)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error batch adding memberships", err, "A database error occurred.", "/groups/"+gid+"/upload_csv")
			return
		}
		addedToGroup = result.Added
		alreadyInGroup = result.Duplicates
	}

	// Load org name for UI
	orgName, err := orgutil.GetOrgName(ctx, h.DB, group.OrganizationID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error loading organization", err, "A database error occurred.", "/groups/"+gid+"/upload_csv")
		return
	}

	templates.Render(w, r, "group_upload_csv", groupCSVData{
		BaseVM:         viewdata.NewBaseVM(r, h.DB, "Upload CSV", "/groups/"+group.ID.Hex()+"/manage"),
		GroupID:        group.ID.Hex(),
		GroupName:      group.Name,
		OrgName:        orgName,
		Created:        upsertResult.Created,
		Previously:     upsertResult.Updated,
		AddedToGroup:   addedToGroup,
		AlreadyInGroup: alreadyInGroup,
		SkippedCount:   upsertResult.Skipped,
		SkippedEmails:  upsertResult.SkippedEmails,
		Success:        template.HTML("Upload complete."),
	})
}
