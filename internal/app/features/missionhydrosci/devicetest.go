// internal/app/features/missionhydrosci/devicetest.go
//
// The Unit 2 Device Test: one public URL per workspace that downloads and
// plays the configured Unit 2 build exactly as a student would, with no
// account or session, and records everything about the run. Design and
// rationale: docs/mission-hydrosci/mhs-loading-status-and-unit2-device-test-plan.md §4.
package missionhydrosci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/mhsbuilds"
	"github.com/dalemusser/stratahub/internal/app/store/mhscollections"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicetests"
	"github.com/dalemusser/stratahub/internal/app/system/format"
	"github.com/dalemusser/stratahub/internal/app/system/ratelimit"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// DeviceTestPathPrefix is the public route. Referenced by the maintenance-mode
// exemption and the bootstrap registration; keep them in step.
const DeviceTestPathPrefix = "/missionhydrosci/devicetest"

const (
	deviceTestStartLimit  = 10 // runs one client IP may start per window
	deviceTestStartWindow = 10 * time.Minute
	deviceTestRunTTL      = 24 * time.Hour // a run stops accepting writes after this

	deviceTestMaxBody      = 64 << 10
	deviceTestMaxSteps     = 60 // per POST; the page sends at most 40
	deviceTestMaxField     = 200
	deviceTestMaxNotes     = 2000
	deviceTestMaxDiagKeys  = 120
	deviceTestMaxDiagValue = 500
)

// MountDeviceTestRoutes registers the public device-test routes on a router
// already mounted at DeviceTestPathPrefix (no session; workspace from host).
func (h *Handler) MountDeviceTestRoutes(r chi.Router) {
	r.Get("/", h.ServeDeviceTestLanding)
	r.Post("/start", h.HandleDeviceTestStart)
	r.Route("/run/{testId}", func(rr chi.Router) {
		rr.Get("/", h.ServeDeviceTestRun)
		rr.Get("/manifest", h.ServeDeviceTestManifest)
		rr.Get("/play", h.ServeDeviceTestPlay)
		rr.Post("/diagnostics", h.HandleDeviceTestDiagnostics)
		rr.Post("/steps", h.HandleDeviceTestSteps)
		rr.Post("/summary", h.HandleDeviceTestSummary)
		rr.Post("/report", h.HandleDeviceTestReport)
		rr.Post("/complete", h.HandleDeviceTestComplete)
	})
}

// deviceTestConfig is what the workspace's settings resolve to.
type deviceTestConfig struct {
	Enabled        bool
	Reason         string // why it is unavailable (admin-facing wording is fine; testers see a generic line)
	Unit           ContentManifestUnit
	Build          models.MHSBuild
	CollectionName string
}

// resolveDeviceTest reads the workspace's device-test settings and finds the
// Unit 2 build to run: the configured one, else the active collection's.
func (h *Handler) resolveDeviceTest(ctx context.Context, wsID primitive.ObjectID) deviceTestConfig {
	settings, err := h.SettingsStore.Get(ctx, wsID)
	if err != nil {
		return deviceTestConfig{Reason: "settings unavailable: " + err.Error()}
	}
	if !settings.MHSDeviceTestEnabled {
		return deviceTestConfig{Reason: "disabled in Site Settings"}
	}
	unitID := models.MHSDeviceTestUnitID
	title := "Unit 2"
	version := ""
	collectionName := ""
	if settings.MHSDeviceTestUnit != nil && settings.MHSDeviceTestUnit.Version != "" {
		version = settings.MHSDeviceTestUnit.Version
	} else if settings.MHSActiveCollectionID != nil {
		coll, err := h.CollectionStore.GetByID(ctx, *settings.MHSActiveCollectionID)
		if err != nil && !errors.Is(err, mhscollections.ErrNotFound) {
			return deviceTestConfig{Reason: "active collection unavailable: " + err.Error()}
		}
		if err == nil {
			collectionName = coll.Name
			for _, u := range coll.Units {
				if u.UnitID == unitID {
					version = u.Version
					if u.Title != "" {
						title = u.Title
					}
					break
				}
			}
		}
	}
	if version == "" {
		return deviceTestConfig{Reason: "no Unit 2 build is configured and the active collection has none"}
	}
	build, err := h.BuildStore.GetByUnitVersion(ctx, unitID, version)
	if err != nil {
		if errors.Is(err, mhsbuilds.ErrNotFound) {
			return deviceTestConfig{Reason: "the configured Unit 2 build (v" + version + ") no longer exists"}
		}
		return deviceTestConfig{Reason: "build lookup failed: " + err.Error()}
	}
	return deviceTestConfig{
		Enabled:        true,
		Unit:           buildToManifestUnit(unitID, title, version, "", build),
		Build:          build,
		CollectionName: collectionName,
	}
}

