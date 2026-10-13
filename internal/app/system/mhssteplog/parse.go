// Package mhssteplog reads the Mission HydroSci page step log (mhs-steplog.js
// entries stored on mhs_device_tests records) as typed events. The pages
// write a human sentence per entry and, from the structured-detail release
// on, a detail map with the same facts as fields; Parse prefers the detail
// and falls back to the sentence, so old records and new ones read the same.
//
// The sentences are our own: internal/app/resources/assets/js/mhs-delivery.js
// (download, method, storage steps) and
// internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml
// (launch and game steps). templates_pin_test.go fails when a sentence this
// package depends on changes there without a matching change here.
package mhssteplog

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

// Kind is what a step-log entry records.
type Kind string

const (
	KindPage           Kind = "page"            // "Opened the <page> page for <unit> v<ver>"
	KindRequesting     Kind = "requesting"      // download/running "Requesting <Title> v<ver> (<MB> MB)"
	KindProgress       Kind = "progress"        // download/running "<Title>: <pct>% · <MB> of <MB> MB · <x> MB/s · m:ss left"
	KindResumed        Kind = "resumed"         // download/info "<Title>: picked up at <MB> MB after a dropped connection (N drops so far)"
	KindNoData         Kind = "no_data"         // download/warn "<Title>: no data received for N s — restarting the download" (a stall)
	KindWaiting        Kind = "waiting"         // download/warn "No data for N s — switching to the direct download in N s" / "restarting in N s"
	KindRetry          Kind = "retry"           // download/warn "<error> Retrying in N s|now (attempt N)"
	KindNote           Kind = "note"            // download/warn free text the page already explained elsewhere
	KindDownloadFail   Kind = "download_fail"   // download/fail "<Title>: <error> [<class>:<reason>] — <raw>"
	KindDownloadOK     Kind = "download_ok"     // download/ok "<Title>: download complete — N MB in m:ss (x MB/s)"
	KindCached         Kind = "cached"          // download/ok "<Title>: already on this device (downloaded earlier)"
	KindBgfetchRefused Kind = "bgfetch_refused" // method/warn "The browser refused the background download (<reason>) — …"
	KindBgfetchFrozen  Kind = "bgfetch_frozen"  // method/warn "Background download stopped receiving data for N s — switched …"
	KindStorage        Kind = "storage"         // storage "Storage: N MB used of N MB (p%) · persisted: yes|no"
	KindLoaderOK       Kind = "loader_ok"       // launch/ok "Game loader loaded (from the downloaded files|from the network)"
	KindLoaderFail     Kind = "loader_fail"     // launch/fail "The game loader failed to load — …"
	KindUnityOK        Kind = "unity_ok"        // launch/ok "Unity started in X s"
	KindUnityFail      Kind = "unity_fail"      // launch/fail "Unity failed to start: <reason>"
	KindWatchdogWarn   Kind = "watchdog_warn"   // launch/warn "No progress for N s while <phase> — <probe>"
	KindWatchdogFail   Kind = "watchdog_fail"   // launch/fail "Still no progress after N s — stuck <phase>…"
	KindRendering      Kind = "rendering"       // game/ok "Game is rendering"
	KindCrash          Kind = "crash"           // game/fail "Crash (<type>, <phase>): <message>"
)

// Event is one parsed entry. Fields not meaningful for the Kind are zero.
type Event struct {
	Kind  Kind
	At    time.Time
	Step  string
	State string

	Unit    string // unit1 … unit6 (or "end"); "" when the entry names no unit
	Title   string // the unit title as written, when the sentence carried one
	Version string

	MB      int // progress: bytes reached; resumed: resume point; download_ok: size; bgfetch_frozen: bytes when it froze
	TotalMB int // progress and requesting: the unit's size
	Pct     int
	Drops   int
	Attempt int

	Seconds float64 // unity_ok: start time; no_data/waiting/bgfetch_frozen: quiet time; watchdog: idle time; download_ok: duration; retry: delay
	MBps    float64

	ErrorClass string // download_fail: quota | network | generic | incomplete | stalled | cdn-unreachable | bgfetch-* | manifest | unit-missing | no-sw
	Reason     string // download_fail: the class's reason; bgfetch_refused: the browser's reason; unity_fail/loader_fail: the error text
	Path       string // download_fail: fallback | background
	Source     string // loader_ok: cache | network | ""
	Phase      string // watchdog: launch phase text; crash: loading | compiling | running | game …
	CrashType  string // crash: load_stalled | load_error | runtime_error | unhandled_rejection
	Message    string // crash: the message; download_fail: the error text before the class
	Notice     bool   // crash: a browser notice (pointer lock, fullscreen, audio), not a failure

	UsedMB    int   // storage
	QuotaMB   int   // storage
	Persisted *bool // storage; nil when unknown
	Fallback  bool  // watchdog_fail: the content-server retry had already been tried
}

