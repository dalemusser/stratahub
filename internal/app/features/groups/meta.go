// internal/app/features/groups/meta.go
package groups

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Shared timeouts for groups feature handlers.
const (
	metaShortTimeout = 5 * time.Second
	metaMedTimeout   = 10 * time.Second
)

// orgOption represents an active organization choice.
type orgOption struct {
	ID   primitive.ObjectID
	Name string
}

// leaderOption represents an active leader that can be assigned to a group.
type leaderOption struct {
	ID       primitive.ObjectID
	FullName string
	OrgID    primitive.ObjectID
	OrgHex   string // populated for template filtering
}

// newGroupData is the view model for the "Add Group" page.
type newGroupData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	// Admin: list of orgs; Leader: their org shown read-only
	Organizations []orgOption
	LeaderOrgID   string
	LeaderOrgName string
	Error         string

	// Admin only: possible leaders across active orgs (filtered on the page)
	Leaders []leaderOption

	// Back/navigation
	BackURL     string
	CurrentPath string

	// Echo-on-error fields
	Name           string
	Description    string
	OrgHex         string
	SelectedLeader map[string]bool
}

// editGroupData is the view model for the Edit Group page.
type editGroupData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID          string
	Name             string
	Description      string
	OrganizationID   string
	OrganizationName string
	Error            string

	BackURL     string
	CurrentPath string
}

// assignedResourceViewItem is used on the read-only group view for
// listing resources assigned to a group.
type assignedResourceViewItem struct {
	ResourceID, ResourceTitle string
}

// groupViewData is the view model for the View Group page.
type groupViewData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID          string
	Name             string
	Description      string
	OrganizationName string
	LeadersCount     int
	MembersCount     int
	CreatedAt        time.Time
	UpdatedAt        time.Time

	AssignedResources []assignedResourceViewItem

	BackURL     string
	CurrentPath string
}

// groupResourceViewData is the view model for viewing a single resource
// in the context of a group.
type groupResourceViewData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID   string
	GroupName string

	ResourceID    string
	ResourceTitle string
	Subject       string
	Description   string
	Status        string
	LaunchURL     string

	BackURL     string
	CurrentPath string
}

// loadActiveOrgs returns (id,name) options for active organizations.
func loadActiveOrgs(ctx context.Context, db *mongo.Database) ([]orgOption, []primitive.ObjectID, error) {
	cur, err := db.Collection("organizations").Find(ctx, bson.M{"status": "active"})
	if err != nil {
		return nil, nil, err
	}
	defer cur.Close(ctx)

	var opts []orgOption
	var ids []primitive.ObjectID
	for cur.Next(ctx) {
		var o models.Organization
		if cur.Decode(&o) == nil {
			opts = append(opts, orgOption{ID: o.ID, Name: o.Name})
			ids = append(ids, o.ID)
		}
	}

	// Sort A→Z by name (case-insensitive, stable on original)
	sort.SliceStable(opts, func(i, j int) bool {
		ni := strings.ToLower(opts[i].Name)
		nj := strings.ToLower(opts[j].Name)
		if ni == nj {
			return opts[i].Name < opts[j].Name
		}
		return ni < nj
	})

	return opts, ids, nil
}

// loadActiveLeaders returns leader options for a set of org IDs (can be nil → all).
func loadActiveLeaders(ctx context.Context, db *mongo.Database, orgIDs []primitive.ObjectID) ([]leaderOption, error) {
	filter := bson.M{"role": "leader", "status": "active"}
	if len(orgIDs) > 0 {
		filter["organization_id"] = bson.M{"$in": orgIDs}
	}

	cu, err := db.Collection("users").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cu.Close(ctx)

	var out []leaderOption
	for cu.Next(ctx) {
		var u models.User
		if cu.Decode(&u) == nil && u.OrganizationID != nil {
			out = append(out, leaderOption{
				ID:       u.ID,
				FullName: u.FullName,
				OrgID:    *u.OrganizationID,
				OrgHex:   u.OrganizationID.Hex(),
			})
		}
	}

	// Sort A→Z by full name
	sort.SliceStable(out, func(i, j int) bool {
		ni := strings.ToLower(out[i].FullName)
		nj := strings.ToLower(out[j].FullName)
		if ni == nj {
			return out[i].FullName < out[j].FullName
		}
		return ni < nj
	})

	return out, nil
}

// toSet converts a slice of strings into a set (map[string]bool) with
// whitespace trimmed and empties removed.
func toSet(vals []string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			m[v] = true
		}
	}
	return m
}
