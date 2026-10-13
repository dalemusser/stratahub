package mhsactivity

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	activity "github.com/dalemusser/stratahub/internal/app/system/mhsactivity"
)

// Window ranges.
const (
	RangeToday     = "today"
	RangeYesterday = "yesterday"
	RangeWeek      = "7d"
	RangeCustom    = "custom"
)

// Window is the span of days the page shows. Today, yesterday and the last
// seven days are relative to each organization's local today; a custom range
// is the same day strings for every organization.
type Window struct {
	Range string
	From  string // custom only, "2006-01-02"
	To    string
}

// ParseWindow reads range, from and to. A custom range must be at most
// maxDays long; one with no dates yet means today (UTC).
func ParseWindow(r *http.Request, maxDays int, now time.Time) (Window, error) {
	q := r.URL.Query()
	w := Window{Range: strings.TrimSpace(q.Get("range"))}
	switch w.Range {
	case "", RangeToday:
		w.Range = RangeToday
	case RangeYesterday, RangeWeek:
	case RangeCustom:
		w.From, w.To = strings.TrimSpace(q.Get("from")), strings.TrimSpace(q.Get("to"))
		if w.From == "" && w.To == "" {
			// The range was just chosen: start from today so the date
			// inputs have something to edit.
			w.From = now.UTC().Format(activity.DayFormat)
			w.To = w.From
		}
		if w.From == "" {
			w.From = w.To
		}
		if w.To == "" {
			w.To = w.From
		}
		for _, d := range []string{w.From, w.To} {
			if _, err := time.Parse(activity.DayFormat, d); err != nil {
				return w, fmt.Errorf("%q is not a date (use 2006-01-02)", d)
			}
		}
		days, err := activity.DaysBetween(w.From, w.To)
		if err != nil {
			return w, errors.New("the to date is before the from date")
		}
		if maxDays > 0 && len(days) > maxDays {
			return w, fmt.Errorf("a range can cover at most %d days for now", maxDays)
		}
	default:
		return w, fmt.Errorf("unknown range %q", w.Range)
	}
	if w.Range == RangeWeek && maxDays > 0 && maxDays < 7 {
		return w, fmt.Errorf("a range can cover at most %d days for now", maxDays)
	}
	return w, nil
}

// DaysIn lists the window's days for one time zone, oldest first.
func (w Window) DaysIn(loc *time.Location, now time.Time) []string {
	today := activity.LocalDay(now, loc)
	switch w.Range {
	case RangeYesterday:
		d, _ := activity.AddDays(today, -1)
		return []string{d}
	case RangeWeek:
		first, _ := activity.AddDays(today, -6)
		days, _ := activity.DaysBetween(first, today)
		return days
	case RangeCustom:
		days, _ := activity.DaysBetween(w.From, w.To)
		return days
	}
	return []string{today}
}

// Label is the window as the page names it.
func (w Window) Label() string {
	switch w.Range {
	case RangeYesterday:
		return "Yesterday"
	case RangeWeek:
		return "Last 7 days"
	case RangeCustom:
		if w.From == w.To {
			return prettyDay(w.From)
		}
		return prettyDay(w.From) + " – " + prettyDay(w.To)
	}
	return "Today"
}

// Query reproduces the window as URL parameters.
func (w Window) Query() url.Values {
	v := url.Values{}
	v.Set("range", w.Range)
	if w.Range == RangeCustom {
		v.Set("from", w.From)
		v.Set("to", w.To)
	}
	return v
}

func prettyDay(day string) string {
	t, err := time.Parse(activity.DayFormat, day)
	if err != nil {
		return day
	}
	return t.Format("Jan 2, 2006")
}
