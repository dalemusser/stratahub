// internal/app/features/groups/uploadcsv.go
package groups

import (
	"context"
	"encoding/csv"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

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

// ServeUploadCSV handles GET /groups/{id}/upload_csv.
func (h *Handler) ServeUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		http.Error(w, "bad group id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	orgName := ""
	if !group.OrganizationID.IsZero() {
		var org models.Organization
		_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": group.OrganizationID}).Decode(&org)
		orgName = org.Name
	}

	templates.Render(w, r, "group_upload_csv", groupCSVData{
		Title:       "Upload CSV",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		GroupID:     group.ID.Hex(),
		GroupName:   group.Name,
		OrgName:     orgName,
		BackURL:     nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"),
		CurrentPath: nav.CurrentPath(r),
	})
}

// HandleUploadCSV handles POST /groups/{id}/upload_csv.
func (h *Handler) HandleUploadCSV(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		http.Error(w, "bad group id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	file, _, err := r.FormFile("csv")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	cr := csv.NewReader(file)
	cr.TrimLeadingSpace = true

	const maxRows = 20000

	// Pre-scan CSV to validate all rows before any writes
	type csvRow struct {
		Full  string
		Email string
		Auth  string
	}
	var rows []csvRow

	type badRow struct {
		line   int
		reason string
		raw    []string
	}
	var invalid []badRow

	line := 0
	allowedAuths := map[string]struct{}{
		"internal":  {},
		"google":    {},
		"classlink": {},
		"clever":    {},
		"microsoft": {},
	}

	for {
		rec, readErr := cr.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		line++
		if line > maxRows {
			http.Error(w, "CSV too large", http.StatusRequestEntityTooLarge)
			return
		}
		if len(rec) == 0 {
			continue
		}

		// Header/BOM-tolerant detection on first row
		if line == 1 {
			c0 := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(rec[0], "\ufeff")))
			c1, c2 := "", ""
			if len(rec) > 1 {
				c1 = strings.ToLower(strings.TrimSpace(rec[1]))
			}
			if len(rec) > 2 {
				c2 = strings.ToLower(strings.TrimSpace(rec[2]))
			}
			isHeader := (c0 == "full name" || c0 == "name") && c1 == "email"
			hasAuth := (c1 == "email" && strings.Contains(c2, "auth"))
			if isHeader || hasAuth {
				continue
			}
		}

		full := ""
		if len(rec) > 0 {
			full = strings.TrimSpace(rec[0])
		}
		email := ""
		if len(rec) > 1 {
			email = strings.TrimSpace(rec[1])
		}
		authm := ""
		if len(rec) > 2 {
			authm = strings.TrimSpace(rec[2])
		}

		authmLower := strings.ToLower(authm)

		if full == "" || email == "" || authm == "" {
			invalid = append(invalid, badRow{line: line, reason: "missing Full Name/Email/Auth Method", raw: rec})
			continue
		}
		if _, ok := allowedAuths[authmLower]; !ok {
			invalid = append(invalid, badRow{line: line, reason: "Auth Method not allowed", raw: rec})
			continue
		}

		rows = append(rows, csvRow{Full: full, Email: email, Auth: authmLower})
	}

	if len(invalid) > 0 {
		var hb strings.Builder
		hb.WriteString("Invalid CSV: ")
		hb.WriteString(strconv.Itoa(len(invalid)))
		hb.WriteString(" row(s) failed validation. Allowed Auth Methods: internal, google, classlink, clever, microsoft.<br>")
		maxShow := 3
		if len(invalid) < maxShow {
			maxShow = len(invalid)
		}
		for i := 0; i < maxShow; i++ {
			br := invalid[i]
			hb.WriteString("- row ")
			hb.WriteString(strconv.Itoa(br.line))
			hb.WriteString(": ")
			hb.WriteString(template.HTMLEscapeString(br.reason))
			if len(br.raw) > 0 {
				hb.WriteString(" | values: ")
				hb.WriteString(template.HTMLEscapeString(strings.Join(br.raw, ", ")))
			}
			hb.WriteString("<br>")
		}
		if len(invalid) > maxShow {
			hb.WriteString("... and ")
			hb.WriteString(strconv.Itoa(len(invalid) - maxShow))
			hb.WriteString(" more.")
		}

		templates.Render(w, r, "group_upload_csv", groupCSVData{
			Title:       "Upload CSV",
			IsLoggedIn:  true,
			Role:        role,
			UserName:    uname,
			GroupID:     group.ID.Hex(),
			GroupName:   group.Name,
			OrgName:     "",
			BackURL:     nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"),
			CurrentPath: nav.CurrentPath(r),
			Error:       template.HTML(hb.String()),
		})
		return
	}

	// Now process rows
	var created, previously, addedToGroup, alreadyInGroup, skipped int
	var skippedEmails []string

	seen := map[string]struct{}{}

	for _, row := range rows {
		emailLower := strings.ToLower(row.Email)

		// per-upload de-dupe
		if _, dup := seen[emailLower]; dup {
			continue
		}
		seen[emailLower] = struct{}{}

		now := time.Now()

		var u models.User
		findErr := db.Collection("users").FindOne(ctx, bson.M{"email": emailLower}).Decode(&u)

		switch {
		case mongodb.IsDup(findErr):
			// unlikely here, but treat as existing user
			if e2 := db.Collection("users").FindOne(ctx, bson.M{"email": emailLower}).Decode(&u); e2 != nil {
				http.Error(w, "db error", http.StatusInternalServerError)
				return
			}
			fallthrough

		case findErr == mongo.ErrNoDocuments:
			// create new user in this org
			orgPtr := group.OrganizationID
			doc := models.User{
				ID:             primitive.NewObjectID(),
				FullName:       row.Full,
				FullNameCI:     textfold.Fold(row.Full),
				Email:          emailLower,
				AuthMethod:     row.Auth,
				Role:           "member",
				Status:         "active",
				OrganizationID: &orgPtr,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			_, ierr := db.Collection("users").InsertOne(ctx, doc)
			if ierr != nil {
				if mongodb.IsDup(ierr) {
					// Reload user and treat as existing
					if e2 := db.Collection("users").FindOne(ctx, bson.M{"email": emailLower}).Decode(&u); e2 != nil {
						http.Error(w, "db error", http.StatusInternalServerError)
						return
					}
					// fallthrough to existing user handling below
				} else {
					http.Error(w, "db error", http.StatusInternalServerError)
					return
				}
			} else {
				created++
				// Add membership for new user
				err := membershipstore.New(db).Add(ctx, group.ID, doc.ID, "member")
				if err == membershipstore.ErrDuplicateMembership {
					alreadyInGroup++
				} else if err == nil {
					addedToGroup++
				} else {
					http.Error(w, "db error", http.StatusInternalServerError)
					return
				}
				continue
			}

			fallthrough

		case findErr == nil:
			// existing user: must belong to same org
			if u.OrganizationID == nil || *u.OrganizationID != group.OrganizationID {
				skipped++
				skippedEmails = append(skippedEmails, emailLower)
				continue
			}

			previously++

			// ensure full_name_ci is populated/correct
			folded := textfold.Fold(u.FullName)
			if u.FullNameCI == "" || u.FullNameCI != folded {
				_, _ = db.Collection("users").UpdateOne(
					ctx,
					bson.M{"_id": u.ID},
					bson.M{"$set": bson.M{"full_name_ci": folded, "updated_at": now}},
				)
			}

			// Add membership
			err := membershipstore.New(db).Add(ctx, group.ID, u.ID, "member")
			if err == membershipstore.ErrDuplicateMembership {
				alreadyInGroup++
			} else if err == nil {
				addedToGroup++
			} else {
				http.Error(w, "db error", http.StatusInternalServerError)
				return
			}

		default:
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
	}

	// load org name for UI
	var org models.Organization
	_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": group.OrganizationID}).Decode(&org)

	templates.Render(w, r, "group_upload_csv", groupCSVData{
		Title:       "Upload CSV",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		GroupID:     group.ID.Hex(),
		GroupName:   group.Name,
		OrgName:     org.Name,
		BackURL:     nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"),
		CurrentPath: nav.CurrentPath(r),

		Created:        created,
		Previously:     previously,
		AddedToGroup:   addedToGroup,
		AlreadyInGroup: alreadyInGroup,
		SkippedCount:   skipped,
		SkippedEmails:  skippedEmails,
		Success:        template.HTML("Upload complete."),
	})
}
