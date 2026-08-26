// Package memberstatuscfg loads the list of externally-tracked entities
// (today: the Abt surveys) that the Member Status API accepts and the MHS
// Dashboard displays. It is a neutral home for that list because three
// features need it — the dashboard, the API, and the resource forms — and
// none of them should import the others.
//
// The list lives in the embedded file mhs_member_status.json, next to the
// dashboard's progress-point configuration, and follows the same
// load-once-at-startup pattern. Changing the number, names, or order of
// entities is an edit to that file followed by a rebuild and deploy.
// See docs/member-status-api/plan.md.
package memberstatuscfg

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	appresources "github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/waffle/pantry/text"
)

// FileName is the embedded configuration file.
const FileName = "mhs_member_status.json"

// UnknownKeyPrefix marks an entity key derived from a name the configuration
// does not recognize. Such keys never collide with configured item ids.
const UnknownKeyPrefix = "name:"

// Item is one tracked entity — a dashboard column and the set of names the
// external provider may use for it.
type Item struct {
	ID          string   `json:"id"`          // stable internal id; the canonical entity key
	Title       string   `json:"title"`       // display title (dashboard column header, resource form)
	ShortName   string   `json:"short_name"`  // compact label for narrow cells
	APINames    []string `json:"api_names"`   // names the provider may send (matched case-insensitively)
	Description string   `json:"description"` // shown in tooltips/modals
}

// Config is the parsed configuration.
type Config struct {
	TabTitle string `json:"tab_title"` // dashboard tab title (e.g. "Surveys")
	Items    []Item `json:"items"`     // in display order

	byName map[string]int // folded name → index into Items
	byID   map[string]int // id → index into Items
}

var (
	loaded  *Config
	loadErr error
	once    sync.Once
)

// Load parses the embedded configuration once and caches it for the life of
// the process. A malformed file is reported on every call so startup fails
// fast rather than silently running without tracked entities.
func Load() (*Config, error) {
	once.Do(func() {
		data, err := appresources.FS.ReadFile(FileName)
		if err != nil {
			loadErr = fmt.Errorf("memberstatuscfg: read %s: %w", FileName, err)
			return
		}
		loaded, loadErr = Parse(data)
	})
	return loaded, loadErr
}

// Parse builds a Config from JSON and validates it: every item needs a
// non-empty, unique id and a title, and no name may map to two items.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("memberstatuscfg: parse: %w", err)
	}
	if err := cfg.index(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// index builds the lookup maps and validates the items.
func (c *Config) index() error {
	c.byName = make(map[string]int)
	c.byID = make(map[string]int)
	for i, item := range c.Items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return fmt.Errorf("memberstatuscfg: item %d has no id", i)
		}
		if strings.HasPrefix(id, UnknownKeyPrefix) {
			return fmt.Errorf("memberstatuscfg: item %q: ids may not start with %q", id, UnknownKeyPrefix)
		}
		if strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("memberstatuscfg: item %q has no title", id)
		}
		if _, dup := c.byID[id]; dup {
			return fmt.Errorf("memberstatuscfg: duplicate item id %q", id)
		}
		c.Items[i].ID = id
		c.byID[id] = i

		// Every api name, the title, and the id itself resolve to the item.
		names := append([]string{item.Title, id}, item.APINames...)
		for _, n := range names {
			key := fold(n)
			if key == "" {
				continue
			}
			if j, dup := c.byName[key]; dup && j != i {
				return fmt.Errorf("memberstatuscfg: name %q maps to both %q and %q", n, c.Items[j].ID, id)
			}
			c.byName[key] = i
		}
	}
	return nil
}

// fold normalizes a name for matching: trimmed, case- and diacritic-insensitive,
// with internal whitespace runs collapsed to a single space.
func fold(name string) string {
	return strings.Join(strings.Fields(text.Fold(name)), " ")
}

// Find returns the item with the given id.
func (c *Config) Find(id string) (Item, bool) {
	if c == nil {
		return Item{}, false
	}
	i, ok := c.byID[id]
	if !ok {
		return Item{}, false
	}
	return c.Items[i], true
}

// Resolve returns the item a provider-supplied (or admin-typed) name refers
// to, matching case-insensitively against each item's api names, title, and id.
func (c *Config) Resolve(name string) (Item, bool) {
	if c == nil {
		return Item{}, false
	}
	i, ok := c.byName[fold(name)]
	if !ok {
		return Item{}, false
	}
	return c.Items[i], true
}

// KeyFor returns the canonical entity key for a name: the matching item's id
// when the name is recognized, otherwise UnknownKey(name). The returned Item
// is zero when known is false.
func (c *Config) KeyFor(name string) (key string, item Item, known bool) {
	if item, ok := c.Resolve(name); ok {
		return item.ID, item, true
	}
	return UnknownKey(name), Item{}, false
}

// UnknownKey derives a stable entity key for a name the configuration does
// not recognize, so events for it are stored (and later matched, once the
// name is added to the configuration) rather than lost.
func UnknownKey(name string) string {
	return UnknownKeyPrefix + fold(name)
}

// APINames returns the primary provider-facing name of each item, in display
// order — what the API's ping endpoint reports so a provider can verify the
// exact strings in use.
func (c *Config) APINames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Items))
	for _, item := range c.Items {
		if len(item.APINames) > 0 {
			names = append(names, item.APINames[0])
		} else {
			names = append(names, item.Title)
		}
	}
	return names
}
