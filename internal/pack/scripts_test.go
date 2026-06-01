package pack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/scripting"
)

// minimalCorePackWithScripts writes a manifest declaring a scripts
// glob plus two sample Lua files, returning the pack root.
func minimalCorePackWithScripts(t *testing.T, body1, body2 string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  scripts: [scripts/*.lua]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `
id: a
area: town
name: Room A
`)
	writeFile(t, filepath.Join(pack, "scripts/first.lua"), body1)
	writeFile(t, filepath.Join(pack, "scripts/second.lua"), body2)
	return root
}

func TestLoad_DiscoversScripts(t *testing.T) {
	root := minimalCorePackWithScripts(t,
		`local x = 1`,
		`local y = 2`,
	)
	regs := NewRegistries()
	engine := scripting.New(scripting.Options{})
	if err := Load(context.Background(), root, nil, regs, nil, nil, engine); err != nil {
		t.Fatalf("Load: %v", err)
	}
	entries := regs.Scripts.All()
	if len(entries) != 2 {
		t.Fatalf("registered scripts = %d, want 2", len(entries))
	}
	// Both entries must carry the pack namespace and the relative
	// path under the pack dir, not the absolute filesystem path.
	for _, e := range entries {
		if e.PackID != "tapestry-core" {
			t.Errorf("entry %q PackID = %q, want tapestry-core", e.Path, e.PackID)
		}
		if !strings.HasPrefix(e.Path, "scripts/") {
			t.Errorf("entry Path = %q, want scripts/ prefix", e.Path)
		}
		if !strings.HasSuffix(e.Path, ".lua") {
			t.Errorf("entry Path = %q, want .lua suffix", e.Path)
		}
	}
	// All() returns LoadOrder-sorted, then Path-lexicographic.
	if entries[0].Path > entries[1].Path {
		t.Errorf("All() not sorted by Path: [%q, %q]", entries[0].Path, entries[1].Path)
	}
}

func TestLoad_ScriptSyntaxError_AttributesPackAndPath(t *testing.T) {
	root := minimalCorePackWithScripts(t,
		`local x = 1 +`, // syntax error
		`local y = 2`,
	)
	regs := NewRegistries()
	engine := scripting.New(scripting.Options{})
	err := Load(context.Background(), root, nil, regs, nil, nil, engine)
	if err == nil {
		t.Fatal("expected Load to fail on broken script")
	}
	// The underlying *scripting.Error must surface so an admin can
	// see which file blew up.
	var se *scripting.Error
	if !errors.As(err, &se) {
		t.Fatalf("expected *scripting.Error in chain, got %v", err)
	}
	if se.PackID != "tapestry-core" {
		t.Errorf("PackID = %q, want tapestry-core", se.PackID)
	}
	if se.ScriptPath != "scripts/first.lua" {
		t.Errorf("ScriptPath = %q, want scripts/first.lua", se.ScriptPath)
	}
}

func TestLoad_NoScripts_LeavesRegistryEmpty(t *testing.T) {
	// A pack with no scripts manifest entry must still load
	// cleanly and leave Scripts.Len() == 0.
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `
id: a
area: town
name: Room A
`)
	regs := NewRegistries()
	engine := scripting.New(scripting.Options{})
	if err := Load(context.Background(), root, nil, regs, nil, nil, engine); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := regs.Scripts.Len(); got != 0 {
		t.Errorf("Scripts.Len = %d, want 0", got)
	}
}

func TestLoad_NilCompiler_RegistersWithoutCompileCheck(t *testing.T) {
	// With a nil compiler, a syntax-broken script still registers
	// (compile is skipped) — useful in tests that don't want to
	// construct a scripting engine.
	root := minimalCorePackWithScripts(t,
		`local x = 1 +`, // syntax error
		`local y = 2`,
	)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load(nil compiler): %v", err)
	}
	if got := regs.Scripts.Len(); got != 2 {
		t.Errorf("Scripts.Len = %d, want 2 (compile skipped)", got)
	}
}
