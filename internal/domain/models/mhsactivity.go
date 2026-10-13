// internal/domain/models/mhsactivity.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MHSActivityVersion is the version of the activity computation. A stored
// day whose Version is older is recomputed by the next backfill.
const MHSActivityVersion = 1

// Who a day document is about.
const (
	MHSActivityRoleMember = "member" // a student
	MHSActivityRoleStaff  = "staff"  // a teacher's, coordinator's or administrator's own launches
)

// MHSActivityUnit.State values, from best to worst.
const (
	MHSActivityUnitCached     = "cached"      // already on the device; nothing downloaded
	MHSActivityUnitComplete   = "complete"    // downloaded without a hold
	MHSActivityUnitHeldDone   = "held_done"   // held near the end, then released and completed
	MHSActivityUnitInProgress = "in_progress" // started, no end seen
	MHSActivityUnitHeld       = "held"        // held near the end, never completed that day
	MHSActivityUnitFailed     = "failed"      // a failure and no completion
)

// MHSActivityDay is what one student (or staff member) did with Mission
// HydroSci on one calendar day in their organization's time zone: whether
// they opened the units page, what the downloads did, whether the game
// launched and played, crashes, and the devices they used. One document per
// (workspace, user, day). Open days are computed on request and not stored;
// closed days are stored once by the rollup job (Final).
type MHSActivityDay struct {
	ID             primitive.ObjectID   `bson:"_id,omitempty" json:"id,omitempty"`
	WorkspaceID    primitive.ObjectID   `bson:"workspace_id" json:"workspace_id"`
	UserID         primitive.ObjectID   `bson:"user_id" json:"user_id"`
	OrganizationID primitive.ObjectID   `bson:"organization_id" json:"organization_id"`
	GroupIDs       []primitive.ObjectID `bson:"group_ids,omitempty" json:"group_ids,omitempty"`
	Role           string               `bson:"role" json:"role"` // MHSActivityRoleMember | MHSActivityRoleStaff
	Day            string               `bson:"day" json:"day"`   // "2006-01-02", local to the organization
	TZ             string               `bson:"tz" json:"tz"`     // IANA zone; "UTC" when the organization has none
	TZFallback     bool                 `bson:"tz_fallback,omitempty" json:"tz_fallback,omitempty"`

	Opened   bool       `bson:"opened" json:"opened"`     // the units page was opened (page view, load record, device status) or the game logged
	Launched bool       `bson:"launched" json:"launched"` // the game rendered at least once
	Played   bool       `bson:"played" json:"played"`     // the game sent log events
	Blocked  bool       `bson:"blocked" json:"blocked"`   // opened, never played, and a download or launch problem
	FirstAt  *time.Time `bson:"first_at,omitempty" json:"first_at,omitempty"`
	LastAt   *time.Time `bson:"last_at,omitempty" json:"last_at,omitempty"`

	Units   []MHSActivityUnit   `bson:"units,omitempty" json:"units,omitempty"` // one per (unit, device) download episode
	Devices []MHSActivityDevice `bson:"devices,omitempty" json:"devices,omitempty"`
	Launch  MHSActivityLaunch   `bson:"launch" json:"launch"`
	Crashes MHSActivityCrashes  `bson:"crashes" json:"crashes"`
	Log     MHSActivityLog      `bson:"log" json:"log"`
	Flags   MHSActivityFlags    `bson:"flags" json:"flags"`

	CurrentUnit string `bson:"current_unit,omitempty" json:"current_unit,omitempty"` // the grader's current unit when computed

	Records   int `bson:"records" json:"records"`                           // load records stored that day
	PageViews int `bson:"page_views,omitempty" json:"page_views,omitempty"` // units-page views that day

	Partial    bool      `bson:"partial,omitempty" json:"partial,omitempty"` // a source was unavailable (see Missing)
	Missing    []string  `bson:"missing,omitempty" json:"missing,omitempty"`
	Final      bool      `bson:"final" json:"final"` // stored for a closed day; false for a live result
	ComputedAt time.Time `bson:"computed_at" json:"computed_at"`
	Version    int       `bson:"version" json:"version"`
}