var (
	titleRe      = regexp.MustCompile(`(?i)^unit\s*(\d+)(?::\s*([^:]*?))?:\s+(.*)$`)
	unitIDRe     = regexp.MustCompile(`(?i)^unit\s*(\d+)\b`)
	requestingRe = regexp.MustCompile(`^Requesting (.+?) v(\S+) \((\d+) MB\)`)
	progressRe   = regexp.MustCompile(`^(\d+)% · (\d+) of (\d+) MB(?: · ([\d.]+) MB/s)?`)
	resumedRe    = regexp.MustCompile(`^picked up at (\d+) MB after a dropped connection(?: \((\d+) drops so far\))?`)
	noDataRe     = regexp.MustCompile(`^no data received for (\d+) s`)
	waitingRe    = regexp.MustCompile(`^No data for (\d+) s`)
	retryRe      = regexp.MustCompile(`Retrying (?:in (\d+) s|(now)) \(attempt (\d+)\)$`)
	completeRe   = regexp.MustCompile(`^download complete(?: — (\d+) MB)?(?: in (\d+):(\d+)(?: \(([\d.]+) MB/s\))?)?`)
	failRe       = regexp.MustCompile(`^(.*?)(?: \[([A-Za-z0-9_-]+)(?::([^\]]*))?\])?(?: — (.*))?$`)
	refusedRe    = regexp.MustCompile(`^The browser refused the background download \((.*?)\)(?: — |$)`)
	frozenRe     = regexp.MustCompile(`^Background download stopped receiving data for (\d+) s`)
	storageRe    = regexp.MustCompile(`^Storage: (\d+) MB used of (\d+) MB \((\d+)%\)(?: · persisted: (yes|no))?`)
	unityOKRe    = regexp.MustCompile(`^Unity started in ([\d.]+) s`)
	watchWarnRe  = regexp.MustCompile(`^No progress for (\d+) s while (.*?)(?: — (.*))?$`)
	watchFailRe  = regexp.MustCompile(`^Still no progress after (\d+) s — stuck (.*?)(, even straight from the content server)?\.`)
	crashRe      = regexp.MustCompile(`^Crash \(([^,]+), ([^)]+)\): (.*)$`)
	pageRe       = regexp.MustCompile(`^Opened the (\S+) page(?: for (\S+)(?: v(\S+))?)?`)
)

// Sentence prefixes the parser keys on; templates_pin_test.go checks that
// the pages still produce them.
const (
	phraseRequesting     = "Requesting "
	phraseResumed        = ": picked up at "
	phraseDropped        = " MB after a dropped connection"
	phraseDrops          = " drops so far)"
	phraseNoData         = ": no data received for "
	phraseWaiting        = "No data for "
	phraseRetrying       = " Retrying "
	phraseAttempt        = " (attempt "
	phraseComplete       = ": download complete"
	phraseCached         = ": already on this device"
	phraseRefused        = "The browser refused the background download ("
	phraseFrozen         = "Background download stopped receiving data for "
	phraseStorage        = "Storage: "
	phraseUsedOf         = " MB used of "
	phrasePersisted      = " · persisted: "
	phraseLoaderLoaded   = "Game loader loaded"
	phraseFromCache      = "(from the downloaded files)"
	phraseFromNetwork    = "(from the network)"
	phraseUnityStarted   = "Unity started in "
	phraseUnityFailed    = "Unity failed to start: "
	phraseLoaderFailed   = "The game loader failed to load"
	phraseNoProgress     = "No progress for "
	phraseWhile          = " s while "
	phraseStillNo        = "Still no progress after "
	phraseStuck          = " s — stuck "
	phraseEvenStraight   = ", even straight from the content server"
	phraseRendering      = "Game is rendering"
	phraseCrash          = "Crash ("
	phraseOpenedPage     = "Opened the "
	phraseProgressOf     = " of "
	phraseProgressMB     = " MB"
	phraseProgressMBps   = " MB/s"
	phraseProgressPct    = "% · "
	phraseDownloadOKIn   = " in "
	phraseDownloadOKMBps = " MB/s)"
)

