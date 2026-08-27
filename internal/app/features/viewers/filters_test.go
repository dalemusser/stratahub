package viewers_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var specs = []viewers.FilterSpec{
	{Key: "q", Label: "Search", Type: viewers.FilterText},
	{Key: "kind", Label: "Kind", Type: viewers.FilterSelect, Options: []viewers.Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}},
	{Key: "who", Label: "Student", Type: viewers.FilterID},
	{Key: "when", Label: "Received", Type: viewers.FilterDateRange, Default: viewers.RangeWeek},
}

func TestParseFilters(t *testing.T) {
	q := url.Values{}
	q.Set("q", "  hello ")
	q.Set("kind", "b")
	q.Set("who", "68f138c495cdf54a392b20aa")
	q.Set("unknown", "dropped")

	f := viewers.ParseFilters(specs, q)
	if f.Get("q") != "hello" || f.Get("kind") != "b" || !f.Has("who") {
		t.Errorf("parsed: q=%q kind=%q who=%q", f.Get("q"), f.Get("kind"), f.Get("who"))
	}
	if f.Get("unknown") != "" {
		t.Error("undeclared keys must be dropped")
	}
	// Absent parameter → spec default; present-but-empty → empty (explicit "any").
	if f.Get("when") != viewers.RangeWeek {
		t.Errorf("default not applied: %q", f.Get("when"))
	}
	q.Set("when", "")
	if viewers.ParseFilters(specs, q).Get("when") != "" {
		t.Error("explicit empty value should override the default")
	}

	id, ok := f.ID("who")
	if !ok || id.Hex() != "68f138c495cdf54a392b20aa" {
		t.Errorf("ID: %v %v", id, ok)
	}
	q.Set("who", "not-hex")
	if _, ok := viewers.ParseFilters(specs, q).ID("who"); ok {
		t.Error("invalid hex should not parse as an id")
	}

	// Values round-trips only non-empty values.
	v := f.Values()
	if v.Get("q") != "hello" || v.Get("kind") != "b" || v.Get("when") != viewers.RangeWeek || v.Has("unknown") {
		t.Errorf("Values: %v", v)
	}
	if _, has := v["who_from"]; has {
		t.Error("empty values must not be emitted")
	}
}

func TestFilters_DateRange(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	set := func(preset, from, to string) viewers.Filters {
		q := url.Values{"when": {preset}}
		if from != "" {
			q.Set("when_from", from)
		}
		if to != "" {
			q.Set("when_to", to)
		}
		return viewers.ParseFilters(specs, q)
	}

	for preset, want := range map[string]time.Duration{
		viewers.RangeHour: time.Hour, viewers.RangeDay: 24 * time.Hour,
		viewers.RangeWeek: 7 * 24 * time.Hour, viewers.RangeMonth: 30 * 24 * time.Hour,
	} {
		from, to, ok := set(preset, "", "").DateRange("when", now)
		if !ok || !to.IsZero() || !from.Equal(now.Add(-want)) {
			t.Errorf("%s: from=%v to=%v ok=%v", preset, from, to, ok)
		}
	}

	if _, _, ok := set(viewers.RangeAny, "", "").DateRange("when", now); ok {
		t.Error("any time should report ok=false")
	}
	if _, _, ok := set("bogus", "", "").DateRange("when", now); ok {
		t.Error("unknown preset should behave as any time")
	}

	from, to, ok := set(viewers.RangeCustom, "2026-08-01T09:30", "2026-08-02T00:00").DateRange("when", now)
	if !ok || !from.Equal(time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("custom: from=%v to=%v ok=%v", from, to, ok)
	}
	from, to, ok = set(viewers.RangeCustom, "2026-08-01T09:30", "").DateRange("when", now)
	if !ok || from.IsZero() || !to.IsZero() {
		t.Errorf("custom open-ended: from=%v to=%v ok=%v", from, to, ok)
	}
	if _, _, ok := set(viewers.RangeCustom, "garbage", "").DateRange("when", now); ok {
		t.Error("custom with unparsable bounds should be any time")
	}
}

func TestCursor_RoundTrip(t *testing.T) {
	id := primitive.NewObjectID()
	ts := time.Date(2026, 8, 26, 14, 3, 11, 123456789, time.UTC)
	c := viewers.Cursor{Key: viewers.TimeKey(ts), ID: id}

	parsed, err := viewers.ParseCursor(c.String())
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	if parsed.ID != id || parsed.Key != c.Key {
		t.Errorf("round trip: %+v vs %+v", parsed, c)
	}
	back, err := viewers.ParseTimeKey(parsed.Key)
	if err != nil || !back.Equal(ts) {
		t.Errorf("time key: %v %v", back, err)
	}

	if c, err := viewers.ParseCursor(""); c != nil || err != nil {
		t.Errorf("empty cursor should be nil,nil: %v %v", c, err)
	}
	for _, bad := range []string{"!!!", "bm9waXBl", viewers.Cursor{Key: "k", ID: id}.String() + "x"} {
		if _, err := viewers.ParseCursor(bad); err == nil {
			t.Errorf("%q should be a bad cursor", bad)
		}
	}
	// Keys containing the separator still round-trip (last separator wins).
	c2 := viewers.Cursor{Key: "a|b|c", ID: id}
	if p, err := viewers.ParseCursor(c2.String()); err != nil || p.Key != "a|b|c" {
		t.Errorf("separator in key: %+v %v", p, err)
	}
}
