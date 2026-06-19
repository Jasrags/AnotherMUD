package pack

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The wot pack manifest must DECLARE a backgrounds content-glob and that glob
// must resolve to the 11 authored files. Without the manifest entry the loader
// silently registers zero backgrounds (the files are ignored) — a regression
// the file-only integrity test below cannot see, since it reads the directory
// directly rather than going through pack.yaml.
func TestWoTPack_DeclaresBackgroundsGlob(t *testing.T) {
	content := repoContentDir(t)
	wotDir := filepath.Join(content, "wot")

	var manifest struct {
		Content struct {
			Backgrounds []string `yaml:"backgrounds"`
		} `yaml:"content"`
	}
	b, err := os.ReadFile(filepath.Join(wotDir, "pack.yaml"))
	if err != nil {
		t.Fatalf("read wot pack.yaml: %v", err)
	}
	if err := yaml.Unmarshal(b, &manifest); err != nil {
		t.Fatalf("parse wot pack.yaml: %v", err)
	}
	if len(manifest.Content.Backgrounds) == 0 {
		t.Fatal("wot pack.yaml declares no content.backgrounds glob — the loader will register zero backgrounds")
	}

	seen := map[string]bool{}
	for _, pattern := range manifest.Content.Backgrounds {
		matches, err := filepath.Glob(filepath.Join(wotDir, pattern))
		if err != nil {
			t.Fatalf("glob %q: %v", pattern, err)
		}
		for _, m := range matches {
			seen[m] = true
		}
	}
	if len(seen) != 11 {
		t.Errorf("wot backgrounds glob resolves to %d files, want 11", len(seen))
	}
}

// repoContentDir resolves the real content tree relative to this test's
// package directory (internal/pack → ../../content).
func repoContentDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "content")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("content dir not found at %s: %v", dir, err)
	}
	return dir
}

// idSet globs <content>/<pack>/<kind>/*.yaml for every pack and collects the
// top-level `id:` of each file — the universe of resolvable ids for that kind.
func idSet(t *testing.T, content, kind string, packs ...string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, p := range packs {
		matches, err := filepath.Glob(filepath.Join(content, p, kind, "*.yaml"))
		if err != nil {
			t.Fatalf("glob %s/%s: %v", p, kind, err)
		}
		for _, m := range matches {
			var doc struct {
				ID string `yaml:"id"`
			}
			b, err := os.ReadFile(m)
			if err != nil {
				t.Fatalf("read %s: %v", m, err)
			}
			if err := yaml.Unmarshal(b, &doc); err != nil {
				t.Fatalf("parse %s: %v", m, err)
			}
			if doc.ID != "" {
				out[doc.ID] = true
			}
		}
	}
	return out
}

type bgDoc struct {
	ID     string `yaml:"id"`
	Skills []struct {
		Ability string `yaml:"ability"`
	} `yaml:"skills"`
	Items             []string   `yaml:"items"`
	Feats             []string   `yaml:"feats"`
	FeatOptions       []string   `yaml:"feat_options"`
	EquipmentPackages [][]string `yaml:"equipment_packages"`
}

// The thin-slice WoT human backgrounds must reference only skills/items/feats
// that actually exist — the loader fail-softs unknown ids at grant time, so a
// typo would silently drop a grant rather than error. This guard catches that
// at test time. Also asserts the full set of 11 human homelands is present.
func TestWoTBackgrounds_ReferentialIntegrity(t *testing.T) {
	content := repoContentDir(t)

	// Resolvable id universes. Backgrounds may reference core or wot content
	// (wot depends on core), so both packs contribute.
	abilities := idSet(t, content, "abilities", "core", "wot")
	items := idSet(t, content, "items", "core", "wot")
	feats := idSet(t, content, "feats", "core", "wot")

	files, err := filepath.Glob(filepath.Join(content, "wot", "backgrounds", "*.yaml"))
	if err != nil {
		t.Fatalf("glob wot backgrounds: %v", err)
	}

	wantIDs := map[string]bool{
		"aiel": true, "athaan-miere": true, "borderlander": true,
		"cairhienin": true, "domani": true, "ebou-dari": true,
		"illianer": true, "midlander": true, "tairen": true,
		"tar-valoner": true, "taraboner": true,
	}
	gotIDs := map[string]bool{}

	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var bg bgDoc
		if err := yaml.Unmarshal(b, &bg); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		gotIDs[bg.ID] = true

		for _, s := range bg.Skills {
			if !abilities[s.Ability] {
				t.Errorf("%s: skill ability %q does not exist", bg.ID, s.Ability)
			}
		}
		for _, it := range bg.Items {
			if !items[it] {
				t.Errorf("%s: item %q does not exist", bg.ID, it)
			}
		}
		for _, ft := range bg.Feats {
			if !feats[ft] {
				t.Errorf("%s: feat %q does not exist", bg.ID, ft)
			}
		}
		// The pick-one chooser fields: every feat option + every item in every
		// equipment package must resolve too (backgrounds §2 — the chooser).
		for _, ft := range bg.FeatOptions {
			if !feats[ft] {
				t.Errorf("%s: feat_option %q does not exist", bg.ID, ft)
			}
		}
		for pi, pkg := range bg.EquipmentPackages {
			for _, it := range pkg {
				if !items[it] {
					t.Errorf("%s: equipment_packages[%d] item %q does not exist", bg.ID, pi, it)
				}
			}
		}
		// Backgrounds expansion #5: every human background offers a pick-one
		// starting equipment choice (Table 2-1). This guards the re-point so a
		// future edit can't silently regress a background to flavor-only.
		if len(bg.EquipmentPackages) == 0 {
			t.Errorf("%s: declares no equipment_packages (expected the pick-one starting kit)", bg.ID)
		}
	}

	for id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("missing expected WoT human background %q", id)
		}
	}
	for id := range gotIDs {
		if !wantIDs[id] {
			t.Errorf("unexpected WoT background %q (update this test if intended)", id)
		}
	}
}
