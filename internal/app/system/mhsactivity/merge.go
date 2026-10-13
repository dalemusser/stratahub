package mhsactivity

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/app/store/logdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// stallRecoverWindow is how soon after a watchdog warning a rendering on the
// same device makes the stall a recovered one.
const stallRecoverWindow = 10 * time.Minute

// minPlayedHeartbeats is how many heartbeats a launch needs to count as
// played when the game log cannot be read.
const minPlayedHeartbeats = 4

// collapseWindow: the page collapses consecutive identical entries and moves
// their time, so the same warning or failure can arrive from two records
// with two times. Within this window it is one event.
const collapseWindow = 90 * time.Second

func optionsProjection(p interface{}) *options.FindOptions {
	return options.Find().SetProjection(p)
}

// builder accumulates one zone-day.
type builder struct {
	wsID         primitive.ObjectID
	day          string
	zone         Zone
	start, end   time.Time
	people       map[primitive.ObjectID]*person
	accs         map[primitive.ObjectID]*dayAcc
	missingList  []string
	logAvailable bool
}

type dayAcc struct {
	doc          models.MHSActivityDay
	units        map[string]*unitAcc // device|unit
	devices      map[string]*models.MHSActivityDevice
	events       map[string]time.Time   // dedupe key → time, for launch and crash events
	warns        map[string][]time.Time // device → watchdog warnings
	renders      map[string][]time.Time // device → renderings
	types        map[string]bool        // real crash types
	launchPlayed bool                   // a launch record says logs were seen or the game heartbeat
}

type unitAcc struct {
	models.MHSActivityUnit
	lastErrAt time.Time
}

func newBuilder(wsID primitive.ObjectID, day string, zone Zone, start, end time.Time, people map[primitive.ObjectID]*person) *builder {
	return &builder{wsID: wsID, day: day, zone: zone, start: start, end: end, people: people, accs: map[primitive.ObjectID]*dayAcc{}}
}

func (b *builder) missing(name string) {
	for _, m := range b.missingList {
		if m == name {
			return
		}
	}
	b.missingList = append(b.missingList, name)
}

func (b *builder) inDay(t time.Time) bool {
	return !t.Before(b.start) && t.Before(b.end)
}

func (b *builder) acc(p *person) *dayAcc {
	a := b.accs[p.ID]
	if a == nil {
		a = &dayAcc{
			units:   map[string]*unitAcc{},
			devices: map[string]*models.MHSActivityDevice{},
			events:  map[string]time.Time{},
			warns:   map[string][]time.Time{},
			renders: map[string][]time.Time{},
			types:   map[string]bool{},
		}
		a.doc = models.MHSActivityDay{
			WorkspaceID:    b.wsID,
			UserID:         p.ID,
			OrganizationID: p.Org,
			GroupIDs:       p.Groups,
			Role:           p.Role,
			Day:            b.day,
			TZ:             b.zone.TZ,
			TZFallback:     b.zone.Fallback,
		}
		b.accs[p.ID] = a
	}
	return a
}

func (a *dayAcc) touch(t time.Time) {
	if t.IsZero() {
		return
	}
	if a.doc.FirstAt == nil || t.Before(*a.doc.FirstAt) {
		tt := t
		a.doc.FirstAt = &tt
	}
	if a.doc.LastAt == nil || t.After(*a.doc.LastAt) {
		tt := t
		a.doc.LastAt = &tt
	}
}

func (a *dayAcc) device(id string) *models.MHSActivityDevice {
	d := a.devices[id]
	if d == nil {
		d = &models.MHSActivityDevice{DeviceID: id}
		a.devices[id] = d
	}
	return d
}

