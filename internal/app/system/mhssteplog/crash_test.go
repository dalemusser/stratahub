package mhssteplog

import (
	"strings"
	"testing"
)

// The play page tells notices apart itself (so they never mark a launch as
// crashed); its pattern must stay the one IsCrashNotice uses.
func TestPlayPageUsesTheSameNoticePattern(t *testing.T) {
	play := readSource(t, "..", "..", "features", "missionhydrosci", "templates", "missionhydrosci_play.gohtml")
	want := "var crashNoticeRe = /" + strings.TrimPrefix(crashNoticeRe.String(), "(?i)") + "/i;"
	if !strings.Contains(play, want) {
		t.Errorf("the play page's notice pattern differs from crashNoticeRe; want %q", want)
	}
}

func TestIsCrashNotice(t *testing.T) {
	tests := []struct {
		typ, msg string
		want     bool
	}{
		{"unhandled_rejection", "A user gesture is required to request Pointer Lock.", true},
		{"unhandled_rejection", "The root document of this element is not valid for pointer lock.", true},
		{"unhandled_rejection", "Permissions check failed", true},
		{"unhandled_rejection", "Too many pointer lock requests", true},
		{"unhandled_rejection", "Document is not focused or fullscreen was denied", true},
		{"unhandled_rejection", "The AudioContext was not allowed to start.", true},
		{"unhandled_rejection", "TypeError: Cannot read properties of undefined", false},
		{"runtime_error", "RuntimeError: memory access out of bounds (pointer lock)", false},
		{"load_error", "Failed to load Unity loader script", false},
		{"load_stalled", "No launch progress after 90 s", false},
	}
	for _, tc := range tests {
		if got := IsCrashNotice(tc.typ, tc.msg); got != tc.want {
			t.Errorf("IsCrashNotice(%q, %q) = %v, want %v", tc.typ, tc.msg, got, tc.want)
		}
	}
}