// MHSActivityUnit is one download episode: one unit on one device on one day,
// merged from every load record that described it.
type MHSActivityUnit struct {
	Unit     string `bson:"unit" json:"unit"`
	Version  string `bson:"version,omitempty" json:"version,omitempty"`
	DeviceID string `bson:"device_id,omitempty" json:"device_id,omitempty"`
	State    string `bson:"state" json:"state"`

	Requested bool `bson:"requested,omitempty" json:"requested,omitempty"`
	TotalMB   int  `bson:"total_mb,omitempty" json:"total_mb,omitempty"`
	MaxMB     int  `bson:"max_mb,omitempty" json:"max_mb,omitempty"`
	Drops     int  `bson:"drops,omitempty" json:"drops,omitempty"`
	Stalls    int  `bson:"stalls,omitempty" json:"stalls,omitempty"`
	Retries   int  `bson:"retries,omitempty" json:"retries,omitempty"`
	Fails     int  `bson:"fails,omitempty" json:"fails,omitempty"`

	LastErrorClass string `bson:"last_error_class,omitempty" json:"last_error_class,omitempty"`
	LastErrorText  string `bson:"last_error_text,omitempty" json:"last_error_text,omitempty"`
	LastPath       string `bson:"last_path,omitempty" json:"last_path,omitempty"`

	Cached          bool       `bson:"cached,omitempty" json:"cached,omitempty"`
	Complete        bool       `bson:"complete,omitempty" json:"complete,omitempty"`
	CompleteAt      *time.Time `bson:"complete_at,omitempty" json:"complete_at,omitempty"`
	CompleteMB      int        `bson:"complete_mb,omitempty" json:"complete_mb,omitempty"`
	CompleteSeconds int        `bson:"complete_s,omitempty" json:"complete_s,omitempty"`
	CompleteMBps    float64    `bson:"complete_mbps,omitempty" json:"complete_mbps,omitempty"`

	Held           bool       `bson:"held,omitempty" json:"held,omitempty"`
	HeldAtMB       int        `bson:"held_at_mb,omitempty" json:"held_at_mb,omitempty"`
	HeldAt         *time.Time `bson:"held_at,omitempty" json:"held_at,omitempty"`
	HoldSeconds    int        `bson:"hold_s,omitempty" json:"hold_s,omitempty"`
	BgfetchRefused bool       `bson:"bgfetch_refused,omitempty" json:"bgfetch_refused,omitempty"`
	BgfetchFrozen  bool       `bson:"bgfetch_frozen,omitempty" json:"bgfetch_frozen,omitempty"`

	FirstAt time.Time `bson:"first_at" json:"first_at"`
	LastAt  time.Time `bson:"last_at" json:"last_at"`
	Records int       `bson:"records,omitempty" json:"records,omitempty"` // load records that described this episode
}

// MHSActivityDevice is one browser profile the student used that day.
type MHSActivityDevice struct {
	DeviceID       string     `bson:"device_id" json:"device_id"`
	DeviceType     string     `bson:"device_type,omitempty" json:"device_type,omitempty"` // Chromebook | iPad | …
	Platform       string     `bson:"platform,omitempty" json:"platform,omitempty"`       // ChromeOS | Windows | macOS | iPadOS | iOS | Android | Linux | Other
	Browser        string     `bson:"browser,omitempty" json:"browser,omitempty"`         // "Chrome 152"
	New            bool       `bson:"new,omitempty" json:"new,omitempty"`                 // its device-status row was created that day: a fresh browser profile
	CreatedAt      *time.Time `bson:"created_at,omitempty" json:"created_at,omitempty"`
	Records        int        `bson:"records,omitempty" json:"records,omitempty"`
	LastAt         *time.Time `bson:"last_at,omitempty" json:"last_at,omitempty"`
	Persisted      *bool      `bson:"persisted,omitempty" json:"persisted,omitempty"`
	StorageQuotaMB int        `bson:"storage_quota_mb,omitempty" json:"storage_quota_mb,omitempty"`
}

