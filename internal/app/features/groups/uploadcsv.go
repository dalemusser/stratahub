// internal/app/features/groups/uploadcsv.go
package groups

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/csvutil"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// csvProcessResult holds the result of processing CSV rows into the database.
type csvProcessResult struct {
	Created       int
	Previously    int
	Skipped       int
	SkippedEmails []string
	Memberships   []membershipstore.MembershipEntry
}

// groupCSVData is the view model for the CSV upload page.
type groupCSVData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID   string
	GroupName string
	OrgName   string

	Error   template.HTML
	Success template.HTML

	BackURL     string
	CurrentPath string

	// Summary
	Created        int // new users created in org (and added to this group)
	Previously     int // users already in the org
	AddedToGroup   int // among 'Previously', how many we just added to the group
	AlreadyInGroup int // among 'Previously', how many were already in the group
	SkippedCount   int
	SkippedEmails  []string
}

// deduplicateCSVRows removes duplicate emails and returns unique emails with their row data.
func deduplicateCSVRows(rows []csvutil.MemberRow) ([]string, map[string]csvutil.MemberRow) {
	seen := map[string]struct{}{}
	uniqueEmails := make([]string, 0, len(rows))
	emailToRow := make(map[string]csvutil.MemberRow, len(rows))

	for _, row := range rows {
		emailLower := strings.ToLower(row.Email)
		if _, dup := seen[emailLower]; dup {
			continue
		}
		seen[emailLower] = struct{}{}
		uniqueEmails = append(uniqueEmails, emailLower)
		emailToRow[emailLower] = row
	}

	return uniqueEmails, emailToRow
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

// processCSVUsers processes deduplicated CSV rows, creating new users and collecting membership entries.
func (h *Handler) processCSVUsers(
	ctx context.Context,
	uniqueEmails []string,
	emailToRow map[string]csvutil.MemberRow,
	existingUsers map[string]*models.User,
	group *models.Group,
) (csvProcessResult, error) {
	var result csvProcessResult
	result.Memberships = make([]membershipstore.MembershipEntry, 0, len(uniqueEmails))
	now := time.Now()

	for _, emailLower := range uniqueEmails {
		row := emailToRow[emailLower]
		u, exists := existingUsers[emailLower]

		if !exists {
			// User doesn't exist - create new user in this org
			orgPtr := group.OrganizationID
			doc := models.User{
				ID:             primitive.NewObjectID(),
				FullName:       row.FullName,
				FullNameCI:     text.Fold(row.FullName),
				Email:          emailLower,
				AuthMethod:     row.Auth,
				Role:           "member",
				Status:         "active",
				OrganizationID: &orgPtr,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			_, ierr := h.DB.Collection("users").InsertOne(ctx, doc)
			if ierr != nil {
				if wafflemongo.IsDup(ierr) {
					// Race condition: reload and treat as existing
					var raceUser models.User
					if e2 := h.DB.Collection("users").FindOne(ctx, bson.M{"email": emailLower}).Decode(&raceUser); e2 != nil {
						return result, e2
					}
					if raceUser.OrganizationID == nil || *raceUser.OrganizationID != group.OrganizationID {
						actualOrg := "<nil>"
						if raceUser.OrganizationID != nil {
							actualOrg = raceUser.OrganizationID.Hex()
						}
						h.Log.Warn("CSV upload race condition: user created by concurrent request in different org",
							zap.String("email", emailLower),
							zap.String("target_org", group.OrganizationID.Hex()),
							zap.String("actual_org", actualOrg),
						)
						result.SkippedEmails = append(result.SkippedEmails, emailLower+" (concurrent create in different org)")
						result.Skipped++
						continue
					}
					result.Previously++
					result.Memberships = append(result.Memberships, membershipstore.MembershipEntry{
						UserID: raceUser.ID,
						Role:   "member",
					})
					continue
				}
				return result, ierr
			}
			result.Created++
			result.Memberships = append(result.Memberships, membershipstore.MembershipEntry{
				UserID: doc.ID,
				Role:   "member",
			})
			continue
		}

		// Existing user - check org membership
		if u.OrganizationID == nil || *u.OrganizationID != group.OrganizationID {
			result.SkippedEmails = append(result.SkippedEmails, emailLower)
			result.Skipped++
			continue
		}

		result.Previously++

		// Ensure full_name_ci is populated/correct
		folded := text.Fold(u.FullName)
		if u.FullNameCI == "" || u.FullNameCI != folded {
			if _, err := h.DB.Collection("users").UpdateOne(
				ctx,
				bson.M{"_id": u.ID},
				bson.M{"$set": bson.M{"full_name_ci": folded, "updated_at": now}},
			); err != nil {
				return result, err
			}
		}

		result.Memberships = append(result.Memberships, membershipstore.MembershipEntry{
			UserID: u.ID,
			Role:   "member",
		})
	}

	return result, nil
}

// ServeUploadCSV handles GET /groups/{id}/upload_csv.
func (h *Handler) ServeUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
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
		Title:       "Upload CSV",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		GroupID:     group.ID.Hex(),
		GroupName:   group.Name,
		OrgName:     orgName,
		BackURL:     httpnav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"),
		CurrentPath: httpnav.CurrentPath(r),
	})
}

// HandleUploadCSV handles POST /groups/{id}/upload_csv.
func (h *Handler) HandleUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
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
			Title:       "Upload CSV",
			IsLoggedIn:  true,
			Role:        role,
			UserName:    uname,
			GroupID:     group.ID.Hex(),
			GroupName:   group.Name,
			BackURL:     httpnav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"),
			CurrentPath: httpnav.CurrentPath(r),
			Error:       parsed.FormatErrorsHTML(3),
		})
		return
	}

	// Deduplicate and batch fetch existing users
	uniqueEmails, emailToRow := deduplicateCSVRows(parsed.Rows)

	existingUsers, err := h.batchFetchUsersByEmail(ctx, uniqueEmails)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error batch loading users", err, "A database error occurred.", "/groups/"+gid+"/upload_csv")
		return
	}

	// Process rows: create users, collect memberships
	processed, err := h.processCSVUsers(ctx, uniqueEmails, emailToRow, existingUsers, &group)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error processing CSV users", err, "A database error occurred.", "/groups/"+gid+"/upload_csv")
		return
	}

	// Batch add memberships
	var addedToGroup, alreadyInGroup int
	if len(processed.Memberships) > 0 {
		result, err := membershipstore.New(h.DB).AddBatch(ctx, group.ID, group.OrganizationID, processed.Memberships)
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
		Title:          "Upload CSV",
		IsLoggedIn:     true,
		Role:           role,
		UserName:       uname,
		GroupID:        group.ID.Hex(),
		GroupName:      group.Name,
		OrgName:        orgName,
		BackURL:        httpnav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"),
		CurrentPath:    httpnav.CurrentPath(r),
		Created:        processed.Created,
		Previously:     processed.Previously,
		AddedToGroup:   addedToGroup,
		AlreadyInGroup: alreadyInGroup,
		SkippedCount:   processed.Skipped,
		SkippedEmails:  processed.SkippedEmails,
		Success:        template.HTML("Upload complete."),
	})
}