// --- pages ------------------------------------------------------------------

type deviceTestLandingData struct {
	viewdata.BaseVM
	Enabled     bool
	UnitTitle   string
	UnitVersion string
	SizeLabel   string
	Form        models.MHSDeviceTestForm
	Error       string
}

// ServeDeviceTestLanding renders the public landing page: what the test does,
// what is collected, and the form.
func (h *Handler) ServeDeviceTestLanding(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	h.renderDeviceTestLanding(w, r, ctx, models.MHSDeviceTestForm{}, "")
}

func (h *Handler) renderDeviceTestLanding(w http.ResponseWriter, r *http.Request, ctx context.Context, form models.MHSDeviceTestForm, errMsg string) {
	wsID := workspace.IDFromRequest(r)
	cfg := h.resolveDeviceTest(ctx, wsID)
	if !cfg.Enabled {
		h.Log.Info("device test: not available", zap.String("workspace_id", wsID.Hex()), zap.String("reason", cfg.Reason))
	}
	data := deviceTestLandingData{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "Unit 2 Device Test", "/"),
		Enabled:     cfg.Enabled,
		UnitTitle:   cfg.Unit.Title,
		UnitVersion: cfg.Unit.Version,
		SizeLabel:   format.Bytes(cfg.Unit.TotalSize),
		Form:        form,
		Error:       errMsg,
	}
	w.Header().Set("Cache-Control", "no-store")
	templates.Render(w, r, "devicetest_landing", data)
}

