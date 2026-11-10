package missionhydrosci

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
)

func renderPlayPage(t *testing.T, data PlayData) string {
	t.Helper()
	bootCeremonyTemplates(t)
	rec := httptest.NewRecorder()
	templates.Render(rec, httptest.NewRequest("GET", "/missionhydrosci/play/unit1", nil), "missionhydrosci_play", data)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func memberPlayData() PlayData {
	return PlayData{
		BaseVM:          viewdata.BaseVM{CSRFToken: "token"},
		UnitID:          "unit1",
		UnitTitle:       "Unit 1",
		UnitVersion:     "2.8.9",
		CDNBaseURL:      "https://cdn.example.com/mhs",
		UserName:        "Pat O'Neil",
		UserIDHex:       "69b4449ec6006ac370dad9df",
		NextUnitID:      "unit2",
		NextUnitVersion: "2.8.9",
		DataFile:        "unit1.data.unityweb",
		FrameworkFile:   "unit1.framework.js.unityweb",
		CodeFile:        "unit1.wasm.unityweb",
		LogSubmitURL:    "https://log.example.com/api/log/submit",
		LogAuth:         "Bearer log-key",
		StateSaveURL:    "https://save.example.com/api/state/save",
		StateLoadURL:    "https://save.example.com/api/state/load",
		SettingsSaveURL: "https://save.example.com/api/settings/save",
		SettingsLoadURL: "https://save.example.com/api/settings/load",
		SaveAuth:        "Bearer save-key",
		PlayBackURL:     "/missionhydrosci/units",
		SignInURL:       "/login?return=%2Fmissionhydrosci%2Fsigned-in",
	}
}

// The play page of a signed-in student carries the signed-out bar: its link
// to the sign-in page (returning to the "signed in again" page), and the
// code that shows it when StrataHub answers the page's posts with 401.
func TestPlayPageHasSignedOutBar(t *testing.T) {
	body := renderPlayPage(t, memberPlayData())
	for _, want := range []string{
		`id="mhs-signedout-notice"`,
		`id="mhs-so-signin"`,
		`target="_blank"`,
		`MHSStepLog.signInState(`,
		`onSignInStatus(resp.status)`,
		`new BroadcastChannel('mhs-signin')`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered play page lacks %q", want)
		}
	}
	link := regexp.MustCompile(`id="mhs-so-signin" href="([^"]*)"`).FindStringSubmatch(body)
	if link == nil || !strings.HasPrefix(link[1], "/login?return=") || !strings.Contains(link[1], "signed-in") {
		t.Errorf("sign-in link = %v, want the login page returning to the signed-in page", link)
	}
	norm := regexp.MustCompile(`\s+`).ReplaceAllString(body, " ")
	if !strings.Contains(norm, "var signInWatch = true ;") && !strings.Contains(norm, "var signInWatch = true;") {
		t.Errorf("the sign-in watch is not on for a member's page")
	}
}

// A device test has no account: no bar, and the watch is off.
func TestDeviceTestPlayPageHasNoSignedOutBar(t *testing.T) {
	data := memberPlayData()
	data.DeviceTest = true
	data.DeviceTestID = "ffffffff0123456789abcdef"
	data.DeviceTestShortID = "abcdef"
	data.DeviceTestBase = "/missionhydrosci/devicetest/run/ffffffff0123456789abcdef"
	data.SignInURL = ""
	body := renderPlayPage(t, data)
	if strings.Contains(body, `id="mhs-signedout-notice"`) {
		t.Errorf("a device-test play page must not carry the signed-out bar")
	}
	norm := regexp.MustCompile(`\s+`).ReplaceAllString(body, " ")
	if !strings.Contains(norm, "var signInWatch = false ;") && !strings.Contains(norm, "var signInWatch = false;") {
		t.Errorf("the sign-in watch must be off on a device-test page")
	}
}

