package pack

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/channel"
)

// TestLoad_ChannelMapFromCore confirms the engine baseline combat-channel
// derivation now loads from the core pack YAML (content/core/channel-map)
// rather than only a Go literal, and Build() yields the pre-channel reads.
func TestLoad_ChannelMapFromCore(t *testing.T) {
	regs := loadRealCore(t)

	if regs.ChannelMap.Len() == 0 {
		t.Fatal("core pack registered no channel-map formulas")
	}
	m, err := regs.ChannelMap.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	lookup := func(name string) int { return map[string]int{"hit_mod": 3, "ac": 12}[name] }
	if got := m.Value(channel.Attack, lookup); got != 3 {
		t.Errorf("attack = %d; want 3 (baseline attack=hit_mod)", got)
	}
	if got := m.Value(channel.Defense, lookup); got != 12 {
		t.Errorf("defense = %d; want 12 (baseline defense=ac)", got)
	}
}

// TestLoad_ChannelMapOverride validates the content path end-to-end: two
// packs where the downstream pack overrides one channel's formula, and the
// built Mapping reflects later-wins (override) while the non-overridden
// channel survives from the base pack.
func TestLoad_ChannelMapOverride(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "base", "name: base\ncontent:\n  channel_map:\n    - channel-map/*.yaml\n")
	writeFile(t, filepath.Join(root, "base/channel-map/m.yaml"), "channels:\n  attack: hit_mod\n  defense: ac\n")
	// `over` depends on base ⇒ loads after it, so its defense override wins.
	writePack(t, root, "over", "name: over\ndependencies:\n  base: \"*\"\ncontent:\n  channel_map:\n    - channel-map/*.yaml\n")
	writeFile(t, filepath.Join(root, "over/channel-map/m.yaml"), "channels:\n  defense: ac + 5\n")

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	m, err := regs.ChannelMap.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	lookup := func(name string) int { return map[string]int{"hit_mod": 2, "ac": 10}[name] }
	if got := m.Value(channel.Attack, lookup); got != 2 {
		t.Errorf("attack = %d; want 2 (base, not overridden)", got)
	}
	if got := m.Value(channel.Defense, lookup); got != 15 {
		t.Errorf("defense = %d; want 15 (override ac+5 wins via dep order)", got)
	}
}

func TestLoad_ChannelMapRejectsUnknownChannel(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "bad", "name: bad\ncontent:\n  channel_map:\n    - channel-map/*.yaml\n")
	writeFile(t, filepath.Join(root, "bad/channel-map/m.yaml"), "channels:\n  bogus: hit_mod\n")

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err == nil {
		t.Fatal("Load should reject an unknown combat channel name")
	}
}

func TestLoad_ChannelMapRejectsBadFormula(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "bad", "name: bad\ncontent:\n  channel_map:\n    - channel-map/*.yaml\n")
	writeFile(t, filepath.Join(root, "bad/channel-map/m.yaml"), "channels:\n  defense: \"10 + bogus(\"\n")

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err == nil {
		t.Fatal("Load should reject a malformed channel formula")
	}
}
