// internal/app/features/members/util.go
package members

import (
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// userCtx is a thin wrapper around authz.UserCtx for backwards compatibility.
func userCtx(r *http.Request) (role, name string, uid primitive.ObjectID, ok bool) {
	role, name, uid, ok = authz.UserCtx(r)
	return
}

// Stable list URL for Back – prevents loops to /members/upload_csv.
func backToMembersURL(r *http.Request) string {
	ret := strings.TrimSpace(r.URL.Query().Get("return"))
	if ret == "" {
		ret = strings.TrimSpace(r.FormValue("return"))
	}
	if strings.HasPrefix(ret, "/members") && !strings.HasPrefix(ret, "/members/upload_csv") {
		return ret
	}
	org := strings.TrimSpace(r.URL.Query().Get("org"))
	if org == "" {
		org = strings.TrimSpace(r.FormValue("org"))
		if org == "" {
			org = strings.TrimSpace(r.FormValue("orgID"))
		}
	}
	if org != "" && org != "all" {
		return "/members?org=" + org
	}
	return "/members"
}
