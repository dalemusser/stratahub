// internal/domain/models/mhsdevicetest.go
package models

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MHSDeviceTestUserIDPrefix marks a game user id issued by the Mission
// HydroSci device test. A real users._id is a MongoDB ObjectID whose first 8
// hex characters are its creation time in seconds; "ffffffff" decodes to the
// year 2106, so no real id can ever start with it, and the marker is
// unmistakable in any log or export. The remaining 16 hex characters are
// random, so ids never collide with each other either. The same value is the
// test record's _id, the id in the run URL, and the user_id the game sends to
// stratalog and stratasave (which accept any well-formed 24-hex id).
const MHSDeviceTestUserIDPrefix = "ffffffff"

var mhsDeviceTestUserIDRe = regexp.MustCompile(`^ffffffff[0-9a-f]{16}$`)

// IsMHSDeviceTestUserID reports whether a 24-hex user id was issued by the
// device test. Downstream consumers (exports, the grader, support tooling)
// can use it to keep test traffic apart from students.
func IsMHSDeviceTestUserID(hexID string) bool {
	return mhsDeviceTestUserIDRe.MatchString(hexID)
}

// NewMHSDeviceTestUserID mints a device-test id: the marker followed by 16
// random hex characters (64 random bits).
func NewMHSDeviceTestUserID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return MHSDeviceTestUserIDPrefix + hex.EncodeToString(b[:]), nil
}

// ClipRunes shortens s to at most n runes (no marker), for bounded storage
// of free-text fields.
func ClipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Device-test kinds: a run from the public device test, or a member's load
// record stored on failure/completion (plan step A2).
const (
	MHSDeviceTestKindDeviceTest = "devicetest"
	MHSDeviceTestKindMember     = "member"
)

// Stages a run moves through, in order. "failed" is set when the most recent
// step is a failure (the page keeps retrying, so a later success moves the
// stage forward again).
const (
	MHSDeviceTestStageForm        = "form"
	MHSDeviceTestStageRun         = "run"
	MHSDeviceTestStageDownloading = "downloading"
	MHSDeviceTestStageDownloaded  = "downloaded"
	MHSDeviceTestStageLaunching   = "launching"
	MHSDeviceTestStageGameplay    = "gameplay"
	MHSDeviceTestStageCompleted   = "completed"
	MHSDeviceTestStageFailed      = "failed"
)

// MHSDeviceTestMaxSteps caps the embedded step log (a $push with $slice keeps
// the newest entries).
const MHSDeviceTestMaxSteps = 300

// MHSDeviceTestForm is what the tester filled in before starting.
type MHSDeviceTestForm struct {
	School        string `bson:"school" json:"school"`
	TesterName    string `bson:"tester_name" json:"tester_name"`
	TesterRole    string `bson:"tester_role,omitempty" json:"tester_role,omitempty"`
	TesterEmail   string `bson:"tester_email,omitempty" json:"tester_email,omitempty"`
	DeviceType    string `bson:"device_type,omitempty" json:"device_type,omitempty"`
	ManagedDevice string `bson:"managed_device,omitempty" json:"managed_device,omitempty"` // yes | no | unknown
	NetworkType   string `bson:"network_type,omitempty" json:"network_type,omitempty"`
	Notes         string `bson:"notes,omitempty" json:"notes,omitempty"`
}

// MHSDeviceTestStep is one entry of the page's step log (mhs-steplog.js).
type MHSDeviceTestStep struct {
	T      int64             `bson:"t" json:"t"`   // ms since the page's log started
	At     time.Time         `bson:"at" json:"at"` // wall clock
	Step   string            `bson:"step" json:"step"`
	State  string            `bson:"state" json:"state"` // running | ok | warn | fail | info
	Msg    string            `bson:"msg" json:"msg"`
	Detail map[string]string `bson:"detail,omitempty" json:"detail,omitempty"`
}

// MHSDeviceTestDownload summarizes the unit download for the run.
type MHSDeviceTestDownload struct {
	Path     string  `bson:"path,omitempty" json:"path,omitempty"` // background | direct
	Bytes    int64   `bson:"bytes,omitempty" json:"bytes,omitempty"`
	Seconds  float64 `bson:"seconds,omitempty" json:"seconds,omitempty"`
	AvgBps   float64 `bson:"avg_bps,omitempty" json:"avg_bps,omitempty"`
	Switched bool    `bson:"switched,omitempty" json:"switched,omitempty"`
	Stalls   int     `bson:"stalls,omitempty" json:"stalls,omitempty"`
	Retries  int     `bson:"retries,omitempty" json:"retries,omitempty"`
}