// Parse reads one entry. ok is false for entries this package has no use
// for (service-worker, manifest, verify and informational lines).
func Parse(st models.MHSDeviceTestStep) (ev Event, ok bool) {
	ev = Event{At: st.At, Step: st.Step, State: st.State}
	d := st.Detail
	msg := st.Msg
	switch st.Step {
	case "page":
		m := pageRe.FindStringSubmatch(msg)
		if m == nil {
			return ev, false
		}
		ev.Kind = KindPage
		ev.Unit = unitID(pick(d, "unit", m[2]))
		ev.Version = pick(d, "version", m[3])
		return ev, true

	case "download":
		return parseDownload(ev, msg, d)

	case "method":
		if st.State != "warn" {
			return ev, false
		}
		if m := refusedRe.FindStringSubmatch(msg); m != nil || d["kind"] == "bgfetch-refused" {
			ev.Kind = KindBgfetchRefused
			ev.Unit = unitID(d["unit"])
			if m != nil {
				ev.Reason = m[1]
			}
			ev.Reason = pick(d, "reason", ev.Reason)
			return ev, true
		}
		if m := frozenRe.FindStringSubmatch(msg); m != nil || d["kind"] == "bgfetch-frozen" {
			ev.Kind = KindBgfetchFrozen
			ev.Unit = unitID(d["unit"])
			if m != nil {
				ev.Seconds = num(m[1])
			}
			ev.Seconds = numPick(d, "quiet_s", ev.Seconds)
			ev.MB = intPick(d, "mb", 0)
			return ev, true
		}
		return ev, false

	case "storage":
		m := storageRe.FindStringSubmatch(msg)
		if m == nil {
			return ev, false
		}
		ev.Kind = KindStorage
		ev.UsedMB = intPick(d, "used_mb", int(num(m[1])))
		ev.QuotaMB = intPick(d, "quota_mb", int(num(m[2])))
		switch pick(d, "persisted", m[4]) {
		case "yes":
			ev.Persisted = boolPtr(true)
		case "no":
			ev.Persisted = boolPtr(false)
		}
		return ev, true

	case "launch":
		return parseLaunch(ev, msg, d)

	case "game":
		ev.Unit = unitID(d["unit"])
		if st.State == "ok" && strings.HasPrefix(msg, phraseRendering) {
			ev.Kind = KindRendering
			return ev, true
		}
		if st.State == "fail" {
			if m := crashRe.FindStringSubmatch(msg); m != nil {
				ev.Kind = KindCrash
				ev.CrashType = pick(d, "type", m[1])
				ev.Phase = pick(d, "phase", m[2])
				ev.Message = m[3]
				ev.Notice = IsCrashNotice(ev.CrashType, ev.Message)
				return ev, true
			}
		}
		return ev, false
	}
	return ev, false
}

func parseDownload(ev Event, msg string, d map[string]string) (Event, bool) {
	// Split "<Title>: <rest>" when the sentence starts with a unit title.
	rest := msg
	if m := titleRe.FindStringSubmatch(msg); m != nil {
		ev.Unit = "unit" + m[1]
		ev.Title = strings.TrimSpace(strings.TrimSuffix(msg, ": "+m[3]))
		rest = m[3]
	}
	if u := unitID(d["unit"]); u != "" {
		ev.Unit = u
	}
	ev.Version = d["version"]

	switch ev.State {
	case "running":
		if m := requestingRe.FindStringSubmatch(msg); m != nil {
			ev.Kind = KindRequesting
			ev.Title = m[1]
			if ev.Unit == "" {
				ev.Unit = unitID(m[1])
			}
			ev.Version = pick(d, "version", m[2])
			ev.TotalMB = intPick(d, "total_mb", int(num(m[3])))
			return ev, true
		}
		if m := progressRe.FindStringSubmatch(rest); m != nil {
			ev.Kind = KindProgress
			ev.Pct = intPick(d, "pct", int(num(m[1])))
			ev.MB = intPick(d, "mb", int(num(m[2])))
			ev.TotalMB = intPick(d, "total_mb", int(num(m[3])))
			ev.MBps = numPick(d, "mbps", num(m[4]))
			return ev, true
		}
		return ev, false
	case "info":
		if m := resumedRe.FindStringSubmatch(rest); m != nil {
			ev.Kind = KindResumed
			ev.MB = intPick(d, "resumed_mb", int(num(m[1])))
			drops := int(num(m[2]))
			if drops == 0 {
				drops = 1 // the first drop is written without a count
			}
			ev.Drops = intPick(d, "drops", drops)
			return ev, true
		}
		return ev, false
	case "warn":
		if m := noDataRe.FindStringSubmatch(rest); m != nil {
			ev.Kind = KindNoData
			ev.Seconds = numPick(d, "quiet_s", num(m[1]))
			return ev, true
		}
		if m := waitingRe.FindStringSubmatch(msg); m != nil {
			ev.Kind = KindWaiting
			ev.Seconds = numPick(d, "waiting_s", num(m[1]))
			return ev, true
		}
		if m := retryRe.FindStringSubmatch(msg); m != nil {
			ev.Kind = KindRetry
			ev.Seconds = numPick(d, "in_s", num(m[1]))
			ev.Attempt = intPick(d, "attempt", int(num(m[3])))
			ev.ErrorClass = d["error_class"]
			ev.Path = d["path"]
			ev.Message = strings.TrimSpace(msg[:strings.LastIndex(msg, phraseRetrying)])
			return ev, true
		}
		if d["note"] != "" {
			ev.Kind = KindNote
			return ev, true
		}
		return ev, false
	case "fail":
		ev.Kind = KindDownloadFail
		if m := failRe.FindStringSubmatch(rest); m != nil {
			ev.Message = m[1]
			ev.ErrorClass = m[2]
			ev.Reason = m[3]
		} else {
			ev.Message = rest
		}
		ev.ErrorClass = pick(d, "error_class", ev.ErrorClass)
		ev.Reason = pick(d, "reason", ev.Reason)
		ev.Path = d["path"]
		return ev, true
	case "ok":
		if d["cached"] == "1" || strings.HasPrefix(rest, strings.TrimPrefix(phraseCached, ": ")) {
			ev.Kind = KindCached
			return ev, true
		}
		if m := completeRe.FindStringSubmatch(rest); m != nil {
			ev.Kind = KindDownloadOK
			ev.MB = intPick(d, "mb", int(num(m[1])))
			if m[2] != "" {
				ev.Seconds = num(m[2])*60 + num(m[3])
			}
			ev.Seconds = numPick(d, "seconds", ev.Seconds)
			ev.MBps = numPick(d, "mbps", num(m[4]))
			return ev, true
		}
		return ev, false
	}
	return ev, false
}

