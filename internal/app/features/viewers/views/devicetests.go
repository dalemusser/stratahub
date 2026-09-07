package views

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	"github.com/dalemusser/stratahub/internal/app/store/logdata"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicetests"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// DeviceTestsSlug is the viewer's URL segment: /views/device-tests.
const DeviceTestsSlug = "device-tests"

const (
	deviceTestsLiveSeconds = 15
	deviceTestsTimeLayout  = "Jan 2, 2006 3:04 PM"
	deviceTestsExportMax   = 5000
)

// DeviceTests lists runs of the Mission HydroSci device test (and, after
// plan step A2, members' stored load records): who ran it, on what, how the
// download and launch went, and where it stopped — with the full step log,
// diagnostics and the run's game telemetry in the detail.
type DeviceTests struct {
	db       *mongo.Database
	store    *mhsdevicetests.Store
	logs     *logdata.Store  // nil when stratalog is not configured
	gradesDB *mongo.Database // nil when mhsgrader is not configured
}

// NewDeviceTests constructs the viewer. logDB and gradesDB may be nil; the
// gameplay section of the detail then says so.
func NewDeviceTests(db, logDB, gradesDB *mongo.Database) *DeviceTests {
	v := &DeviceTests{db: db, store: mhsdevicetests.New(db), gradesDB: gradesDB}
	if logDB != nil {
		v.logs = logdata.New(logDB)
	}
	return v
}

func (v *DeviceTests) Slug() string  { return DeviceTestsSlug }
func (v *DeviceTests) Title() string { return "Device Tests" }
func (v *DeviceTests) Description() string {
	return "Runs of the Mission HydroSci device test: who ran it, on what device and network, how the download and launch went, where it stopped, and the run's game telemetry."
}
func (v *DeviceTests) Roles() []string          { return []string{"admin", "analyst"} }
func (v *DeviceTests) LiveIntervalSeconds() int { return deviceTestsLiveSeconds }

// Filter keys (prefixed: the package shares one namespace across viewers).
const (
	dtWhen   = "when"
	dtStage  = "stage"
	dtKind   = "kind"
	dtDevice = "device"
	dtSchool = "school"
	dtID     = "id"
)

var deviceTypeOptions = []string{"Chromebook", "iPad", "macOS", "Windows", "Android", "Linux", "Other"}

func (v *DeviceTests) Filters() []viewers.FilterSpec {
	devices := []viewers.Option{}
	for _, d := range deviceTypeOptions {
		devices = append(devices, viewers.Option{Value: d, Label: d})
	}
	return []viewers.FilterSpec{
		{Key: dtWhen, Label: "Started", Type: viewers.FilterDateRange, Default: viewers.RangeMonth},
		{Key: dtStage, Label: "Stage", Type: viewers.FilterSelect, Options: []viewers.Option{
			{Value: models.MHSDeviceTestStageCompleted, Label: "Completed Unit 2"},
			{Value: models.MHSDeviceTestStageGameplay, Label: "Reached gameplay"},
			{Value: models.MHSDeviceTestStageLaunching, Label: "Launching"},
			{Value: models.MHSDeviceTestStageDownloaded, Label: "Downloaded"},
			{Value: models.MHSDeviceTestStageDownloading, Label: "Downloading"},
			{Value: models.MHSDeviceTestStageRun, Label: "Started only"},
			{Value: models.MHSDeviceTestStageFailed, Label: "Failed (latest step)"},
		}},
		{Key: dtKind, Label: "Kind", Type: viewers.FilterSelect, Default: models.MHSDeviceTestKindDeviceTest, Options: []viewers.Option{
			{Value: models.MHSDeviceTestKindDeviceTest, Label: "Device test"},
			{Value: models.MHSDeviceTestKindMember, Label: "Member load record"},
		}},
		{Key: dtDevice, Label: "Device", Type: viewers.FilterSelect, Options: devices},
		{Key: dtSchool, Label: "School", Type: viewers.FilterText, Placeholder: "school or district"},
		{Key: dtID, Label: "Test id", Type: viewers.FilterID, Placeholder: "24-char test id"},
	}
}