// MHSDeviceTestLaunch summarizes the game launch timings (milliseconds).
type MHSDeviceTestLaunch struct {
	LoaderMs     int64 `bson:"loader_ms,omitempty" json:"loader_ms,omitempty"`
	UnityMs      int64 `bson:"unity_ms,omitempty" json:"unity_ms,omitempty"`
	FirstFrameMs int64 `bson:"first_frame_ms,omitempty" json:"first_frame_ms,omitempty"`
}

// MHSDeviceTestReport is a note the tester sent from the page.
type MHSDeviceTestReport struct {
	At   time.Time `bson:"at" json:"at"`
	Note string    `bson:"note" json:"note"`
}

// MHSDeviceTest is one run of the device test (kind "devicetest") or one
// member's stored load record (kind "member"). For a device test the _id is
// the marked 24-hex id that is also the game's user_id.
type MHSDeviceTest struct {
	ID          primitive.ObjectID `bson:"_id" json:"id"`
	WorkspaceID primitive.ObjectID `bson:"workspace_id" json:"workspace_id"`
	Kind        string             `bson:"kind" json:"kind"`

	// Member kind only: whose load record this is.
	UserID         *primitive.ObjectID `bson:"user_id,omitempty" json:"user_id,omitempty"`
	OrganizationID *primitive.ObjectID `bson:"organization_id,omitempty" json:"organization_id,omitempty"`

	UnitID          string `bson:"unit_id" json:"unit_id"`
	UnitVersion     string `bson:"unit_version" json:"unit_version"`
	BuildIdentifier string `bson:"build_identifier,omitempty" json:"build_identifier,omitempty"`
	CollectionName  string `bson:"collection_name,omitempty" json:"collection_name,omitempty"`
	DeviceID        string `bson:"device_id,omitempty" json:"device_id,omitempty"`

	Form MHSDeviceTestForm `bson:"form" json:"form"`

	RemoteIP   string     `bson:"remote_ip,omitempty" json:"remote_ip,omitempty"`
	UserAgent  string     `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	StartedAt  time.Time  `bson:"started_at" json:"started_at"`
	LastSeenAt time.Time  `bson:"last_seen_at" json:"last_seen_at"`
	EndedAt    *time.Time `bson:"ended_at,omitempty" json:"ended_at,omitempty"`
	ExpiresAt  time.Time  `bson:"expires_at" json:"expires_at"` // writes are refused after this

	// Summary fields extracted from the diagnostics snapshot for columns and
	// filters; the full snapshot is kept verbatim in Diagnostics.
	DeviceType  string                 `bson:"device_type,omitempty" json:"device_type,omitempty"`
	Platform    string                 `bson:"platform,omitempty" json:"platform,omitempty"`
	Browser     string                 `bson:"browser,omitempty" json:"browser,omitempty"`
	Network     string                 `bson:"network,omitempty" json:"network,omitempty"`
	Diagnostics map[string]interface{} `bson:"diagnostics,omitempty" json:"diagnostics,omitempty"`

	Stage        string `bson:"stage" json:"stage"`                                   // current: the furthest stage reached, or "failed" while the latest step is a failure
	ReachedStage string `bson:"reached_stage,omitempty" json:"reached_stage,omitempty"` // furthest stage reached, kept while Stage is "failed"
	FailedStep   string `bson:"failed_step,omitempty" json:"failed_step,omitempty"`
	FailedReason string `bson:"failed_reason,omitempty" json:"failed_reason,omitempty"`

	Download          MHSDeviceTestDownload `bson:"download" json:"download"`
	Launch            MHSDeviceTestLaunch   `bson:"launch" json:"launch"`
	GameplayReachedAt *time.Time            `bson:"gameplay_reached_at,omitempty" json:"gameplay_reached_at,omitempty"`
	UnitCompletedAt   *time.Time            `bson:"unit_completed_at,omitempty" json:"unit_completed_at,omitempty"`
	CrashCount        int                   `bson:"crash_count,omitempty" json:"crash_count,omitempty"`

	Steps          []MHSDeviceTestStep   `bson:"steps,omitempty" json:"steps,omitempty"`
	ProblemReports []MHSDeviceTestReport `bson:"problem_reports,omitempty" json:"problem_reports,omitempty"`
}
