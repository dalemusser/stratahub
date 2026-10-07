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
