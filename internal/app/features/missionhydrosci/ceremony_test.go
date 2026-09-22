package missionhydrosci

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

func TestBuildEAScores(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	complete := func(ago time.Duration) models.MHSUserProgress {
		return models.MHSUserProgress{CurrentUnit: "complete",
			CompletedUnits: []string{"unit1", "unit2", "unit3", "unit4", "unit5"}, UpdatedAt: now.Add(-ago)}
	}

	t.Run("no grades right after finishing → pending, empty items", func(t *testing.T) {
		r := buildEAScores("abc", nil, complete(time.Minute), now)
		if r.Status != "pending" || len(r.Items) != 0 || r.Stars != nil {
			t.Fatalf("got %+v", r)
		}
		if r.CurrentUnit != "complete" || len(r.CompletedUnits) != 5 || r.Game != "mhs" || r.UserID != "abc" {
			t.Fatalf("progress fields not carried: %+v", r)
		}
	})

	t.Run("final point graded → ready, items from finished attempts, stars pass through", func(t *testing.T) {
		doc := &eaGradeDoc{
			LastUpdated: now,
			Grades: map[string][]eaGradeItem{
				"u2p2": {{Status: "passed", EAScores: map[string]eaScore{"U2.C2": {Score: 1, Max: 1}}}},
				"u2p3": {{Status: "flagged", EAScores: map[string]eaScore{"U2.C3": {Score: 0.5, Max: 3}}}},
				"u3p5": {
					{Status: "passed", EAScores: map[string]eaScore{"U3.C5": {Score: 2.5, Max: 4}}},
					{Status: "active"}, // replay in progress: the finished attempt still counts
				},
				"u5p1": {{Status: "active"}}, // never finished: contributes nothing
				"u5p4": {{Status: "passed", EAScores: map[string]eaScore{"U5.C4": {Score: 1.5, Max: 1.5}}}},
			},
			EAStars: map[string]int{"unit2": 2, "unit3": 3},
		}
		r := buildEAScores("abc", doc, complete(time.Minute), now)
		if r.Status != "ready" {
			t.Fatalf("status = %q", r.Status)
		}
		want := map[string]float64{"U2.C2": 1, "U2.C3": 0.5, "U3.C5": 2.5, "U5.C4": 1.5}
		if len(r.Items) != len(want) {
			t.Fatalf("items = %v", r.Items)
		}
		for k, v := range want {
			if r.Items[k].Score != v {
				t.Errorf("%s = %v, want %v", k, r.Items[k].Score, v)
			}
		}
		if r.Stars["unit2"] != 2 || r.Stars["unit3"] != 3 || len(r.Stars) != 2 {
			t.Errorf("stars = %v", r.Stars)
		}
	})

	t.Run("final grade missing long after completion → ready (no endless polling)", func(t *testing.T) {
		doc := &eaGradeDoc{LastUpdated: now.Add(-2 * time.Hour), Grades: map[string][]eaGradeItem{
			"u2p2": {{Status: "passed", EAScores: map[string]eaScore{"U2.C2": {Score: 1, Max: 1}}}},
		}}
		r := buildEAScores("abc", doc, complete(2*time.Hour), now)
		if r.Status != "ready" || r.Items["U2.C2"].Score != 1 {
			t.Fatalf("got %+v", r)
		}
	})

	t.Run("a fresh grade write keeps pending alive even if completion was earlier", func(t *testing.T) {
		doc := &eaGradeDoc{LastUpdated: now.Add(-time.Minute), Grades: map[string][]eaGradeItem{}}
		if r := buildEAScores("abc", doc, complete(2*time.Hour), now); r.Status != "pending" {
			t.Fatalf("status = %q", r.Status)
		}
	})

	t.Run("mid-game student (staff preview) is never pending", func(t *testing.T) {
		p := models.MHSUserProgress{CurrentUnit: "unit3", CompletedUnits: []string{"unit1", "unit2"}, UpdatedAt: now}
		r := buildEAScores("abc", nil, p, now)
		if r.Status != "ready" || len(r.CompletedUnits) != 2 || r.CurrentUnit != "unit3" {
			t.Fatalf("got %+v", r)
		}
	})

	t.Run("nil completed units becomes an empty list, not null", func(t *testing.T) {
		r := buildEAScores("abc", nil, models.MHSUserProgress{CurrentUnit: "unit1"}, now)
		if r.CompletedUnits == nil || len(r.CompletedUnits) != 0 {
			t.Fatalf("completedUnits = %v", r.CompletedUnits)
		}
	})
}

func TestCeremonyBaseAndURL(t *testing.T) {
	if got := ceremonyBase("0.1.8"); got != "/missionhydrosci/content/end/v0.1.8/" {
		t.Fatalf("ceremonyBase = %q", got)
	}
	if got := ceremonyURLFor(ContentManifest{}); got != "" {
		t.Fatalf("no ceremony → %q", got)
	}
	if got := ceremonyURLFor(ContentManifest{Ceremony: &ContentManifestCeremony{Version: "0.1.8"}}); got != CeremonyPath {
		t.Fatalf("with ceremony → %q", got)
	}
}