// Every inline script of the rendered play page must be valid JavaScript,
// for a member's page and for a device-test page (the template has
// branches for each). Needs Node; skipped where it is not installed.
func TestPlayPageScriptsParse(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	deviceTest := memberPlayData()
	deviceTest.DeviceTest = true
	deviceTest.DeviceTestID = "ffffffff0123456789abcdef"
	deviceTest.DeviceTestShortID = "abcdef"
	deviceTest.DeviceTestBase = "/missionhydrosci/devicetest/run/ffffffff0123456789abcdef"
	deviceTest.SignInURL = ""

	inline := regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	for name, data := range map[string]PlayData{"member": memberPlayData(), "devicetest": deviceTest} {
		body := renderPlayPage(t, data)
		// Scripts inside HTML comments (the switched-off look shim) are not run.
		body = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(body, "")
		blocks := inline.FindAllStringSubmatch(body, -1)
		if len(blocks) < 3 {
			t.Fatalf("%s: found %d inline scripts, expected several", name, len(blocks))
		}
		for i, b := range blocks {
			file := filepath.Join(t.TempDir(), name+"-script.js")
			if err := os.WriteFile(file, []byte(b[1]), 0o600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(node, "--check", file).CombinedOutput(); err != nil {
				t.Errorf("%s: inline script %d does not parse:\n%s", name, i+1, out)
			}
		}
	}
}

// The page the bar's link returns to after signing in.
func TestSignedInPageRenders(t *testing.T) {
	bootCeremonyTemplates(t)
	rec := httptest.NewRecorder()
	templates.Render(rec, httptest.NewRequest("GET", SignedInPath, nil), "missionhydrosci_signedin",
		struct{ viewdata.BaseVM }{BaseVM: viewdata.BaseVM{Title: "Signed in"}.AsBare()})
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, body)
	}
	for _, want := range []string{"You are signed in again", "Close this tab", "BroadcastChannel('mhs-signin')"} {
		if !strings.Contains(body, want) {
			t.Errorf("signed-in page lacks %q", want)
		}
	}
	if strings.Contains(body, `href="/missionhydrosci/play/`) {
		t.Errorf("the signed-in page must not offer a way to start a second game")
	}
}