// MHSActivityLaunch counts the day's launch events (distinct events, not
// records: records from one tab repeat each other's entries).
type MHSActivityLaunch struct {
	OK               int     `bson:"ok" json:"ok"`                                                   // the game rendered
	Stalled          int     `bson:"stalled" json:"stalled"`                                         // the 30 s watchdog fired
	StalledRecovered int     `bson:"stalled_recovered,omitempty" json:"stalled_recovered,omitempty"` // … and the game rendered within 10 min on that device
	Failed           int     `bson:"failed" json:"failed"`                                           // watchdog escalation, Unity failed, loader failed
	LoaderCache      int     `bson:"loader_cache,omitempty" json:"loader_cache,omitempty"`
	LoaderNetwork    int     `bson:"loader_network,omitempty" json:"loader_network,omitempty"`
	UnitySecondsLast float64 `bson:"unity_s_last,omitempty" json:"unity_s_last,omitempty"`
	UnitySecondsMax  float64 `bson:"unity_s_max,omitempty" json:"unity_s_max,omitempty"`
}

// MHSActivityCrashes splits the day's "Crash (…)" entries into browser
// notices the game survives and real failures.
type MHSActivityCrashes struct {
	Real   int        `bson:"real" json:"real"`
	Notice int        `bson:"notice" json:"notice"`
	Types  []string   `bson:"types,omitempty" json:"types,omitempty"` // distinct real crash types
	Last   *time.Time `bson:"last,omitempty" json:"last,omitempty"`
}

// MHSActivityLog is what the game's log service received that day.
type MHSActivityLog struct {
	Available   bool       `bson:"available" json:"available"`               // the log service could be read
	Source      string     `bson:"source,omitempty" json:"source,omitempty"` // "logdata", or "launch" when Played came from launch records because the log service was unavailable
	Events      int        `bson:"events,omitempty" json:"events,omitempty"`
	First       *time.Time `bson:"first,omitempty" json:"first,omitempty"`
	Last        *time.Time `bson:"last,omitempty" json:"last,omitempty"`
	CrashEvents int        `bson:"crash_events,omitempty" json:"crash_events,omitempty"`
}

// MHSActivityFlags are the day's problem signals, for filtering and pills.
type MHSActivityFlags struct {
	DownloadFailed      bool `bson:"download_failed,omitempty" json:"download_failed,omitempty"`
	Held                bool `bson:"held,omitempty" json:"held,omitempty"` // any episode held, released or not
	LaunchFailed        bool `bson:"launch_failed,omitempty" json:"launch_failed,omitempty"`
	BgfetchRefused      bool `bson:"bgfetch_refused,omitempty" json:"bgfetch_refused,omitempty"`
	StorageNotPersisted bool `bson:"storage_not_persisted,omitempty" json:"storage_not_persisted,omitempty"`
	NewDevice           bool `bson:"new_device,omitempty" json:"new_device,omitempty"`
	LoggingProblem      bool `bson:"logging_problem,omitempty" json:"logging_problem,omitempty"` // launched and heartbeating but no log events
}

// HasProblem reports whether the day shows something staff would act on.
func (d MHSActivityDay) HasProblem() bool {
	return d.Blocked || d.Flags.DownloadFailed || d.Flags.Held || d.Flags.LaunchFailed || d.Crashes.Real > 0
}

// MHSActivityMark records that an organization's day has been rolled up
// (one per workspace, organization and day). Final marks are never
// recomputed by the job; Partial ones are retried until Attempts runs out.
type MHSActivityMark struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	WorkspaceID    primitive.ObjectID `bson:"workspace_id" json:"workspace_id"`
	OrganizationID primitive.ObjectID `bson:"organization_id" json:"organization_id"`
	Day            string             `bson:"day" json:"day"`
	TZ             string             `bson:"tz" json:"tz"`
	Final          bool               `bson:"final" json:"final"`
	Partial        bool               `bson:"partial,omitempty" json:"partial,omitempty"`
	Missing        []string           `bson:"missing,omitempty" json:"missing,omitempty"`
	Attempts       int                `bson:"attempts" json:"attempts"`
	Students       int                `bson:"students" json:"students"`
	ComputedAt     time.Time          `bson:"computed_at" json:"computed_at"`
	Version        int                `bson:"version" json:"version"`
}
