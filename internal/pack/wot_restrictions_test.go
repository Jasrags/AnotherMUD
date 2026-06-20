package pack

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// weaponCategorySet collects every weapon_category authored across the given
// packs' item files — the universe of categories a restriction can match.
func weaponCategorySet(t *testing.T, content string, packs ...string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, p := range packs {
		matches, err := filepath.Glob(filepath.Join(content, p, "items", "*.yaml"))
		if err != nil {
			t.Fatalf("glob %s/items: %v", p, err)
		}
		for _, m := range matches {
			var doc struct {
				WeaponCategory string `yaml:"weapon_category"`
			}
			b, err := os.ReadFile(m)
			if err != nil {
				t.Fatalf("read %s: %v", m, err)
			}
			if err := yaml.Unmarshal(b, &doc); err != nil {
				t.Fatalf("parse %s: %v", m, err)
			}
			if doc.WeaponCategory != "" {
				out[doc.WeaponCategory] = true
			}
		}
	}
	return out
}

type bgRestrictDoc struct {
	ID                 string   `yaml:"id"`
	WeaponRestrictions []string `yaml:"weapon_restrictions"`
}

// Every WoT background's weapon_restrictions must name a REAL weapon category
// authored in content. A restriction that names no existing category (a typo
// like "sword" — which is not itself a category) silently never fires, leaving
// the taboo unenforced. The equip gate matches against the live weapon_category,
// so this guard keeps the restriction honest at test time.
func TestWoTBackgrounds_WeaponRestrictionsResolve(t *testing.T) {
	content := repoContentDir(t)
	categories := weaponCategorySet(t, content, "core", "wot")
	if len(categories) == 0 {
		t.Fatal("no weapon categories found in content — the guard cannot run")
	}

	files, err := filepath.Glob(filepath.Join(content, "wot", "backgrounds", "*.yaml"))
	if err != nil {
		t.Fatalf("glob wot backgrounds: %v", err)
	}
	sawRestriction := false
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var bg bgRestrictDoc
		if err := yaml.Unmarshal(b, &bg); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, cat := range bg.WeaponRestrictions {
			sawRestriction = true
			if !categories[cat] {
				t.Errorf("%s: weapon_restriction %q is not a real weapon_category — the taboo would never fire", bg.ID, cat)
			}
		}
	}
	// The Aiel sword taboo is the marquee restriction; if NO background declares
	// one, the content regressed (the feature has no live consumer).
	if !sawRestriction {
		t.Error("no WoT background declares a weapon_restriction — expected at least the Aiel sword taboo")
	}
}