// HandleDeviceTestStart validates the form, creates the run with a marked
// game id, and redirects to the run page.
func (h *Handler) HandleDeviceTestStart(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	if err := r.ParseForm(); err != nil {
		h.renderDeviceTestLanding(w, r, ctx, models.MHSDeviceTestForm{}, "The form could not be read. Please try again.")
		return
	}
	clip := func(name string, n int) string { return models.ClipRunes(strings.TrimSpace(r.FormValue(name)), n) }
	form := models.MHSDeviceTestForm{
		School:        clip("school", deviceTestMaxField),
		TesterName:    clip("tester_name", deviceTestMaxField),
		TesterRole:    clip("tester_role", deviceTestMaxField),
		TesterEmail:   strings.ToLower(clip("tester_email", deviceTestMaxField)),
		DeviceType:    clip("device_type", 40),
		ManagedDevice: clip("managed_device", 10),
		NetworkType:   clip("network_type", 40),
		Notes:         clip("notes", deviceTestMaxNotes),
	}
	if form.School == "" || form.TesterName == "" {
		h.renderDeviceTestLanding(w, r, ctx, form, "Please enter your school or district and your name.")
		return
	}
	if form.TesterEmail != "" && (!strings.Contains(form.TesterEmail, "@") || strings.ContainsAny(form.TesterEmail, " \t\n")) {
		h.renderDeviceTestLanding(w, r, ctx, form, "That email address doesn't look right. You can also leave it blank.")
		return
	}

	wsID := workspace.IDFromRequest(r)
	cfg := h.resolveDeviceTest(ctx, wsID)
	if !cfg.Enabled {
		h.renderDeviceTestLanding(w, r, ctx, form, "The device test is not enabled for this site right now.")
		return
	}

	ip := ratelimit.ClientIP(r)
	if !h.startLimiter.Allow(ip) {
		h.Log.Warn("device test: start rate limited", zap.String("ip", ip))
		w.Header().Set("Retry-After", fmt.Sprint(int(deviceTestStartWindow.Seconds())))
		h.renderDeviceTestLanding(w, r, ctx, form, "Too many test runs have been started from this network in the last few minutes. Please wait a little and try again.")
		return
	}

	idHex, err := models.NewMHSDeviceTestUserID()
	if err != nil {
		h.ErrLog.LogServerError(w, r, "device test: mint id failed", err, "Couldn't start the test. Please try again.", DeviceTestPathPrefix)
		return
	}
	oid, _ := primitive.ObjectIDFromHex(idHex)
	now := time.Now().UTC()
	run := models.MHSDeviceTest{
		ID:              oid,
		WorkspaceID:     wsID,
		Kind:            models.MHSDeviceTestKindDeviceTest,
		UnitID:          cfg.Unit.ID,
		UnitVersion:     cfg.Unit.Version,
		BuildIdentifier: cfg.Build.BuildIdentifier,
		CollectionName:  cfg.CollectionName,
		Form:            form,
		DeviceType:      form.DeviceType,
		RemoteIP:        ip,
		UserAgent:       models.ClipRunes(r.UserAgent(), 400),
		StartedAt:       now,
		ExpiresAt:       now.Add(deviceTestRunTTL),
		Stage:           models.MHSDeviceTestStageRun,
		ReachedStage:    models.MHSDeviceTestStageRun,
	}
	if _, err := h.DeviceTestStore.Create(ctx, run); err != nil {
		h.ErrLog.LogServerError(w, r, "device test: create run failed", err, "Couldn't start the test. Please try again.", DeviceTestPathPrefix)
		return
	}
	h.Log.Info("device test: run started",
		zap.String("workspace_id", wsID.Hex()),
		zap.String("test_id", idHex),
		zap.String("school", form.School),
		zap.String("unit", cfg.Unit.ID+" v"+cfg.Unit.Version))
	http.Redirect(w, r, DeviceTestPathPrefix+"/run/"+idHex, http.StatusSeeOther)
}

// loadDeviceTestRun resolves {testId} to an open or closed run in this
// workspace. A malformed or unknown id renders 404 and returns false.
func (h *Handler) loadDeviceTestRun(w http.ResponseWriter, r *http.Request, ctx context.Context, htmlPage bool) (models.MHSDeviceTest, bool) {
	idHex := strings.ToLower(chi.URLParam(r, "testId"))
	if !models.IsMHSDeviceTestUserID(idHex) {
		h.deviceTestNotFound(w, r, htmlPage)
		return models.MHSDeviceTest{}, false
	}
	oid, _ := primitive.ObjectIDFromHex(idHex)
	run, err := h.DeviceTestStore.Get(ctx, workspace.IDFromRequest(r), oid)
	if err != nil {
		if !errors.Is(err, mhsdevicetests.ErrNotFound) {
			h.Log.Error("device test: load run failed", zap.Error(err))
		}
		h.deviceTestNotFound(w, r, htmlPage)
		return models.MHSDeviceTest{}, false
	}
	return run, true
}

func (h *Handler) deviceTestNotFound(w http.ResponseWriter, r *http.Request, htmlPage bool) {
	if htmlPage {
		uierrors.RenderNotFound(w, r, "That device-test link is not valid. Start a new test from the device-test page.", DeviceTestPathPrefix)
		return
	}
	writeDeviceTestJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
}

type deviceTestRunData struct {
	viewdata.BaseVM
	TestID      string
	ShortID     string
	Base        string
	School      string
	Completed   bool
	Expired     bool
	UnitID      string
	UnitTitle   string
	UnitVersion string
	SizeLabel   string
	ManifestURL string
	PlayURL     string
}

