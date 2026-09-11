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

// MHSDeviceTestHeartbeat is one periodic sample from the page: memory, frame
// rate and visibility while the game runs (phase "game"), or just liveness
// during the download (phase "download"). A renderer crash kills the page
// with nothing left to report, so the trend up to the last beat is the
// evidence; see MHSDeviceTest.StoppedResponding.
type MHSDeviceTestHeartbeat struct {
	At            time.Time `bson:"at" json:"at"`
	ElapsedMs     int64     `bson:"elapsed_ms,omitempty" json:"elapsed_ms,omitempty"` // since the launch (or the run page's load)
	Phase         string    `bson:"phase,omitempty" json:"phase,omitempty"`
	JSHeapMB      int       `bson:"js_heap_mb,omitempty" json:"js_heap_mb,omitempty"`
	JSHeapTotalMB int       `bson:"js_heap_total_mb,omitempty" json:"js_heap_total_mb,omitempty"`
	WasmHeapMB    int       `bson:"wasm_heap_mb,omitempty" json:"wasm_heap_mb,omitempty"`
	FPS           float64   `bson:"fps,omitempty" json:"fps,omitempty"`
	Visibility    string    `bson:"visibility,omitempty" json:"visibility,omitempty"`
}

// MHSDeviceTestMaxHeartbeats caps the stored trend (two hours at 30 s).
const MHSDeviceTestMaxHeartbeats = 240

// MHSDeviceTestHeartbeatGrace is how long after the last beat a run that has
// not ended cleanly counts as "page stopped responding".
const MHSDeviceTestHeartbeatGrace = 2 * time.Minute

// How a run ended.
const (
	MHSDeviceTestEndCompleted = "completed" // the game reported the unit complete
	MHSDeviceTestEndClosed    = "closed"    // the page said it was leaving (back, tab closed)
	MHSDeviceTestEndReset     = "reset"     // the tester pressed Reset this device (the run continues after the next launch)
)

// MHSDeviceTestQuestionnaire is what the tester answered after playing:
// the things the page cannot observe by itself. Values are the short codes
// in MHSDeviceTestQuestionnaireOptions; empty means not answered.
type MHSDeviceTestQuestionnaire struct {
	Sound       string    `bson:"sound,omitempty" json:"sound,omitempty"`             // worked | none | problems
	Controls    string    `bson:"controls,omitempty" json:"controls,omitempty"`       // worked | problems
	Display     string    `bson:"display,omitempty" json:"display,omitempty"`         // fine | problems
	Performance string    `bson:"performance,omitempty" json:"performance,omitempty"` // smooth | choppy | froze
	Progress    string    `bson:"progress,omitempty" json:"progress,omitempty"`       // farthest point reached: started | glyphs-on-wall | met-anderson | found-jasper | finished
	Notes       string    `bson:"notes,omitempty" json:"notes,omitempty"`
	AnsweredAt  time.Time `bson:"answered_at" json:"answered_at"`
}

