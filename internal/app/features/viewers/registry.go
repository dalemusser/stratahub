package viewers

import (
	"fmt"
	"sync"

	"github.com/dalemusser/stratahub/internal/app/system/normalize"
)

// Registry holds the viewers available in this process, in registration
// order. Bootstrap registers them; the handler and the menu consult it.
type Registry struct {
	mu     sync.RWMutex
	bySlug map[string]Viewer
	order  []Viewer
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{bySlug: make(map[string]Viewer)}
}

// Register adds a viewer. A duplicate or empty slug is a programming error
// and panics at startup.
func (r *Registry) Register(v Viewer) {
	slug := v.Slug()
	if slug == "" {
		panic("viewers: viewer with empty slug")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.bySlug[slug]; dup {
		panic(fmt.Sprintf("viewers: duplicate viewer slug %q", slug))
	}
	r.bySlug[slug] = v
	r.order = append(r.order, v)
}

// Get returns the viewer with the given slug.
func (r *Registry) Get(slug string) (Viewer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.bySlug[slug]
	return v, ok
}

// All returns every registered viewer in registration order.
func (r *Registry) All() []Viewer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Viewer(nil), r.order...)
}

// ForRole returns the viewers the role may open, in registration order.
func (r *Registry) ForRole(role string) []Viewer {
	var out []Viewer
	for _, v := range r.All() {
		if Allowed(v, role) {
			out = append(out, v)
		}
	}
	return out
}

// Allowed reports whether the role may open the viewer. Superadmin always may.
func Allowed(v Viewer, role string) bool {
	role = normalize.Role(role)
	if role == "superadmin" {
		return true
	}
	for _, allowed := range v.Roles() {
		if normalize.Role(allowed) == role {
			return true
		}
	}
	return false
}
