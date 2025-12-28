// Package navigation provides helpers for safe URL navigation and redirects.
package navigation

import (
	"net/http"
	"strings"

	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/urlutil"
)

// BackURLOptions configures the behavior of SafeBackURL.
type BackURLOptions struct {
	// AllowedPrefix is the required URL prefix (e.g., "/members", "/system-users").
	// If empty, any safe URL is allowed.
	AllowedPrefix string

	// ExcludedSubpaths are subpath patterns to reject (e.g., "/edit", "/delete", "/new").
	// These prevent redirect loops back to action pages.
	ExcludedSubpaths []string

	// Fallback is the default URL if no valid return URL is found.
	Fallback string

	// PreserveQueryParam is an optional query parameter to preserve in the fallback URL.
	// For example, "org" would check for an org parameter and append it to the fallback.
	PreserveQueryParam string
}

// SafeBackURL extracts and validates a return URL from the request.
//
// It checks both the query parameter and form value for "return", validates
// the URL is safe (not an open redirect), optionally validates the prefix,
// and excludes specified subpaths to prevent redirect loops.
//
// Example usage:
//
//	url := navigation.SafeBackURL(r, navigation.BackURLOptions{
//	    AllowedPrefix:    "/members",
//	    ExcludedSubpaths: []string{"/upload_csv"},
//	    Fallback:         "/members",
//	    PreserveQueryParam: "org",
//	})
func SafeBackURL(r *http.Request, opts BackURLOptions) string {
	// Try query parameter first, then form value
	ret := urlutil.SafeReturn(query.Get(r, "return"), "", "")
	if ret == "" {
		ret = urlutil.SafeReturn(strings.TrimSpace(r.FormValue("return")), "", "")
	}

	// Validate against allowed prefix if specified
	if ret != "" {
		valid := true

		if opts.AllowedPrefix != "" && !strings.HasPrefix(ret, opts.AllowedPrefix) {
			valid = false
		}

		// Check excluded subpaths
		for _, excluded := range opts.ExcludedSubpaths {
			if strings.Contains(ret, excluded) {
				valid = false
				break
			}
		}

		if valid {
			return ret
		}
	}

	// Build fallback URL, optionally preserving a query parameter
	fallback := opts.Fallback
	if opts.PreserveQueryParam != "" {
		param := query.Get(r, opts.PreserveQueryParam)
		if param == "" {
			param = strings.TrimSpace(r.FormValue(opts.PreserveQueryParam))
		}
		if param == "" {
			// Also check common variations like "orgID" for "org"
			param = strings.TrimSpace(r.FormValue(opts.PreserveQueryParam + "ID"))
		}
		if param != "" && param != "all" {
			if strings.Contains(fallback, "?") {
				fallback += "&" + opts.PreserveQueryParam + "=" + param
			} else {
				fallback += "?" + opts.PreserveQueryParam + "=" + param
			}
		}
	}

	return fallback
}

// Common back URL configurations for reuse across packages.
var (
	// SystemUsersBackURL returns options for system-users pages.
	SystemUsersBackURL = BackURLOptions{
		AllowedPrefix:    "/system-users",
		ExcludedSubpaths: []string{"/edit", "/delete", "/new"},
		Fallback:         "/system-users",
	}

	// MembersBackURL returns options for members pages.
	MembersBackURL = BackURLOptions{
		AllowedPrefix:      "/members",
		ExcludedSubpaths:   []string{"/upload_csv"},
		Fallback:           "/members",
		PreserveQueryParam: "org",
	}

	// LeadersBackURL returns options for leaders pages.
	LeadersBackURL = BackURLOptions{
		AllowedPrefix:      "/leaders",
		ExcludedSubpaths:   []string{"/edit", "/delete", "/new"},
		Fallback:           "/leaders",
		PreserveQueryParam: "org",
	}

	// OrganizationsBackURL returns options for organizations pages.
	OrganizationsBackURL = BackURLOptions{
		AllowedPrefix:    "/organizations",
		ExcludedSubpaths: []string{"/edit", "/delete", "/new"},
		Fallback:         "/organizations",
	}

	// GroupsBackURL returns options for groups pages.
	GroupsBackURL = BackURLOptions{
		AllowedPrefix:      "/groups",
		ExcludedSubpaths:   []string{"/edit", "/delete", "/new"},
		Fallback:           "/groups",
		PreserveQueryParam: "org",
	}

	// ResourcesBackURL returns options for resources pages.
	ResourcesBackURL = BackURLOptions{
		AllowedPrefix:    "/resources",
		ExcludedSubpaths: []string{"/edit", "/delete", "/new"},
		Fallback:         "/resources",
	}

	// MaterialsBackURL returns options for materials pages.
	MaterialsBackURL = BackURLOptions{
		AllowedPrefix:    "/materials",
		ExcludedSubpaths: []string{"/edit", "/delete", "/new", "/assign"},
		Fallback:         "/materials",
	}
)