// ServeDeviceTestRun renders the run page: one Unit 2 card with the status
// panel, download and launch.
func (h *Handler) ServeDeviceTestRun(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	run, ok := h.loadDeviceTestRun(w, r, ctx, true)
	if !ok {
		return
	}
	build, err := h.BuildStore.GetByUnitVersion(ctx, run.UnitID, run.UnitVersion)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "device test: build lookup failed", err, "The unit for this test is no longer available.", DeviceTestPathPrefix)
		return
	}
	base := DeviceTestPathPrefix + "/run/" + run.ID.Hex()
	data := deviceTestRunData{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "Unit 2 Device Test", DeviceTestPathPrefix),
		TestID:      run.ID.Hex(),
		ShortID:     deviceTestShortID(run.ID.Hex()),
		Base:        base,
		School:      run.Form.School,
		Completed:   run.UnitCompletedAt != nil,
		Expired:     time.Now().After(run.ExpiresAt),
		UnitID:      run.UnitID,
		UnitTitle:   deviceTestUnitTitle(run),
		UnitVersion: run.UnitVersion,
		SizeLabel:   format.Bytes(build.TotalSize),
		ManifestURL: base + "/manifest",
		PlayURL:     base + "/play",
	}
	w.Header().Set("Cache-Control", "no-store")
	templates.Render(w, r, "devicetest_run", data)
}

func deviceTestShortID(hex string) string {
	if len(hex) < 6 {
		return hex
	}
	return strings.ToUpper(hex[len(hex)-6:])
}

func deviceTestUnitTitle(run models.MHSDeviceTest) string {
	if run.UnitID == models.MHSDeviceTestUnitID {
		return "Unit 2"
	}
	return run.UnitID
}

// ServeDeviceTestManifest returns the run's one-unit manifest, pinned to the
// build the run started with.
func (h *Handler) ServeDeviceTestManifest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	run, ok := h.loadDeviceTestRun(w, r, ctx, false)
	if !ok {
		return
	}
	build, err := h.BuildStore.GetByUnitVersion(ctx, run.UnitID, run.UnitVersion)
	if err != nil {
		writeDeviceTestJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "build_missing"})
		return
	}
	m := h.oneUnitManifest(run.UnitID, deviceTestUnitTitle(run), run.UnitVersion, build)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(m)
}

// ServeDeviceTestPlay renders the Unity host page for the run: the same
// template, game-service URLs and keys as a student launch, with the run's
// marked id as the game's user_id.
func (h *Handler) ServeDeviceTestPlay(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	run, ok := h.loadDeviceTestRun(w, r, ctx, true)
	if !ok {
		return
	}
	build, err := h.BuildStore.GetByUnitVersion(ctx, run.UnitID, run.UnitVersion)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "device test: build lookup failed", err, "The unit for this test is no longer available.", DeviceTestPathPrefix)
		return
	}
	base := DeviceTestPathPrefix + "/run/" + run.ID.Hex()
	h.renderPlay(w, r, playRender{
		Unit:      buildToManifestUnit(run.UnitID, deviceTestUnitTitle(run), run.UnitVersion, "", build),
		UserName:  "Device Test",
		UserIDHex: run.ID.Hex(),
		BackURL:   base,
		DeviceTest: &deviceTestPlay{
			ID:      run.ID.Hex(),
			ShortID: deviceTestShortID(run.ID.Hex()),
			Base:    base,
		},
	})
}

// --- writes from the page ---------------------------------------------------

func writeDeviceTestJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// deviceTestWriteError maps store errors to responses for the page.
func (h *Handler) deviceTestWriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mhsdevicetests.ErrNotFound):
		writeDeviceTestJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	case errors.Is(err, mhsdevicetests.ErrClosed):
		writeDeviceTestJSON(w, http.StatusGone, map[string]any{"ok": false, "error": "closed", "message": "This test run is finished or expired."})
	default:
		h.Log.Error("device test: write failed", zap.Error(err))
		writeDeviceTestJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "server_error"})
	}
}

