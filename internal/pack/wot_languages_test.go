package pack

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The wot pack manifest must DECLARE a languages content-glob and that glob must
// resolve to the authored files. Without the manifest entry the loader silently
// registers zero languages (the files are ignored) — the same trap the
// backgrounds glob test guards (the loader enumerates by manifest, not by
// directory convention).
func TestWoTPack_DeclaresLanguagesGlob(t *testing.T) {
	content := repoContentDir(t)
	wotDir := filepath.Join(content, "wot")

	var manifest struct {
		Content struct {
			Languages []string `yaml:"languages"`
		} `yaml:"content"`
	}
	b, err := os.ReadFile(filepath.Join(wotDir, "pack.yaml"))
	if err != nil {
		t.Fatalf("read wot pack.yaml: %v", err)
	}
	if err := yaml.Unmarshal(b, &manifest); err != nil {
		t.Fatalf("parse wot pack.yaml: %v", err)
	}
	if len(manifest.Content.Languages) == 0 {
		t.Fatal("wot pack.yaml declares no content.languages glob — the loader will register zero languages")
	}

	seen := map[string]bool{}
	for _, pattern := range manifest.Content.Languages {
		matches, err := filepath.Glob(filepath.Join(wotDir, pattern))
		if err != nil {
			t.Fatalf("glob %q: %v", pattern, err)
		}
		for _, m := range matches {
			seen[m] = true
		}
	}
	if len(seen) == 0 {
		t.Error("wot languages glob resolves to 0 files")
	}
}

type bgLangDoc struct {
	ID             string   `yaml:"id"`
	HomeLanguage   string   `yaml:"home_language"`
	BonusLanguages []string `yaml:"bonus_languages"`
}

// Every WoT background's home_language + bonus_languages must reference a real
// language id. The loader qualifies these at grant time and fail-softs an
// unknown id (silently dropping the grant), so a typo would vanish a home
// tongue rather than error — this guard catches it at test time. Also asserts
// every human background declares a home language (languages.md §3).
func TestWoTBackgrounds_LanguageReferentialIntegrity(t *testing.T) {
	content := repoContentDir(t)
	languages := idSet(t, content, "languages", "core", "wot")

	files, err := filepath.Glob(filepath.Join(content, "wot", "backgrounds", "*.yaml"))
	if err != nil {
		t.Fatalf("glob wot backgrounds: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no wot background files found")
	}

	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var bg bgLangDoc
		if err := yaml.Unmarshal(b, &bg); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		if bg.HomeLanguage == "" {
			t.Errorf("%s: declares no home_language (every human background has a home tongue — languages.md §3)", bg.ID)
		} else if !languages[bg.HomeLanguage] {
			t.Errorf("%s: home_language %q does not resolve to a registered language", bg.ID, bg.HomeLanguage)
		}
		for _, bl := range bg.BonusLanguages {
			if !languages[bl] {
				t.Errorf("%s: bonus_language %q does not resolve to a registered language", bg.ID, bl)
			}
		}
	}
}

// Every WoT language file must declare a comprehension family, and all the
// dialects of the Common tongue must share one family (languages.md §1 — the
// dialects are mutually intelligible; the family is the comprehension unit a
// future gating check keys on). A dialect that drifted out of the `common`
// family would silently become its own barrier.
func TestWoTLanguages_FamilyConsistency(t *testing.T) {
	content := repoContentDir(t)
	files, err := filepath.Glob(filepath.Join(content, "wot", "languages", "*.yaml"))
	if err != nil {
		t.Fatalf("glob wot languages: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no wot language files found")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var doc struct {
			ID     string `yaml:"id"`
			Family string `yaml:"family"`
		}
		if err := yaml.Unmarshal(b, &doc); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		if doc.Family == "" {
			t.Errorf("%s: declares no family (languages.md §1 — the comprehension group)", doc.ID)
		}
		// Every "common-*" dialect must sit in the shared `common` family.
		if len(doc.ID) >= 7 && doc.ID[:7] == "common-" && doc.Family != "common" {
			t.Errorf("%s: Common dialect has family %q, want common (the dialects are one comprehension family)", doc.ID, doc.Family)
		}
	}
}