// seen records an event once per (device, kind, time); for warn/fail kinds a
// second time within collapseWindow is the same event moved by the page.
func (a *dayAcc) seen(dev, kind string, at time.Time, collapse bool) bool {
	key := dev + "|" + kind + "|" + at.UTC().Format(time.RFC3339)
	if _, ok := a.events[key]; ok {
		return true
	}
	if collapse {
		prefix := dev + "|" + kind + "|"
		for k, t := range a.events {
			if strings.HasPrefix(k, prefix) && absDur(at.Sub(t)) <= collapseWindow {
				return true
			}
		}
	}
	a.events[key] = at
	return false
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// addRecord folds one load record and its summary into the student's day.
func (b *builder) addRecord(rec models.MHSDeviceTest, sum *models.MHSMemberSummary) {
	if rec.UserID == nil {
		return
	}
	p := b.people[*rec.UserID]
	if p == nil {
		return
	}
	a := b.acc(p)
	loc := b.zone.Loc
	inDay := b.inDay(rec.StartedAt)
	if inDay {
		a.doc.Records++
		a.doc.Opened = true
		a.touch(rec.StartedAt)
	}
	dev := rec.DeviceID
	if dev != "" {
		d := a.device(dev)
		if d.DeviceType == "" {
			d.DeviceType = rec.DeviceType
		}
		if d.Platform == "" && rec.UserAgent != "" {
			d.Platform = PlatformOf(rec.UserAgent)
			d.Browser = BrowserOf(rec.UserAgent)
		}
		if inDay {
			d.Records++
			if d.LastAt == nil || rec.StartedAt.After(*d.LastAt) {
				t := rec.StartedAt
				d.LastAt = &t
			}
		}
	}
	if inDay && outcomeOf(rec) == "launch-ok" && (rec.LogsState == models.MHSLogsStateSeen || rec.HeartbeatCount >= minPlayedHeartbeats) {
		a.launchPlayed = true
	}
	if inDay && outcomeOf(rec) == "launch-ok" && rec.LoggingProblem() {
		a.doc.Flags.LoggingProblem = true
	}
	if sum == nil {
		return
	}
	for _, u := range sum.Units {
		at := u.LastAt
		if at.IsZero() {
			at = rec.StartedAt
		}
		if LocalDay(at, loc) != b.day {
			continue
		}
		a.mergeUnit(dev, u)
		a.doc.Opened = true // a download that day is an open units page that day
		a.touch(u.FirstAt)
		a.touch(u.LastAt)
	}
	for _, l := range sum.Launch {
		if l.At.IsZero() || LocalDay(l.At, loc) != b.day {
			continue
		}
		warnOrFail := l.Kind == models.MHSLaunchEventWatchdogWarn || l.Kind == models.MHSLaunchEventWatchdogFail || l.Kind == models.MHSLaunchEventUnityFail || l.Kind == models.MHSLaunchEventLoaderFail
		if a.seen(dev, l.Kind, l.At, warnOrFail) {
			continue
		}
		a.doc.Opened = true
		a.touch(l.At)
		switch l.Kind {
		case models.MHSLaunchEventLoaderOK:
			if l.Source == "cache" {
				a.doc.Launch.LoaderCache++
			} else if l.Source == "network" {
				a.doc.Launch.LoaderNetwork++
			}
		case models.MHSLaunchEventUnityOK:
			a.doc.Launch.UnitySecondsLast = l.Seconds
			if l.Seconds > a.doc.Launch.UnitySecondsMax {
				a.doc.Launch.UnitySecondsMax = l.Seconds
			}
		case models.MHSLaunchEventWatchdogWarn:
			a.doc.Launch.Stalled++
			a.warns[dev] = append(a.warns[dev], l.At)
		case models.MHSLaunchEventWatchdogFail, models.MHSLaunchEventUnityFail, models.MHSLaunchEventLoaderFail:
			a.doc.Launch.Failed++
		case models.MHSLaunchEventRendering:
			a.doc.Launch.OK++
			a.renders[dev] = append(a.renders[dev], l.At)
		}
	}
	for _, c := range sum.Crashes {
		if c.At.IsZero() || LocalDay(c.At, loc) != b.day {
			continue
		}
		if a.seen(dev, "crash:"+c.Type, c.At, true) {
			continue
		}
		a.touch(c.At)
		if c.Notice {
			a.doc.Crashes.Notice++
			continue
		}
		a.doc.Crashes.Real++
		if c.Type != "" {
			a.types[c.Type] = true
		}
		if a.doc.Crashes.Last == nil || c.At.After(*a.doc.Crashes.Last) {
			t := c.At
			a.doc.Crashes.Last = &t
		}
	}
	if sum.Storage != nil && sum.Storage.Persisted != nil && !*sum.Storage.Persisted && b.inDay(sum.Storage.At) {
		a.doc.Flags.StorageNotPersisted = true
	}
}

// mergeUnit folds one record's reading of a unit download into the episode
// for that unit on that device. Counters take the maximum (a later record
// from the same tab repeats and extends an earlier one); states combine.
func (a *dayAcc) mergeUnit(dev string, u models.MHSStepUnitSummary) {
	key := dev + "|" + u.Unit
	e := a.units[key]
	if e == nil {
		e = &unitAcc{}
		e.Unit = u.Unit
		e.DeviceID = dev
		e.FirstAt = u.FirstAt
		e.LastAt = u.LastAt
		a.units[key] = e
	}
	e.Records++
	if u.Version != "" {
		e.Version = u.Version
	}
	e.Requested = e.Requested || u.Requested
	e.Cached = e.Cached || u.Cached
	e.TotalMB = maxInt(e.TotalMB, u.TotalMB)
	e.MaxMB = maxInt(e.MaxMB, u.MaxMB)
	e.Drops = maxInt(e.Drops, u.Drops)
	e.Stalls = maxInt(e.Stalls, u.Stalls)
	e.Retries = maxInt(e.Retries, u.Retries)
	e.Fails = maxInt(e.Fails, u.Fails)
	if u.LastErrorClass != "" || u.LastErrorText != "" {
		if e.lastErrAt.IsZero() || !u.LastAt.Before(e.lastErrAt) {
			e.LastErrorClass = u.LastErrorClass
			e.LastErrorText = u.LastErrorText
			e.LastPath = u.LastPath
			e.lastErrAt = u.LastAt
		}
	}
	if u.Complete && u.CompleteAt != nil {
		if !e.Complete || e.CompleteAt == nil || u.CompleteAt.Before(*e.CompleteAt) {
			e.Complete = true
			t := *u.CompleteAt
			e.CompleteAt = &t
			e.CompleteMB = u.CompleteMB
			e.CompleteSeconds = u.CompleteSeconds
			e.CompleteMBps = u.CompleteMBps
		}
	}
	if u.Held {
		if !e.Held || (u.HeldAt != nil && (e.HeldAt == nil || u.HeldAt.Before(*e.HeldAt))) {
			e.Held = true
			e.HeldAtMB = u.HeldAtMB
			if u.HeldAt != nil {
				t := *u.HeldAt
				e.HeldAt = &t
			}
		}
	}
	e.BgfetchRefused = e.BgfetchRefused || u.BgfetchRefused
	e.BgfetchFrozen = e.BgfetchFrozen || u.BgfetchFrozen
	if !u.FirstAt.IsZero() && (e.FirstAt.IsZero() || u.FirstAt.Before(e.FirstAt)) {
		e.FirstAt = u.FirstAt
	}
	if u.LastAt.After(e.LastAt) {
		e.LastAt = u.LastAt
	}
}

func (e *unitAcc) finish() models.MHSActivityUnit {
	u := e.MHSActivityUnit
	if u.Held && u.Complete && u.HeldAt != nil && u.CompleteAt != nil && u.CompleteAt.After(*u.HeldAt) {
		u.HoldSeconds = int(u.CompleteAt.Sub(*u.HeldAt) / time.Second)
	}
	switch {
	case u.Held && u.Complete:
		u.State = models.MHSActivityUnitHeldDone
	case u.Complete:
		u.State = models.MHSActivityUnitComplete
	case u.Held:
		u.State = models.MHSActivityUnitHeld
	case u.Fails > 0:
		u.State = models.MHSActivityUnitFailed
	case u.Cached:
		u.State = models.MHSActivityUnitCached
	default:
		u.State = models.MHSActivityUnitInProgress
	}
	return u
}

// addDevice folds a device-status row in: created in the day = a fresh
// browser profile; seen in the day = the units page was opened.
func (b *builder) addDevice(d models.MHSDeviceStatus) {
	p := b.people[d.UserID]
	if p == nil {
		return
	}
	created := b.inDay(d.CreatedAt)
	seen := b.inDay(d.LastSeen)
	a := b.accs[p.ID]
	if a == nil {
		if !created && !seen {
			return
		}
		a = b.acc(p)
	}
	dev, exists := a.devices[d.DeviceID]
	if !exists && !created && !seen {
		return
	}
	if !exists {
		dev = a.device(d.DeviceID)
	}
	if d.DeviceType != "" {
		dev.DeviceType = d.DeviceType
	}
	if ua := d.DeviceDetails["user_agent"]; ua != "" {
		dev.Platform = PlatformOf(ua)
		dev.Browser = BrowserOf(ua)
	}
	if created {
		dev.New = true
		t := d.CreatedAt
		dev.CreatedAt = &t
		a.doc.Flags.NewDevice = true
	}
	if seen {
		a.doc.Opened = true
		a.touch(d.LastSeen)
		if dev.LastAt == nil || d.LastSeen.After(*dev.LastAt) {
			t := d.LastSeen
			dev.LastAt = &t
		}
	}
	if d.StoragePersisted != nil {
		v := *d.StoragePersisted
		dev.Persisted = &v
		if !v {
			a.doc.Flags.StorageNotPersisted = true
		}
	}
	if d.StorageQuota > 0 {
		dev.StorageQuotaMB = int(d.StorageQuota / 1048576)
	}
}

func (b *builder) addPageViews(userID primitive.ObjectID, v activity.PageViewSummary) {
	p := b.people[userID]
	if p == nil || v.Count == 0 {
		return
	}
	a := b.acc(p)
	a.doc.Opened = true
	a.doc.PageViews += v.Count
	a.touch(v.First)
	a.touch(v.Last)
}

func (b *builder) addLog(userID primitive.ObjectID, ua logdata.UserActivity) {
	p := b.people[userID]
	if p == nil || ua.Events == 0 {
		return
	}
	a := b.acc(p)
	a.doc.Opened = true
	a.doc.Played = true
	first, last := ua.First, ua.Last
	a.doc.Log = models.MHSActivityLog{Available: true, Source: "logdata", Events: ua.Events, First: &first, Last: &last, CrashEvents: ua.Crashes}
	a.touch(first)
	a.touch(last)
}

// finish closes every accumulator into its day document.
func (b *builder) finish(now time.Time) []models.MHSActivityDay {
	docs := make([]models.MHSActivityDay, 0, len(b.accs))
	for _, a := range b.accs {
		d := &a.doc
		d.Log.Available = b.logAvailable
		if !b.logAvailable && a.launchPlayed {
			d.Played = true
			d.Log.Source = "launch"
		}
		keys := make([]string, 0, len(a.units))
		for k := range a.units {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		problem := false
		for _, k := range keys {
			u := a.units[k].finish()
			d.Units = append(d.Units, u)
			switch u.State {
			case models.MHSActivityUnitFailed:
				d.Flags.DownloadFailed = true
				problem = true
			case models.MHSActivityUnitHeld:
				d.Flags.Held = true
				problem = true
			case models.MHSActivityUnitHeldDone:
				d.Flags.Held = true
			}
			if u.Fails > 0 {
				d.Flags.DownloadFailed = true
			}
			if u.BgfetchRefused {
				d.Flags.BgfetchRefused = true
			}
		}
		devKeys := make([]string, 0, len(a.devices))
		for k := range a.devices {
			devKeys = append(devKeys, k)
		}
		sort.Strings(devKeys)
		for _, k := range devKeys {
			d.Devices = append(d.Devices, *a.devices[k])
		}
		for dev, warns := range a.warns {
			for _, w := range warns {
				for _, r := range a.renders[dev] {
					if !r.Before(w) && r.Sub(w) <= stallRecoverWindow {
						d.Launch.StalledRecovered++
						break
					}
				}
			}
		}
		if d.Launch.Failed > 0 {
			d.Flags.LaunchFailed = true
			problem = true
		}
		for t := range a.types {
			d.Crashes.Types = append(d.Crashes.Types, t)
		}
		sort.Strings(d.Crashes.Types)
		d.Launched = d.Launch.OK > 0
		if d.Launched || d.Played {
			d.Opened = true
		}
		d.Blocked = d.Opened && !d.Played && problem
		d.Final = false
		d.ComputedAt = now
		d.Version = models.MHSActivityVersion
		docs = append(docs, *d)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].UserID.Hex() < docs[j].UserID.Hex() })
	return docs
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	chromeRe  = regexp.MustCompile(`(?:Chrome|CriOS)/(\d+)`)
	edgeRe    = regexp.MustCompile(`Edg/(\d+)`)
	firefoxRe = regexp.MustCompile(`Firefox/(\d+)`)
	safariRe  = regexp.MustCompile(`Version/(\d+)[.\d]* .*Safari`)
)

// PlatformOf names the operating system from a user agent.
func PlatformOf(ua string) string {
	switch {
	case ua == "":
		return ""
	case strings.Contains(ua, "CrOS"):
		return "ChromeOS"
	case strings.Contains(ua, "Windows"):
		return "Windows"
	case strings.Contains(ua, "iPad"):
		return "iPadOS"
	case strings.Contains(ua, "iPhone"):
		return "iOS"
	case strings.Contains(ua, "Macintosh"):
		return "macOS"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	}
	return "Other"
}

// BrowserOf names the browser and major version from a user agent.
func BrowserOf(ua string) string {
	if m := edgeRe.FindStringSubmatch(ua); m != nil {
		return "Edge " + m[1]
	}
	if m := chromeRe.FindStringSubmatch(ua); m != nil {
		return "Chrome " + m[1]
	}
	if m := firefoxRe.FindStringSubmatch(ua); m != nil {
		return "Firefox " + m[1]
	}
	if m := safariRe.FindStringSubmatch(ua); m != nil {
		return "Safari " + m[1]
	}
	return ""
}
