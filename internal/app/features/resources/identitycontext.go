package resources

import (
	"context"

	"github.com/dalemusser/stratahub/internal/app/features/resources/resourceurl"
	"github.com/dalemusser/stratahub/internal/app/policy/resourcepolicy"
	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// resolveWorkspaceIdentity returns the workspace subdomain and hex ID for wsID.
// On lookup failure it returns ("", id.Hex()) so the hex dimension is still
// available even when the subdomain can't be loaded. A nil/zero id yields two
// empty strings.
func resolveWorkspaceIdentity(ctx context.Context, db *mongo.Database, wsID *primitive.ObjectID) (subdomain, idHex string) {
	if wsID == nil || wsID.IsZero() {
		return "", ""
	}
	ws, err := workspacestore.New(db).GetByID(ctx, *wsID)
	if err != nil {
		return "", wsID.Hex()
	}
	return ws.Subdomain, wsID.Hex()
}

// buildMemberIdentityContext assembles the identity values for a member opening
// a resource. groupID/groupName describe the group through which access was
// granted; orgName is the member's organization name (already resolved by the
// caller); wsSubdomain/wsID identify the workspace. Empty/zero values are left
// empty and get omitted from the URL by resourceurl.BuildLaunchURL.
func buildMemberIdentityContext(member *resourcepolicy.MemberInfo, wsSubdomain, wsID, orgName, groupName string, groupID primitive.ObjectID) resourceurl.IdentityContext {
	orgID := ""
	if member.OrganizationID != nil {
		orgID = member.OrganizationID.Hex()
	}
	groupIDHex := ""
	if !groupID.IsZero() {
		groupIDHex = groupID.Hex()
	}
	return resourceurl.IdentityContext{
		WorkspaceSubdomain: wsSubdomain,
		WorkspaceID:        wsID,
		OrgName:            orgName,
		OrgID:              orgID,
		GroupName:          groupName,
		GroupID:            groupIDHex,
		UserName:           member.FullName,
		UserID:             member.ID.Hex(),
		LoginID:            member.LoginID,
	}
}