func decodeDeviceTestBody(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, deviceTestMaxBody)).Decode(v)
}

// deviceTestDiagnosticsRequest is the snapshot the run page posts once.
type deviceTestDiagnosticsRequest struct {
	DeviceID    string                 `json:"device_id"`
	DeviceType  string                 `json:"device_type"`
	Platform    string                 `json:"platform"`
	Browser     string                 `json:"browser"`
	Network     string                 `json:"network"`
	Diagnostics map[string]interface{} `json:"diagnostics"`
}

// HandleDeviceTestDiagnostics stores the device/browser/storage/network
// snapshot and the summary columns derived from it.
func (h *Handler) HandleDeviceTestDiagnostics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	run, ok := h.loadDeviceTestRun(w, r, ctx, false)
	if !ok {
		return
	}
	var req deviceTestDiagnosticsRequest
	if err := decodeDeviceTestBody(r, &req); err != nil {
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	set := bson.M{
		"diagnostics": boundDiagnostics(req.Diagnostics),
	}
	if v := models.ClipRunes(strings.TrimSpace(req.DeviceID), 64); v != "" {
		set["device_id"] = v
	}
	if v := models.ClipRunes(strings.TrimSpace(req.DeviceType), 40); v != "" {
		set["device_type"] = v
	}
	if v := models.ClipRunes(strings.TrimSpace(req.Platform), 120); v != "" {
		set["platform"] = v
	}
	if v := models.ClipRunes(strings.TrimSpace(req.Browser), 120); v != "" {
		set["browser"] = v
	}
	if v := models.ClipRunes(strings.TrimSpace(req.Network), 120); v != "" {
		set["network"] = v
	}
	if err := h.DeviceTestStore.SetFields(ctx, run.WorkspaceID, run.ID, set); err != nil {
		h.deviceTestWriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// boundDiagnostics caps the snapshot: at most deviceTestMaxDiagKeys keys,
// strings clipped, nested values re-encoded as clipped JSON strings.
func boundDiagnostics(in map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	n := 0
	for k, v := range in {
		if n >= deviceTestMaxDiagKeys {
			break
		}
		k = models.ClipRunes(k, 80)
		switch t := v.(type) {
		case string:
			out[k] = models.ClipRunes(t, deviceTestMaxDiagValue)
		case bool, float64, nil:
			out[k] = t
		default:
			b, err := json.Marshal(t)
			if err != nil {
				continue
			}
			out[k] = models.ClipRunes(string(b), deviceTestMaxDiagValue)
		}
		n++
	}
	return out
}

// deviceTestStepsRequest is a batch from the page's step log.
type deviceTestStepsRequest struct {
	Context map[string]interface{} `json:"context"`
	Entries []struct {
		T      int64             `json:"t"`
		At     string            `json:"at"`
		Step   string            `json:"step"`
		State  string            `json:"state"`
		Msg    string            `json:"msg"`
		Detail map[string]string `json:"detail"`
	} `json:"entries"`
}

var deviceTestStageRank = map[string]int{
	models.MHSDeviceTestStageForm:        0,
	models.MHSDeviceTestStageRun:         1,
	models.MHSDeviceTestStageDownloading: 2,
	models.MHSDeviceTestStageDownloaded:  3,
	models.MHSDeviceTestStageLaunching:   4,
	models.MHSDeviceTestStageGameplay:    5,
	models.MHSDeviceTestStageCompleted:   6,
}

// stageFromStep maps a step-log entry to the stage it proves was reached.
func stageFromStep(step, state string) string {
	switch step {
	case "download":
		if state == "ok" {
			return models.MHSDeviceTestStageDownloaded
		}
		if state == "running" || state == "warn" {
			return models.MHSDeviceTestStageDownloading
		}
	case "verify":
		return models.MHSDeviceTestStageDownloaded
	case "launch":
		return models.MHSDeviceTestStageLaunching
	case "game":
		if state == "running" || state == "ok" {
			return models.MHSDeviceTestStageGameplay
		}
	}
	return ""
}

// HandleDeviceTestSteps appends step-log entries and updates the run's stage
// from them: the furthest stage reached, or "failed" while the latest step
// is a failure (the page keeps retrying, so a later success moves it on).
func (h *Handler) HandleDeviceTestSteps(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	run, ok := h.loadDeviceTestRun(w, r, ctx, false)
	if !ok {
		return
	}
	var req deviceTestStepsRequest
	if err := decodeDeviceTestBody(r, &req); err != nil {
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if len(req.Entries) > deviceTestMaxSteps {
		req.Entries = req.Entries[:deviceTestMaxSteps]
	}
	now := time.Now().UTC()
	initial := run.ReachedStage
	if initial == "" {
		initial = run.Stage
	}
	steps, derived := deriveFromStepsWith(req, initial, run.Stage == models.MHSDeviceTestStageFailed,
		run.FailedStep, run.FailedReason, run.GameplayReachedAt != nil, now)
	set := bson.M{"reached_stage": derived.reached}
	if run.UnitCompletedAt == nil {
		set["stage"] = derived.stage
	}
	if derived.failedStep != "" {
		set["failed_step"] = derived.failedStep
		set["failed_reason"] = derived.failedReason
	}
	if derived.gameplayAt != nil {
		set["gameplay_reached_at"] = *derived.gameplayAt
	}
	if len(req.Context) > 0 {
		set["diagnostics.steplog_context"] = boundDiagnostics(req.Context)
	}
	if err := h.DeviceTestStore.AppendSteps(ctx, run.WorkspaceID, run.ID, steps, set); err != nil {
		h.deviceTestWriteError(w, err)
		return
	}
	writeDeviceTestJSON(w, http.StatusOK, map[string]any{"ok": true, "stored": len(steps)})
}

// derivedStages is what a batch of step entries says about a run.
type derivedStages struct {
	reached      string // furthest stage reached
	stage        string // reached, or "failed" while the latest step is a failure
	failedStep   string
	failedReason string
	gameplayAt   *time.Time // first time gameplay was reached in this batch (nil if earlier or never)
}

// deriveFromSteps converts and bounds a batch of entries and derives the
// stages from a fresh starting point.
func deriveFromSteps(req deviceTestStepsRequest, initial string, now time.Time) ([]models.MHSDeviceTestStep, derivedStages) {
	return deriveFromStepsWith(req, initial, false, "", "", false, now)
}

// deriveFromStepsWith is deriveFromSteps continuing from a run's stored
// state (its reached stage, whether it is currently failed and why, and
// whether gameplay was already reached).
func deriveFromStepsWith(req deviceTestStepsRequest, initial string, failed bool, failedStep, failedReason string, gameplayKnown bool, now time.Time) ([]models.MHSDeviceTestStep, derivedStages) {
	reached := initial
	if reached == "" {
		reached = models.MHSDeviceTestStageRun
	}
	var gameplayAt *time.Time
	steps := make([]models.MHSDeviceTestStep, 0, len(req.Entries))
	for _, e := range req.Entries {
		at, err := time.Parse(time.RFC3339Nano, e.At)
		if err != nil {
			at = now
		}
		st := models.MHSDeviceTestStep{
			T:     e.T,
			At:    at.UTC(),
			Step:  models.ClipRunes(e.Step, 32),
			State: models.ClipRunes(e.State, 16),
			Msg:   models.ClipRunes(e.Msg, 500),
		}
		if len(e.Detail) > 0 {
			st.Detail = map[string]string{}
			n := 0
			for k, v := range e.Detail {
				if n >= 20 {
					break
				}
				st.Detail[models.ClipRunes(k, 40)] = models.ClipRunes(v, 300)
				n++
			}
		}
		steps = append(steps, st)

		if s := stageFromStep(st.Step, st.State); s != "" && deviceTestStageRank[s] > deviceTestStageRank[reached] {
			reached = s
			if s == models.MHSDeviceTestStageGameplay && !gameplayKnown && gameplayAt == nil {
				t := st.At
				gameplayAt = &t
			}
		}
		switch st.State {
		case "fail":
			failed = true
			failedStep, failedReason = st.Step, st.Msg
		case "ok", "running":
			failed = false
		}
	}
	d := derivedStages{reached: reached, stage: reached, failedStep: failedStep, failedReason: failedReason, gameplayAt: gameplayAt}
	if failed {
		d.stage = models.MHSDeviceTestStageFailed
	}
	return steps, d
}

// deviceTestSummaryRequest carries the structured summaries the pages
// compute at milestones.
type deviceTestSummaryRequest struct {
	Download        *models.MHSDeviceTestDownload `json:"download"`
	Launch          *models.MHSDeviceTestLaunch   `json:"launch"`
	GameplayReached bool                          `json:"gameplay_reached"`
	Crash           bool                          `json:"crash"`
}

// HandleDeviceTestSummary stores download/launch summaries, the gameplay
// timestamp, and crash counts.
func (h *Handler) HandleDeviceTestSummary(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	run, ok := h.loadDeviceTestRun(w, r, ctx, false)
	if !ok {
		return
	}
	var req deviceTestSummaryRequest
	if err := decodeDeviceTestBody(r, &req); err != nil {
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	update := bson.M{}
	set := bson.M{}
	if req.Download != nil {
		d := *req.Download
		d.Path = models.ClipRunes(d.Path, 20)
		set["download"] = d
	}
	if req.Launch != nil {
		set["launch"] = *req.Launch
	}
	if req.GameplayReached && run.GameplayReachedAt == nil {
		set["gameplay_reached_at"] = time.Now().UTC()
		if run.UnitCompletedAt == nil && deviceTestStageRank[run.ReachedStage] < deviceTestStageRank[models.MHSDeviceTestStageGameplay] {
			set["reached_stage"] = models.MHSDeviceTestStageGameplay
			if run.Stage != models.MHSDeviceTestStageFailed {
				set["stage"] = models.MHSDeviceTestStageGameplay
			}
		}
	}
	if len(set) > 0 {
		update["$set"] = set
	}
	if req.Crash {
		update["$inc"] = bson.M{"crash_count": 1}
	}
	if len(update) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.DeviceTestStore.Apply(ctx, run.WorkspaceID, run.ID, update); err != nil {
		h.deviceTestWriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleDeviceTestReport stores a note the tester typed.
func (h *Handler) HandleDeviceTestReport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	run, ok := h.loadDeviceTestRun(w, r, ctx, false)
	if !ok {
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if err := decodeDeviceTestBody(r, &req); err != nil {
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	note := models.ClipRunes(strings.TrimSpace(req.Note), deviceTestMaxNotes)
	if note == "" {
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "empty"})
		return
	}
	if err := h.DeviceTestStore.AddReport(ctx, run.WorkspaceID, run.ID, note); err != nil {
		h.deviceTestWriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleDeviceTestComplete records that the game reported the unit complete.
func (h *Handler) HandleDeviceTestComplete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	run, ok := h.loadDeviceTestRun(w, r, ctx, false)
	if !ok {
		return
	}
	if err := h.DeviceTestStore.Complete(ctx, run.WorkspaceID, run.ID); err != nil {
		h.deviceTestWriteError(w, err)
		return
	}
	h.Log.Info("device test: unit completed", zap.String("test_id", run.ID.Hex()), zap.String("school", run.Form.School))
	writeDeviceTestJSON(w, http.StatusOK, map[string]any{"ok": true, "short_id": deviceTestShortID(run.ID.Hex())})
}
