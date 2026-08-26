package memberstatuscfg_test

import (
	"testing"

	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
)

const sample = `{
  "tab_title": "Surveys",
  "items": [
    {"id": "pre",  "title": "Pre-Survey",     "short_name": "Pre",  "api_names": ["Pre", "Pre Survey"]},
    {"id": "mhs",  "title": "MHS Engagement", "short_name": "MHS",  "api_names": ["MHS Engagement"]},
    {"id": "post", "title": "Post-Survey",    "short_name": "Post", "api_names": []}
  ]
}`

func TestParse_ResolvesNamesCaseInsensitively(t *testing.T) {
	cfg, err := memberstatuscfg.Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.TabTitle != "Surveys" || len(cfg.Items) != 3 {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	cases := map[string]string{
		"Pre":            "pre",
		"pre":            "pre",
		"  PRE  ":        "pre",
		"Pre Survey":     "pre",
		"pre   survey":   "pre", // internal whitespace collapsed
		"Pre-Survey":     "pre", // title
		"MHS Engagement": "mhs",
		"mhs engagement": "mhs",
		"mhs":            "mhs", // id
		"Post-Survey":    "post",
		"post":           "post",
	}
	for name, wantID := range cases {
		item, ok := cfg.Resolve(name)
		if !ok || item.ID != wantID {
			t.Errorf("Resolve(%q): got (%q, %v), want %q", name, item.ID, ok, wantID)
		}
		key, _, known := cfg.KeyFor(name)
		if !known || key != wantID {
			t.Errorf("KeyFor(%q): got (%q, %v), want %q", name, key, known, wantID)
		}
	}

	if _, ok := cfg.Resolve("Mid Survey"); ok {
		t.Error("unknown name resolved")
	}
	key, item, known := cfg.KeyFor("  Mid   Survey ")
	if known || item.ID != "" {
		t.Errorf("KeyFor unknown: known=%v item=%+v", known, item)
	}
	if key != "name:mid survey" {
		t.Errorf("unknown key: got %q, want %q", key, "name:mid survey")
	}
	if memberstatuscfg.UnknownKey("Mid Survey") != key {
		t.Error("UnknownKey should be stable for the same folded name")
	}
}

func TestParse_FindAndAPINames(t *testing.T) {
	cfg, err := memberstatuscfg.Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if item, ok := cfg.Find("mhs"); !ok || item.Title != "MHS Engagement" {
		t.Errorf("Find(mhs): %+v %v", item, ok)
	}
	if _, ok := cfg.Find("nope"); ok {
		t.Error("Find(nope) should fail")
	}
	got := cfg.APINames()
	want := []string{"Pre", "MHS Engagement", "Post-Survey"} // falls back to title when api_names is empty
	if len(got) != len(want) {
		t.Fatalf("APINames: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("APINames[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParse_Validation(t *testing.T) {
	bad := map[string]string{
		"missing id":     `{"items":[{"title":"X"}]}`,
		"missing title":  `{"items":[{"id":"x"}]}`,
		"duplicate id":   `{"items":[{"id":"x","title":"A"},{"id":"x","title":"B"}]}`,
		"name collision": `{"items":[{"id":"a","title":"A","api_names":["Same"]},{"id":"b","title":"B","api_names":["same"]}]}`,
		"reserved id":    `{"items":[{"id":"name:x","title":"X"}]}`,
		"not json":       `{"items": [`,
	}
	for name, src := range bad {
		if _, err := memberstatuscfg.Parse([]byte(src)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLoad_EmbeddedFile(t *testing.T) {
	cfg, err := memberstatuscfg.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Items) == 0 {
		t.Fatal("embedded config has no items")
	}
	// The four provider names Abt sends today must all resolve.
	for _, name := range []string{"Pre", "MHS Engagement", "EWS Engagement", "Post"} {
		if _, ok := cfg.Resolve(name); !ok {
			t.Errorf("embedded config does not recognize %q", name)
		}
	}
	// Nil-safe accessors.
	var nilCfg *memberstatuscfg.Config
	if _, ok := nilCfg.Resolve("Pre"); ok {
		t.Error("nil config resolved a name")
	}
	if nilCfg.APINames() != nil {
		t.Error("nil config returned names")
	}
}
