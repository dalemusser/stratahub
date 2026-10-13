// Package mhsactivity serves the MHS Activity page: a cross-school overview of
// what Mission HydroSci students did over a window of days, computed from the
// records the pages and the game leave behind (internal/app/system/mhsactivity).
// See docs/mhs-activity/plan.md.
package mhsactivity

import (
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
)

// Config is the feature's configuration (bootstrap appconfig).
type Config struct {
	Enabled       bool
	CloseAfter    time.Duration // how long after its end a day stays open (live)
	CacheTTL      time.Duration // how long a live day's computation is reused
	MaxLiveDays   int           // how many days one request may compute live
	MaxDays       int           // the longest window (stored days fill the rest)
	BackfillStart string        // the first day the backfill offers by default
}

// adminVM is the admin page's view model.
type adminVM struct {
	viewdata.BaseVM
	From, To   string
	Status     backfillVM
	StartError string
}

// backfillVM is the backfill status as the page shows it.
type backfillVM struct {
	Running    bool
	Ran        bool // a backfill has run since the service started
	Range      string
	Options    string
	StartedAt  string
	FinishedAt string
	Total      int
	Done       int
	Skipped    int
	Open       int
	Written    int
	Stamped    int
	Current    string
	Errors     []string
	Stopped    bool
	Percent    int
}

// pageVM is the page's view model.
type pageVM struct {
	viewdata.BaseVM

	Window        windowVM
	Orgs          []optionVM
	Groups        []optionVM
	SelectedOrg   string
	SelectedGroup string
	ShowStaff     bool

	Freshness []string // one line per computed zone-day
	Missing   []string // sources that were unavailable, if any
	Report    Report

	// Query string (without a leading ?) that reproduces the window and the
	// staff switch, for links that change the organization or group.
	WindowQuery string

	IsAdmin bool // shows the link to the admin (backfill) page
}

type windowVM struct {
	Range   string
	From    string
	To      string
	Label   string
	Custom  bool
	MaxDays int
}

type optionVM struct {
	ID       string
	Name     string
	OrgID    string
	Selected bool
}
