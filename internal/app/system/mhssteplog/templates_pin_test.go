package mhssteplog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pages write the sentences this package parses. When one of them
// changes there, this test fails so the parser (and the structured detail it
// prefers) are updated together.
func TestPagesStillWriteTheSentencesWeParse(t *testing.T) {
	delivery := readSource(t, "..", "..", "resources", "assets", "js", "mhs-delivery.js")
	play := readSource(t, "..", "..", "features", "missionhydrosci", "templates", "missionhydrosci_play.gohtml")

	deliveryPhrases := []string{
		"'Requesting ' + unit.title + ' v' + unit.version + ' (' + fmtMB(totalSize) + ' MB)'",
		"': picked up at ' + fmtMB(detail.resumedFrom) + ' MB after a dropped connection'",
		"' drops so far)'",
		"': no data received for '",
		"' s — restarting the download'",
		"'No data for ' + Math.round(detail.waitingMs / 1000) + ' s'",
		"' Retrying ' +",
		"' (attempt ' + attempt + ')'",
		"title + ': ' + pct + '% · ' + fmtMB(detail.downloaded) + ' of ' +",
		"fmtMB(detail.downloadTotal) + ' MB' + rate",
		"' [' + detail.errorClass + (detail.failureReason ? ':' + detail.failureReason : '') + ']'",
		"var msg = title + ': download complete';",
		"msg += ' — ' + fmtMB(total) + ' MB';",
		"msg += ' in ' + fmtDuration(took);",
		"' MB/s)'",
		"': already on this device (downloaded earlier)'",
		"'The browser refused the background download (' + reason +",
		"'Background download stopped receiving data for ' + Math.round(quietMs / 1000) +",
		"'Storage: ' + text +",
		"' MB used of '",
		"' · persisted: '",
		// the structured detail keys the parser prefers
		"{ unit: unitId, mb: fmtMB(detail.downloaded), total_mb: fmtMB(detail.downloadTotal), pct: pct, mbps: mbps }",
		"{ unit: unitId, resumed_mb: fmtMB(detail.resumedFrom), drops: drops || 1 }",
		"error_class: detail.errorClass || '', reason: detail.failureReason || ''",
		"{ unit: unitId, kind: 'bgfetch-refused', reason: reason }",
		"kind: 'bgfetch-frozen'",
	}
	for _, p := range deliveryPhrases {
		if !strings.Contains(delivery, p) {
			t.Errorf("mhs-delivery.js no longer contains %q", p)
		}
	}

	playPhrases := []string{
		"'Crash (' + (type || 'unknown') + ', ' + (phase || crashPhase) + '): '",
		"'Browser notice (' + type + ', ' + (phase || crashPhase) + '): '",
		"'No progress for ' + Math.round(idle / 1000) + ' s while ' + phaseText() + ' — ' + probe",
		"'Still no progress after ' + Math.round(idle / 1000) + ' s — stuck ' + phaseText()",
		"', even straight from the content server'",
		"'Game loader loaded' + loaderFrom",
		"return ' (from the downloaded files)'",
		"return ' (from the network)'",
		"'Unity started in ' + ((Date.now() - launchStartedAt) / 1000).toFixed(1) + ' s'",
		"'Unity failed to start: ' + err.message",
		"'The game loader failed to load",
		"'Game is rendering'",
		// structured detail
		"{ type: type || 'unknown', phase: phase || crashPhase, unit: unitId }",
		"{ unity_s: ((Date.now() - launchStartedAt) / 1000).toFixed(1), unit: unitId }",
	}
	for _, p := range playPhrases {
		if !strings.Contains(play, p) {
			t.Errorf("missionhydrosci_play.gohtml no longer contains %q", p)
		}
	}
}

func readSource(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(b)
}