// MHSDeviceTestQuestionnaireOptions lists the accepted codes per question.
var MHSDeviceTestQuestionnaireOptions = map[string][]string{
	"sound":       {"worked", "none", "problems"},
	"controls":    {"worked", "problems"},
	"display":     {"fine", "problems"},
	"performance": {"smooth", "choppy", "froze"},
	"progress":    {"started", "glyphs-on-wall", "met-anderson", "found-jasper", "finished"},
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

	Stage        string `bson:"stage" json:"stage"`                                     // current: the furthest stage reached, or "failed" while the latest step is a failure
	ReachedStage string `bson:"reached_stage,omitempty" json:"reached_stage,omitempty"` // furthest stage reached, kept while Stage is "failed"
	FailedStep   string `bson:"failed_step,omitempty" json:"failed_step,omitempty"`
	FailedReason string `bson:"failed_reason,omitempty" json:"failed_reason,omitempty"`

	Download          MHSDeviceTestDownload `bson:"download" json:"download"`
	Launch            MHSDeviceTestLaunch   `bson:"launch" json:"launch"`
	GameplayReachedAt *time.Time            `bson:"gameplay_reached_at,omitempty" json:"gameplay_reached_at,omitempty"`
	UnitCompletedAt   *time.Time            `bson:"unit_completed_at,omitempty" json:"unit_completed_at,omitempty"`
	// LastLaunchAt is the most recent launch activity (a launch step from
	// the run or play page, or a launch summary). LastResetAt and Resets
	// record "Reset this device": the run keeps its id and history, and the
	// run page treats it as not launched again until a launch after the reset.
	LastLaunchAt *time.Time `bson:"last_launch_at,omitempty" json:"last_launch_at,omitempty"`
	LastResetAt  *time.Time `bson:"last_reset_at,omitempty" json:"last_reset_at,omitempty"`
	Resets       int        `bson:"resets,omitempty" json:"resets,omitempty"`
	CrashCount   int        `bson:"crash_count,omitempty" json:"crash_count,omitempty"`

	Steps          []MHSDeviceTestStep   `bson:"steps,omitempty" json:"steps,omitempty"`
	ProblemReports []MHSDeviceTestReport `bson:"problem_reports,omitempty" json:"problem_reports,omitempty"`

	// The tester's post-play answers (nil until answered).
	Questionnaire *MHSDeviceTestQuestionnaire `bson:"questionnaire,omitempty" json:"questionnaire,omitempty"`

	// Heartbeats from the page (see MHSDeviceTestHeartbeat). LastHeartbeat
	// is kept apart from the capped trend so list views need not load it.
	EndReason       string                   `bson:"end_reason,omitempty" json:"end_reason,omitempty"`
	LastHeartbeatAt *time.Time               `bson:"last_heartbeat_at,omitempty" json:"last_heartbeat_at,omitempty"`
	LastHeartbeat   *MHSDeviceTestHeartbeat  `bson:"last_heartbeat,omitempty" json:"last_heartbeat,omitempty"`
	HeartbeatCount  int                      `bson:"heartbeat_count,omitempty" json:"heartbeat_count,omitempty"`
	Heartbeats      []MHSDeviceTestHeartbeat `bson:"heartbeats,omitempty" json:"heartbeats,omitempty"`
}

// Launched reports whether the game page was reached at least once, which
// is when the post-play questionnaire becomes relevant.
// LaunchedSinceReset reports whether the game was launched after the most
// recent Reset this device (or at all, when never reset). The run page
// shows the post-play questions only then.
func (t *MHSDeviceTest) LaunchedSinceReset() bool {
	if t.LastResetAt == nil {
		return t.Launched()
	}
	after := func(p *time.Time) bool { return p != nil && p.After(*t.LastResetAt) }
	return after(t.LastLaunchAt) || after(t.GameplayReachedAt) || after(t.UnitCompletedAt) || after(t.LastHeartbeatAt)
}

// Launched reports whether the game was ever launched in this run.
func (t *MHSDeviceTest) Launched() bool {
	switch t.ReachedStage {
	case MHSDeviceTestStageLaunching, MHSDeviceTestStageGameplay, MHSDeviceTestStageCompleted:
		return true
	}
	switch t.Stage {
	case MHSDeviceTestStageLaunching, MHSDeviceTestStageGameplay, MHSDeviceTestStageCompleted:
		return true
	}
	return t.GameplayReachedAt != nil || t.UnitCompletedAt != nil || t.LastHeartbeat != nil
}

// StoppedResponding reports whether the page went silent without ending
// cleanly: it was sending heartbeats, the last one is older than the grace
// period, and neither a completion nor a closing beat arrived. That is what
// a renderer crash ("Aw, Snap") looks like from the server.
func (t *MHSDeviceTest) StoppedResponding(now time.Time) bool {
	if t.LastHeartbeatAt == nil || t.EndedAt != nil || t.UnitCompletedAt != nil {
		return false
	}
	if t.LastHeartbeat == nil || t.LastHeartbeat.Phase != "game" {
		return false // only the game page beats; silence elsewhere is not a crash
	}
	return now.Sub(*t.LastHeartbeatAt) > MHSDeviceTestHeartbeatGrace
}
