package viewers

import (
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// FilterType selects how a filter is rendered and parsed.
type FilterType string

const (
	FilterSelect    FilterType = "select"    // one of Options (empty value = any)
	FilterText      FilterType = "text"      // free text (viewer decides matching)
	FilterID        FilterType = "id"        // exact 24-char hex ObjectID
	FilterDateRange FilterType = "daterange" // preset or custom from/to, see Filters.DateRange
)

// Option is one choice of a select filter.
type Option struct {
	Value string
	Label string
}

// FilterSpec declares one filter control. Key is the query parameter name
// (a date range also uses Key+"_from" and Key+"_to" for the custom range).
type FilterSpec struct {
	Key         string
	Label       string
	Type        FilterType
	Options     []Option // FilterSelect only; an "any" option is added by the template
	Placeholder string   // FilterText / FilterID
	Default     string   // value when the parameter is absent (e.g. "7d" for a date range)
}

// Date-range presets accepted by a FilterDateRange filter. Anything else is
// treated as "any time".
const (
	RangeHour   = "1h"
	RangeDay    = "24h"
	RangeWeek   = "7d"
	RangeMonth  = "30d"
	RangeCustom = "custom"
	RangeAny    = ""
)

// DateRangeOptions are the preset choices rendered for a date-range filter.
var DateRangeOptions = []Option{
	{Value: RangeAny, Label: "Any time"},
	{Value: RangeHour, Label: "Last hour"},
	{Value: RangeDay, Label: "Last 24 hours"},
	{Value: RangeWeek, Label: "Last 7 days"},
	{Value: RangeMonth, Label: "Last 30 days"},
	{Value: RangeCustom, Label: "Custom (UTC)…"},
}

// customLayout is the datetime-local input format, interpreted as UTC.
const customLayout = "2006-01-02T15:04"

// Filters holds the parsed filter values for one request. Values are trimmed
// strings keyed by FilterSpec.Key (plus the _from/_to suffixes of date
// ranges); only declared keys are kept.
type Filters struct {
	values map[string]string
}

// ParseFilters extracts the declared filters from a query, applying each
// spec's Default when the parameter is absent.
func ParseFilters(specs []FilterSpec, q url.Values) Filters {
	f := Filters{values: make(map[string]string, len(specs))}
	for _, s := range specs {
		if _, present := q[s.Key]; present {
			f.values[s.Key] = strings.TrimSpace(q.Get(s.Key))
		} else {
			f.values[s.Key] = s.Default
		}
		if s.Type == FilterDateRange {
			f.values[s.Key+"_from"] = strings.TrimSpace(q.Get(s.Key + "_from"))
			f.values[s.Key+"_to"] = strings.TrimSpace(q.Get(s.Key + "_to"))
		}
	}
	return f
}

// Get returns the value for key ("" when unset).
func (f Filters) Get(key string) string { return f.values[key] }

// Has reports whether the filter has a non-empty value.
func (f Filters) Has(key string) bool { return f.values[key] != "" }

// ID returns the value as an ObjectID, if it is a valid 24-char hex.
func (f Filters) ID(key string) (primitive.ObjectID, bool) {
	id, err := primitive.ObjectIDFromHex(f.values[key])
	if err != nil {
		return primitive.NilObjectID, false
	}
	return id, true
}

// DateRange resolves a date-range filter against now. ok is false for "any
// time" (no bounds). A zero from or to means that bound is open.
func (f Filters) DateRange(key string, now time.Time) (from, to time.Time, ok bool) {
	switch f.values[key] {
	case RangeHour:
		return now.Add(-time.Hour), time.Time{}, true
	case RangeDay:
		return now.Add(-24 * time.Hour), time.Time{}, true
	case RangeWeek:
		return now.Add(-7 * 24 * time.Hour), time.Time{}, true
	case RangeMonth:
		return now.Add(-30 * 24 * time.Hour), time.Time{}, true
	case RangeCustom:
		if s := f.values[key+"_from"]; s != "" {
			if t, err := time.ParseInLocation(customLayout, s, time.UTC); err == nil {
				from = t
			}
		}
		if s := f.values[key+"_to"]; s != "" {
			if t, err := time.ParseInLocation(customLayout, s, time.UTC); err == nil {
				to = t
			}
		}
		return from, to, !from.IsZero() || !to.IsZero()
	}
	return time.Time{}, time.Time{}, false
}

// Values returns the filters as query parameters (only non-empty values), so
// the framework can build table/export/load-more URLs and the page URL.
func (f Filters) Values() url.Values {
	q := url.Values{}
	for k, v := range f.values {
		if v != "" {
			q.Set(k, v)
		}
	}
	return q
}
