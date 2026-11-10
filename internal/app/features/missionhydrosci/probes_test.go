package missionhydrosci

import "testing"

// Each configured service gets its /health (GET) and the /probe route beside
// the endpoint the game posts to; the legacy one-segment path gets no probe
// route, and a malformed endpoint gets nothing.
func TestProbesFromServices(t *testing.T) {
	got := ProbesFromServices(GameServices{
		LogSubmitURL: "https://log.example.org/api/log/submit",
		StateSaveURL: "https://save.example.org/api/state/save/",
	})
	want := []ManifestProbe{
		{Name: "log", URL: "https://log.example.org/health", Post: "https://log.example.org/api/log/probe"},
		{Name: "save", URL: "https://save.example.org/health", Post: "https://save.example.org/api/state/probe"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("probe %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	legacy := ProbesFromServices(GameServices{LogSubmitURL: "https://log.example.org/logs", StateSaveURL: "not a url"})
	if len(legacy) != 1 || legacy[0].Post != "" || legacy[0].URL != "https://log.example.org/health" {
		t.Errorf("legacy = %+v", legacy)
	}
}