func (v *DeviceTests) Columns() []viewers.ColumnSpec {
	return []viewers.ColumnSpec{
		{Key: "started", Label: "Started", Class: "whitespace-nowrap"},
		{Key: "school", Label: "School"},
		{Key: "tester", Label: "Tester"},
		{Key: "device", Label: "Device"},
		{Key: "network", Label: "Network"},
		{Key: "path", Label: "Path"},
		{Key: "download", Label: "Download", Class: "whitespace-nowrap"},
		{Key: "stage", Label: "Stage", Class: "whitespace-nowrap"},
		{Key: "failed", Label: "Last problem"},
		{Key: "duration", Label: "Duration", Class: "whitespace-nowrap"},
	}
}

func (v *DeviceTests) listQuery(ctx context.Context, scope *viewscope.Scope, f viewers.Filters) (*mhsdevicetests.ListQuery, error) {
	sf, err := scope.Filter(ctx, "organization_id", "user_id")
	if err != nil {
		return nil, err
	}
	q := &mhsdevicetests.ListQuery{
		WorkspaceID: scope.WorkspaceID,
		Scope:       sf,
		Stage:       f.Get(dtStage),
		Kind:        f.Get(dtKind),
		DeviceType:  f.Get(dtDevice),
		School:      strings.TrimSpace(f.Get(dtSchool)),
	}
	if from, to, ok := f.DateRange(dtWhen, time.Now().UTC()); ok {
		q.From, q.To = from, to
	}
	if id, ok := f.ID(dtID); ok {
		q.ID = &id
	} else if f.Has(dtID) {
		return nil, nil
	}
	return q, nil
}

func (v *DeviceTests) Query(ctx context.Context, scope *viewscope.Scope, f viewers.Filters, after *viewers.Cursor, limit int) (viewers.Page, error) {
	q, err := v.listQuery(ctx, scope, f)
	if err != nil || q == nil {
		return viewers.Page{}, err
	}
	if after != nil {
		at, err := viewers.ParseTimeKey(after.Key)
		if err != nil {
			return viewers.Page{}, err
		}
		q.After = &mhsdevicetests.Position{StartedAt: at, ID: after.ID}
	}
	q.Limit = limit

	runs, more, err := v.store.List(ctx, *q)
	if err != nil {
		return viewers.Page{}, err
	}
	page := viewers.Page{}
	for _, t := range runs {
		page.Rows = append(page.Rows, v.row(t))
	}
	if more && len(runs) > 0 {
		last := runs[len(runs)-1]
		page.Next = &viewers.Cursor{Key: viewers.TimeKey(last.StartedAt), ID: last.ID}
	}
	return page, nil
}

func stagePill(stage string) string {
	switch stage {
	case models.MHSDeviceTestStageCompleted:
		return viewers.PillGreen
	case models.MHSDeviceTestStageGameplay:
		return viewers.PillIndigo
	case models.MHSDeviceTestStageDownloaded, models.MHSDeviceTestStageLaunching:
		return viewers.PillBlue
	case models.MHSDeviceTestStageFailed:
		return viewers.PillRed
	}
	return viewers.PillGray
}

func stageLabel(stage string) string {
	switch stage {
	case models.MHSDeviceTestStageCompleted:
		return "Completed"
	case models.MHSDeviceTestStageGameplay:
		return "Gameplay"
	case models.MHSDeviceTestStageLaunching:
		return "Launching"
	case models.MHSDeviceTestStageDownloaded:
		return "Downloaded"
	case models.MHSDeviceTestStageDownloading:
		return "Downloading"
	case models.MHSDeviceTestStageFailed:
		return "Failed"
	case models.MHSDeviceTestStageRun, models.MHSDeviceTestStageForm:
		return "Started"
	}
	return stage
}

