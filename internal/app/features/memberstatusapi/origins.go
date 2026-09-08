// internal/app/features/memberstatusapi/origins.go
//
// Allowed browser origins. A workspace admin lists, on Settings → Member
// Status API, the web-page origins that may call the API from JavaScript
// running in a browser. An origin is "scheme://host[:port]" exactly as a
// browser sends it in the Origin header: no path, no wildcard. The list is
// normalized on save (NormalizeOrigin) and compared exactly at request time
// (OriginAllowed) by the CORS middleware in cors.go.
package memberstatusapi

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// maxAllowedOrigins bounds the list; one provider needs one or two entries.
const maxAllowedOrigins = 20

// NormalizeOrigin validates one origin as an admin typed it and returns the
// canonical form the API compares against: lowercase scheme and host, the
// scheme's default port dropped, no trailing slash. The error text is meant
// for the settings page.
func NormalizeOrigin(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("origin is empty")
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return "", errors.New("origin cannot contain spaces")
	}
	if strings.Contains(s, "*") {
		return "", errors.New("wildcards are not allowed; list each origin exactly")
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Opaque != "" {
		return "", errors.New(`origin must look like "https://host"`)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", errors.New("origin must start with https://")
	}
	if u.User != nil {
		return "", errors.New("origin cannot contain credentials")
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("origin is scheme and host only; drop the path")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", errors.New(`origin must look like "https://host"`)
	}
	if scheme == "http" && !isLoopbackHost(host) {
		return "", errors.New("http is only allowed for localhost; use https://")
	}
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = "" // browsers omit the default port from Origin
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]" // IPv6 literal: Hostname() stripped the brackets
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host, nil
}

// isLoopbackHost reports whether host is a local-development host, where a
// plain http origin is acceptable.
func isLoopbackHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ParseAllowedOrigins parses the settings form's list: one origin per line
// (commas, semicolons, and whitespace also separate entries). Blank entries
// are dropped, each entry is normalized, and duplicates collapse, preserving
// order. The error names the entry that was refused.
func ParseAllowedOrigins(text string) ([]string, error) {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';' || r == ' ' || r == '\t'
	})
	var out []string
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		o, err := NormalizeOrigin(f)
		if err != nil {
			return nil, fmt.Errorf("%q: %v", f, err)
		}
		if seen[o] {
			continue
		}
		seen[o] = true
		out = append(out, o)
	}
	if len(out) > maxAllowedOrigins {
		return nil, fmt.Errorf("at most %d origins can be listed", maxAllowedOrigins)
	}
	return out, nil
}

// OriginAllowed reports whether origin, as a browser sent it, is one of the
// workspace's normalized entries. The comparison is exact apart from case:
// "https://www.example.com" does not admit "https://example.com", a
// different port, or a plain-http variant. The opaque origin "null" (files,
// sandboxed frames) is never allowed.
func OriginAllowed(allowed []string, origin string) bool {
	o := strings.ToLower(strings.TrimSpace(origin))
	if o == "" || o == "null" {
		return false
	}
	for _, a := range allowed {
		if a == o {
			return true
		}
	}
	return false
}
