package pack

import (
	"context"
	"path/filepath"
	"testing"
)

// TestLoadRacesParsesStatBonuses verifies the `stat_bonuses` RaceFile field
// decodes into progression.Race.StatBonuses — the per-metatype starting-
// attribute grant (sr-m3c-deferred-fixes). Mirrors TestLoadRacesHappyPath.
func TestLoadRacesParsesStatBonuses(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  races: [races/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "races/ork.yaml"), `
id: ork
name: Ork
stat_bonuses:
  body: 3
  strength: 2
  logic: -1
  charisma: 0
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, ok := regs.Races.Get("ork")
	if !ok {
		t.Fatal("race ork not registered")
	}
	if r.StatBonuses["body"] != 3 || r.StatBonuses["strength"] != 2 {
		t.Errorf("StatBonuses = %v, want body:3 strength:2", r.StatBonuses)
	}
	// A negative bonus (metatype penalty) is intentionally permitted, unlike
	// stat_caps — this asserts the negatives-allowed asymmetry stays that way.
	if r.StatBonuses["logic"] != -1 {
		t.Errorf("StatBonuses[logic] = %d, want -1 (negative permitted)", r.StatBonuses["logic"])
	}
	// A zero entry is accepted and a no-op at AdjustBase.
	if r.StatBonuses["charisma"] != 0 {
		t.Errorf("StatBonuses[charisma] = %d, want 0", r.StatBonuses["charisma"])
	}
}

// TestLoadRacesRejectsEmptyStatBonusKey mirrors the stat_caps empty-key guard.
func TestLoadRacesRejectsEmptyStatBonusKey(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  races: [races/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "races/bad.yaml"), `
id: bad
name: Bad
stat_bonuses:
  "": 3
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err == nil {
		t.Fatal("Load succeeded; want error for empty stat_bonuses key")
	}
}