func mmss(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func mb(bytes int64) string { return fmt.Sprintf("%d MB", bytes/1048576) }

func joinNonEmpty(sep string, parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func (v *DeviceTests) row(t models.MHSDeviceTest) viewers.Row {
	started := viewers.Cell{
		Text:  t.StartedAt.UTC().Format(deviceTestsTimeLayout) + " UTC",
		Title: t.StartedAt.UTC().Format(time.RFC3339) + " · test " + t.ID.Hex(),
	}
	school := viewers.Cell{Text: t.Form.School}
	tester := viewers.Cell{Text: joinNonEmpty(" · ", t.Form.TesterName, t.Form.TesterRole)}
	if t.Kind == models.MHSDeviceTestKindMember {
		school = viewers.Cell{Text: "Member", Class: viewers.PillGray, Title: "A member's stored load record"}
		if t.UserID != nil {
			tester = viewers.Cell{Text: t.UserID.Hex(), Title: "Member id"}
		}
	}
	device := viewers.Cell{Text: joinNonEmpty(" · ", t.DeviceType, t.Platform, t.Browser)}
	if device.Text == "" {
		device.Text = "—"
	}
	network := viewers.Cell{Text: t.Network}
	if network.Text == "" {
		network.Text = "—"
	}
	path := viewers.Cell{Text: "—"}
	switch t.Download.Path {
	case "background":
		path = viewers.Cell{Text: "Background", Class: viewers.PillBlue, Title: "Background Fetch (Chrome)"}
	case "direct":
		path = viewers.Cell{Text: "Direct", Class: viewers.PillAmber, Title: "Direct download inside the service worker (tab must stay open)"}
	}
	if t.Download.Switched {
		path.Title += " · switched from background after it stopped receiving data"
		path.Text += " ↻"
	}
	download := viewers.Cell{Text: "—"}
	if t.Download.Seconds > 0 && t.Download.Bytes > 0 {
		download = viewers.Cell{
			Text:  fmt.Sprintf("%s in %s", mb(t.Download.Bytes), mmss(time.Duration(t.Download.Seconds*float64(time.Second)))),
			Title: fmt.Sprintf("%.1f MB/s · %d stall(s), %d retry(ies)", t.Download.AvgBps/1048576, t.Download.Stalls, t.Download.Retries),
		}
	} else if t.Download.Stalls > 0 || t.Download.Retries > 0 {
		download = viewers.Cell{Text: fmt.Sprintf("%d stall(s), %d retry(ies)", t.Download.Stalls, t.Download.Retries), Class: viewers.TextMuted}
	}
	stage := viewers.Cell{Text: stageLabel(t.Stage), Class: stagePill(t.Stage)}
	if t.UnitCompletedAt != nil {
		stage.Title = "Unit completed " + t.UnitCompletedAt.UTC().Format(time.RFC3339)
	} else if t.GameplayReachedAt != nil {
		stage.Title = "Gameplay reached " + t.GameplayReachedAt.UTC().Format(time.RFC3339)
	}
	failed := viewers.Cell{Text: ""}
	if t.FailedStep != "" {
		reason := t.FailedReason
		if len(reason) > 90 {
			reason = reason[:87] + "…"
		}
		failed = viewers.Cell{Text: stepLabel(t.FailedStep) + ": " + reason, Title: t.FailedReason, Class: viewers.TextRed}
	}
	end := t.LastSeenAt
	if t.EndedAt != nil {
		end = *t.EndedAt
	}
	duration := viewers.Cell{Text: mmss(end.Sub(t.StartedAt)), Title: "From start to the last activity seen"}

	return viewers.Row{ID: t.ID.Hex(), Cells: []viewers.Cell{started, school, tester, device, network, path, download, stage, failed, duration}}
}

// stepLabel mirrors mhs-steplog.js's step names.
func stepLabel(step string) string {
	switch step {
	case "sw":
		return "Service worker"
	case "manifest":
		return "Game list"
	case "storage":
		return "Storage"
	case "cdn":
		return "Content server"
	case "services":
		return "Game services"
	case "method":
		return "Download method"
	case "download":
		return "Download"
	case "verify":
		return "Verify files"
	case "launch":
		return "Launch"
	case "game":
		return "Game"
	case "page":
		return "Page"
	}
	return step
}

// gameplay is what the run's game id has in stratalog and mhsgrader.
type gameplay struct {
	Available bool       `json:"available"`
	Note      string     `json:"note,omitempty"`
	Events    int64      `json:"events"`
	First     *time.Time `json:"first_event,omitempty"`
	Last      *time.Time `json:"last_event,omitempty"`
	Scenes    []string   `json:"scenes,omitempty"`
	Points    []string   `json:"progress_points,omitempty"` // "ruleId: status" for the latest attempt
	Current   string     `json:"current_unit,omitempty"`
}

type gradeDoc struct {
	Grades      map[string][]gradeItem `bson:"grades"`
	CurrentUnit string                 `bson:"currentUnit,omitempty"`
}
type gradeItem struct {
	Attempt int    `bson:"attempt"`
	Status  string `bson:"status"`
	RuleID  string `bson:"ruleId"`
}

func (v *DeviceTests) loadGameplay(ctx context.Context, idHex string) gameplay {
	g := gameplay{}
	if v.logs == nil {
		g.Note = "Game log service not configured"
		return g
	}
	entries, err := v.logs.ListForUser(ctx, "mhs", idHex)
	if err != nil {
		g.Note = "Game log lookup failed: " + err.Error()
		return g
	}
	g.Available = true
	g.Events = int64(len(entries))
	scenes := map[string]bool{}
	for i, e := range entries {
		if i == 0 {
			t := e.ServerTimestamp.UTC()
			g.First = &t
		}
		if i == len(entries)-1 {
			t := e.ServerTimestamp.UTC()
			g.Last = &t
		}
		if e.SceneName != "" {
			scenes[e.SceneName] = true
		}
	}
	for s := range scenes {
		g.Scenes = append(g.Scenes, s)
	}
	sort.Strings(g.Scenes)
	if v.gradesDB != nil {
		var doc gradeDoc
		err := v.gradesDB.Collection("progress_point_grades").FindOne(ctx, bson.M{"game": "mhs", "user_id": idHex}).Decode(&doc)
		if err == nil {
			g.Current = doc.CurrentUnit
			keys := make([]string, 0, len(doc.Grades))
			for k := range doc.Grades {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				items := doc.Grades[k]
				if len(items) == 0 {
					continue
				}
				last := items[len(items)-1]
				g.Points = append(g.Points, fmt.Sprintf("%s: %s (attempt %d)", k, last.Status, last.Attempt))
			}
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			g.Note = "Grade lookup failed: " + err.Error()
		}
	}
	return g
}

func esc(s string) string { return template.HTMLEscapeString(s) }

func (v *DeviceTests) load(ctx context.Context, scope *viewscope.Scope, id string) (*models.MHSDeviceTest, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, viewers.ErrNotFound
	}
	// Re-apply scope: a restricted scope never matches a device-test run.
	sf, err := scope.Filter(ctx, "organization_id", "user_id")
	if err != nil {
		return nil, err
	}
	if sf != nil {
		runs, _, err := v.store.List(ctx, mhsdevicetests.ListQuery{WorkspaceID: scope.WorkspaceID, Scope: sf, ID: &oid, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(runs) == 0 {
			return nil, viewers.ErrNotFound
		}
	}
	t, err := v.store.Get(ctx, scope.WorkspaceID, oid)
	if errors.Is(err, mhsdevicetests.ErrNotFound) {
		return nil, viewers.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (v *DeviceTests) Detail(ctx context.Context, scope *viewscope.Scope, id string) (*viewers.Detail, error) {
	t, err := v.load(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	add := func(fields []viewers.Field, label, value string, mono bool) []viewers.Field {
		if strings.TrimSpace(value) == "" {
			return fields
		}
		return append(fields, viewers.Field{Label: label, Value: value, Mono: mono})
	}
	var fields []viewers.Field
	fields = add(fields, "Test id", t.ID.Hex(), true)
	fields = add(fields, "Started", t.StartedAt.UTC().Format(time.RFC3339)+" UTC", false)
	fields = add(fields, "Last seen", t.LastSeenAt.UTC().Format(time.RFC3339)+" UTC", false)
	fields = add(fields, "Stage", stageLabel(t.Stage), false)
	if t.FailedStep != "" {
		fields = add(fields, "Last problem", stepLabel(t.FailedStep)+": "+t.FailedReason, false)
	}
	fields = add(fields, "School", t.Form.School, false)
	fields = add(fields, "Tester", joinNonEmpty(" · ", t.Form.TesterName, t.Form.TesterRole, t.Form.TesterEmail), false)
	fields = add(fields, "Device (form)", joinNonEmpty(" · ", t.Form.DeviceType, managedLabel(t.Form.ManagedDevice), t.Form.NetworkType), false)
	fields = add(fields, "Notes", t.Form.Notes, false)
	fields = add(fields, "Unit", joinNonEmpty(" ", t.UnitID, "v"+t.UnitVersion, t.BuildIdentifier)+collectionSuffix(t.CollectionName), false)
	fields = add(fields, "Device (detected)", joinNonEmpty(" · ", t.DeviceType, t.Platform, t.Browser), false)
	fields = add(fields, "Network (detected)", t.Network, false)
	fields = add(fields, "Device id", t.DeviceID, true)
	fields = add(fields, "From IP", t.RemoteIP, true)
	fields = add(fields, "User agent", t.UserAgent, true)
	if t.Download.Bytes > 0 || t.Download.Path != "" {
		fields = add(fields, "Download", fmt.Sprintf("%s · %s in %s · %.1f MB/s · %d stall(s), %d retry(ies)%s",
			t.Download.Path, mb(t.Download.Bytes), mmss(time.Duration(t.Download.Seconds*float64(time.Second))),
			t.Download.AvgBps/1048576, t.Download.Stalls, t.Download.Retries, switchedSuffix(t.Download.Switched)), false)
	}
	if t.Launch.UnityMs > 0 || t.Launch.LoaderMs > 0 {
		fields = add(fields, "Launch", fmt.Sprintf("loader %.1f s · Unity started %.1f s · first frame %.1f s",
			float64(t.Launch.LoaderMs)/1000, float64(t.Launch.UnityMs)/1000, float64(t.Launch.FirstFrameMs)/1000), false)
	}
	if t.GameplayReachedAt != nil {
		fields = add(fields, "Gameplay reached", t.GameplayReachedAt.UTC().Format(time.RFC3339)+" UTC", false)
	}
	if t.UnitCompletedAt != nil {
		fields = add(fields, "Unit completed", t.UnitCompletedAt.UTC().Format(time.RFC3339)+" UTC", false)
	}
	if t.CrashCount > 0 {
		fields = add(fields, "Crashes", fmt.Sprint(t.CrashCount), false)
	}

	g := v.loadGameplay(ctx, t.ID.Hex())
	html := v.detailHTML(t, g)

	raw, _ := json.MarshalIndent(struct {
		Run      *models.MHSDeviceTest `json:"run"`
		Gameplay gameplay              `json:"gameplay"`
	}{t, g}, "", "  ")

	title := "Test " + t.ID.Hex()
	if t.Form.School != "" {
		title = t.Form.School + " · " + title
	}
	return &viewers.Detail{Title: title, Subtitle: stageLabel(t.Stage), Fields: fields, RawJSON: string(raw), HTML: template.HTML(html)}, nil
}

func managedLabel(m string) string {
	switch m {
	case "yes":
		return "school-managed"
	case "no":
		return "not managed"
	}
	return ""
}

func collectionSuffix(name string) string {
	if name == "" {
		return ""
	}
	return " · collection " + name
}

func switchedSuffix(switched bool) string {
	if switched {
		return " · switched from background"
	}
	return ""
}

// detailHTML renders the gameplay summary, the step timeline, the tester's
// reports and the diagnostics snapshot. Every value is escaped here because
// Detail.HTML is emitted raw.
func (v *DeviceTests) detailHTML(t *models.MHSDeviceTest, g gameplay) string {
	var b strings.Builder
	b.WriteString(`<div class="space-y-3 text-sm">`)
	b.WriteString(`<div><a class="text-xs underline text-indigo-600 dark:text-indigo-400" href="/views/` + DeviceTestsSlug +
		`/rows/` + esc(t.ID.Hex()) + `/export.json">Download this run as JSON</a></div>`)

	// Gameplay
	b.WriteString(`<div><div class="font-medium text-gray-700 dark:text-gray-300">Game telemetry for this run</div>`)
	if !g.Available {
		b.WriteString(`<div class="text-xs text-gray-500 dark:text-gray-400">` + esc(g.Note) + `</div>`)
	} else {
		b.WriteString(`<div class="text-xs text-gray-600 dark:text-gray-400">`)
		b.WriteString(fmt.Sprintf("%d log event(s)", g.Events))
		if g.First != nil && g.Last != nil {
			b.WriteString(" · first " + esc(g.First.Format(time.RFC3339)) + " · last " + esc(g.Last.Format(time.RFC3339)))
		}
		if len(g.Scenes) > 0 {
			b.WriteString(" · scenes: " + esc(strings.Join(g.Scenes, ", ")))
		}
		if g.Current != "" {
			b.WriteString(" · grader current unit: " + esc(g.Current))
		}
		if len(g.Points) > 0 {
			b.WriteString("<br>Progress points: " + esc(strings.Join(g.Points, "; ")))
		}
		if g.Note != "" {
			b.WriteString("<br>" + esc(g.Note))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	// Reports
	if len(t.ProblemReports) > 0 {
		b.WriteString(`<div><div class="font-medium text-gray-700 dark:text-gray-300">Tester reports</div><ul class="list-disc ml-5 text-xs text-gray-700 dark:text-gray-300">`)
		for _, r := range t.ProblemReports {
			b.WriteString(`<li>` + esc(r.At.UTC().Format(time.RFC3339)) + ` — ` + esc(r.Note) + `</li>`)
		}
		b.WriteString(`</ul></div>`)
	}

	// Steps
	b.WriteString(`<div><div class="font-medium text-gray-700 dark:text-gray-300">Steps (` + fmt.Sprint(len(t.Steps)) + `)</div>`)
	if len(t.Steps) == 0 {
		b.WriteString(`<div class="text-xs text-gray-500 dark:text-gray-400">No steps received.</div>`)
	} else {
		b.WriteString(`<div class="overflow-auto max-h-96"><table class="text-xs w-full"><tbody>`)
		for _, s := range t.Steps {
			cls := "text-gray-500 dark:text-gray-400"
			switch s.State {
			case "ok":
				cls = "text-green-600 dark:text-green-400"
			case "warn":
				cls = "text-amber-600 dark:text-amber-400"
			case "fail":
				cls = "text-red-600 dark:text-red-400"
			case "running":
				cls = "text-blue-600 dark:text-blue-400"
			}
			b.WriteString(`<tr class="align-top"><td class="pr-2 whitespace-nowrap tabular-nums text-gray-400 dark:text-gray-500">` +
				esc(fmt.Sprintf("+%d:%04.1f", s.T/60000, float64(s.T%60000)/1000)) + `</td>`)
			b.WriteString(`<td class="pr-2 whitespace-nowrap ` + cls + `">` + esc(s.State) + `</td>`)
			b.WriteString(`<td class="pr-2 whitespace-nowrap text-gray-500 dark:text-gray-400">` + esc(stepLabel(s.Step)) + `</td>`)
			b.WriteString(`<td class="text-gray-800 dark:text-gray-100 break-words">` + esc(s.Msg))
			if len(s.Detail) > 0 {
				keys := make([]string, 0, len(s.Detail))
				for k := range s.Detail {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				parts := make([]string, 0, len(keys))
				for _, k := range keys {
					if s.Detail[k] != "" {
						parts = append(parts, k+"="+s.Detail[k])
					}
				}
				if len(parts) > 0 {
					b.WriteString(` <span class="text-gray-400 dark:text-gray-500">{` + esc(strings.Join(parts, ", ")) + `}</span>`)
				}
			}
			b.WriteString(`</td></tr>`)
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</div>`)

	// Diagnostics
	if len(t.Diagnostics) > 0 {
		keys := make([]string, 0, len(t.Diagnostics))
		for k := range t.Diagnostics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(`<details><summary class="cursor-pointer select-none text-xs text-gray-500 dark:text-gray-400">Diagnostics (` + fmt.Sprint(len(keys)) + `)</summary>`)
		b.WriteString(`<dl class="mt-1 grid gap-x-3 gap-y-0.5 text-xs" style="grid-template-columns: max-content 1fr;">`)
		for _, k := range keys {
			val := fmt.Sprint(t.Diagnostics[k])
			if m, ok := t.Diagnostics[k].(map[string]interface{}); ok {
				if j, err := json.Marshal(m); err == nil {
					val = string(j)
				}
			} else if arr, ok := t.Diagnostics[k].(primitive.A); ok {
				if j, err := json.Marshal(arr); err == nil {
					val = string(j)
				}
			}
			if len(val) > 400 {
				val = val[:397] + "…"
			}
			b.WriteString(`<dt class="text-gray-500 dark:text-gray-400 whitespace-nowrap">` + esc(k) + `</dt><dd class="text-gray-800 dark:text-gray-100 break-all">` + esc(val) + `</dd>`)
		}
		b.WriteString(`</dl></details>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func (v *DeviceTests) Summary(ctx context.Context, scope *viewscope.Scope, f viewers.Filters) ([]viewers.Chip, error) {
	q, err := v.listQuery(ctx, scope, f)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return []viewers.Chip{{Label: "Runs", Value: "0"}}, nil
	}
	sum, err := v.store.Summarize(ctx, *q)
	if err != nil {
		return nil, err
	}
	pct := func(n int64) string {
		if sum.Total == 0 {
			return "0"
		}
		return fmt.Sprintf("%d (%d%%)", n, n*100/sum.Total)
	}
	chips := []viewers.Chip{
		{Label: "Runs", Value: fmt.Sprint(sum.Total)},
		{Label: "Reached gameplay", Value: pct(sum.Gameplay), Class: viewers.TextGreen},
		{Label: "Completed", Value: pct(sum.Completed), Class: viewers.TextGreen},
	}
	failed := viewers.Chip{Label: "Failed now", Value: fmt.Sprint(sum.Failed)}
	if sum.Failed > 0 {
		failed.Class = viewers.TextRed
	}
	chips = append(chips, failed)
	last := "—"
	if sum.Last != nil {
		last = relativeTime(time.Since(*sum.Last))
	}
	chips = append(chips, viewers.Chip{Label: "Last run", Value: last, Class: viewers.TextMuted})
	return chips, nil
}

// ExportJSON streams every run matching the filter, in full (steps,
// diagnostics, reports), newest first, as a JSON array — the full-fidelity
// download for analysis.
func (v *DeviceTests) ExportJSON(ctx context.Context, scope *viewscope.Scope, f viewers.Filters, w io.Writer) error {
	q, err := v.listQuery(ctx, scope, f)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, "[\n"); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	written := 0
	if q != nil {
		q.Limit = 200
		for written < deviceTestsExportMax {
			runs, more, err := v.store.ListFull(ctx, *q)
			if err != nil {
				return err
			}
			for _, t := range runs {
				if written > 0 {
					if _, err := io.WriteString(w, ",\n"); err != nil {
						return err
					}
				}
				if err := enc.Encode(t); err != nil {
					return err
				}
				written++
			}
			if !more || len(runs) == 0 {
				break
			}
			last := runs[len(runs)-1]
			q.After = &mhsdevicetests.Position{StartedAt: last.StartedAt, ID: last.ID}
		}
	}
	_, err = io.WriteString(w, "\n]\n")
	return err
}

// DetailJSON returns one run in full plus its gameplay summary.
func (v *DeviceTests) DetailJSON(ctx context.Context, scope *viewscope.Scope, id string) ([]byte, error) {
	t, err := v.load(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	g := v.loadGameplay(ctx, t.ID.Hex())
	return json.MarshalIndent(struct {
		Run      *models.MHSDeviceTest `json:"run"`
		Gameplay gameplay              `json:"gameplay"`
	}{t, g}, "", "  ")
}