// TestUnitsPageScriptsParse runs node --check over the units page's inline
// scripts, as TestPlayPageScriptsParse does for the play page: the launcher's
// progress label and the engine-cache cleanup live in them.
func TestUnitsPageScriptsParse(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	bootCeremonyTemplates(t)
	rec := httptest.NewRecorder()
	templates.Render(rec, httptest.NewRequest("GET", "/missionhydrosci/units", nil), "missionhydrosci_units", UnitsData{
		BaseVM: viewdata.BaseVM{Title: "Mission HydroSci"}, CurrentUnit: "unit1",
	})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(rec.Body.String(), "")
	blocks := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllStringSubmatch(body, -1)
	if len(blocks) == 0 {
		t.Fatal("found no inline scripts")
	}
	for i, b := range blocks {
		file := filepath.Join(t.TempDir(), "units-script.js")
		if err := os.WriteFile(file, []byte(b[1]), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(node, "--check", file).CombinedOutput(); err != nil {
			t.Errorf("inline script %d does not parse:\n%s", i+1, out)
		}
	}
}

// The play page carries the game-service probes (with their POST routes) and
// the blocked form of the logging bar: the student's line, the teacher's
// steps for IT, and the heartbeat field that tells the server.
func TestPlayPageProbesGameServices(t *testing.T) {
	data := memberPlayData()
	data.ServiceProbes = ProbesFromServices(GameServices{LogSubmitURL: data.LogSubmitURL, StateSaveURL: data.StateSaveURL})
	body := renderPlayPage(t, data)
	for _, want := range []string{
		`"post":"https://log.example.com/api/log/probe"`,
		`"post":"https://save.example.com/api/state/probe"`,
		`id="mhs-ln-how-blocked"`,
		`class="mhs-ln-hosts"`,
		`including <code>OPTIONS</code> and <code>POST</code>`,
		`services_blocked:`,
		`showLoggingNotice('blocked')`,
		`MHSDeliveryManager.probeServices(probes, auth)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("play page lacks %q", want)
		}
	}

	// Without configured services the page has no probes and runs none.
	body = renderPlayPage(t, memberPlayData())
	if !strings.Contains(body, "var probes =  null  || [];") {
		t.Errorf("play page without probes: %q", body[strings.Index(body, "var probes"):strings.Index(body, "var probes")+60])
	}
}

// The play page repairs a damaged loader (removes it from the downloaded
// files, loads from the content server, fails plainly if that copy is
// damaged too), and a next-unit download that did not finish goes to the
// units page, never to the next unit's play page.
func TestPlayPageDamagedLoaderAndFailedNextUnit(t *testing.T) {
	body := renderPlayPage(t, memberPlayData())
	for _, want := range []string{
		`if (typeof window.createUnityInstance !== 'function') { onDamagedLoader(); return; }`,
		`c.delete(cachedLoaderKey)`,
		`if (retryFromContentServer()) return;`,
		`The game loader failed to load — it arrived damaged`,
		`reason: 'loader-damaged'`,
		`window.location.href = '/missionhydrosci/units';`,
		`mgr.isCached(nextUnit).then(function(state) {`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("play page lacks %q", want)
		}
	}
	if strings.Contains(body, `setTimeout(function() { navigateToNext(nextUnit); }, 1500);`) {
		t.Error("a failed next-unit download still opens the next unit's play page")
	}
}

// One game window per device (plan B.4): a student's play page names its
// window and answers the units page's 'who' and 'switch'; a device-test
// play page keeps out of it.
func TestPlayPageOneGameWindow(t *testing.T) {
	body := renderPlayPage(t, memberPlayData())
	for _, want := range []string{
		`var memberWindow = true;`,
		`window.name = 'mhs-game'`,
		`if (m.kind === 'who') {`,
		`if (m.kind === 'switch') {`,
		`m.to !== devState.pageId`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("play page lacks %q", want)
		}
	}
	data := memberPlayData()
	data.DeviceTest = true
	data.DeviceTestID = "ffffffff0123456789abcdef"
	data.DeviceTestBase = "/missionhydrosci/devicetest/run/ffffffff0123456789abcdef"
	if body := renderPlayPage(t, data); !strings.Contains(body, `var memberWindow = false;`) {
		t.Error("a device-test play page must not take part in the one-window switch")
	}
}

// The units page opens Play in the named game window (taken at the click),
// asks the game windows with 'who', never with 'hello' (which play pages
// count as another game window), and never opens a '_blank' window.
func TestUnitsPagePlaysInOneGameWindow(t *testing.T) {
	bootCeremonyTemplates(t)
	rec := httptest.NewRecorder()
	templates.Render(rec, httptest.NewRequest("GET", "/missionhydrosci/units", nil), "missionhydrosci_units", UnitsData{
		BaseVM: viewdata.BaseVM{Title: "Mission HydroSci"}, CurrentUnit: "unit1",
	})
	body := rec.Body.String()
	for _, want := range []string{
		`var GAME_WINDOW = 'mhs-game';`,
		`win = window.open('', GAME_WINDOW);`,
		`ch.postMessage({ kind: 'who'`,
		`kind: 'switch'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("units page lacks %q", want)
		}
	}
	if strings.Contains(body, `window.open(link.href, '_blank')`) {
		t.Error("units page still opens a new window per Play")
	}
	if strings.Contains(body, `kind: 'hello'`) {
		t.Error("units page must ask with 'who', not 'hello'")
	}
}

// The discard notice (plan B.4): shown when the page was reloaded after a
// discard, with the student's line and the teacher's steps; it gives way to
// the other bars and hides itself after the game renders.
func TestPlayPageDiscardNotice(t *testing.T) {
	body := renderPlayPage(t, memberPlayData())
	for _, want := range []string{
		`id="mhs-discard-notice"`,
		`Chrome closed the game to free up memory while another window or tab was in front.`,
		`Press START to continue from your last save point.`,
		`Exceptions to tab discarding`,
		`setTimeout(showDiscardNotice, 0);`,
		`if (dcEl && el !== dcEl && !dcEl.hidden) barHide(dcEl);`,
		`hideDiscardNoticeSoon();`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("play page lacks %q", want)
		}
	}
}