func parseLaunch(ev Event, msg string, d map[string]string) (Event, bool) {
	ev.Unit = unitID(d["unit"])
	switch ev.State {
	case "ok":
		if strings.HasPrefix(msg, phraseLoaderLoaded) {
			ev.Kind = KindLoaderOK
			switch {
			case d["source"] != "":
				ev.Source = d["source"]
			case strings.Contains(msg, phraseFromCache):
				ev.Source = "cache"
			case strings.Contains(msg, phraseFromNetwork):
				ev.Source = "network"
			}
			return ev, true
		}
		if m := unityOKRe.FindStringSubmatch(msg); m != nil {
			ev.Kind = KindUnityOK
			ev.Seconds = numPick(d, "unity_s", num(m[1]))
			return ev, true
		}
	case "warn":
		if m := watchWarnRe.FindStringSubmatch(msg); m != nil {
			ev.Kind = KindWatchdogWarn
			ev.Seconds = numPick(d, "idle_s", num(m[1]))
			ev.Phase = pick(d, "phase", m[2])
			ev.Message = m[3]
			return ev, true
		}
	case "fail":
		if m := watchFailRe.FindStringSubmatch(msg); m != nil {
			ev.Kind = KindWatchdogFail
			ev.Seconds = numPick(d, "idle_s", num(m[1]))
			ev.Phase = pick(d, "phase", m[2])
			ev.Fallback = m[3] != "" || d["fallback"] == "1"
			return ev, true
		}
		if strings.HasPrefix(msg, phraseUnityFailed) {
			ev.Kind = KindUnityFail
			ev.Reason = pick(d, "reason", strings.TrimPrefix(msg, phraseUnityFailed))
			return ev, true
		}
		if strings.HasPrefix(msg, phraseLoaderFailed) {
			ev.Kind = KindLoaderFail
			ev.Reason = pick(d, "reason", "loader-script-error")
			return ev, true
		}
	}
	return ev, false
}

// unitID normalizes "Unit 1", "unit1", "Unit 1: Water Cycle" and "end" to
// the unit id the records use ("unit1", "end"); "" when there is none.
func unitID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := unitIDRe.FindStringSubmatch(s); m != nil {
		return "unit" + m[1]
	}
	if strings.EqualFold(s, "end") {
		return "end"
	}
	return ""
}

func pick(d map[string]string, key, fallback string) string {
	if v, ok := d[key]; ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func num(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func numPick(d map[string]string, key string, fallback float64) float64 {
	if v, ok := d[key]; ok && strings.TrimSpace(v) != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return fallback
}

func intPick(d map[string]string, key string, fallback int) int {
	if v, ok := d[key]; ok && strings.TrimSpace(v) != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return int(f)
		}
	}
	return fallback
}

func boolPtr(b bool) *bool { return &b }
