package pack

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// loadRealCore loads the real on-disk content packs into a fresh Registries.
func loadRealCore(t *testing.T) *Registries {
	t.Helper()
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("register engine baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("register engine baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load core: %v", err)
	}
	return regs
}

// TestLoad_BaselineChannelsFromPack confirms the engine baseline `ooc`
// channel now loads from the core pack YAML (M0.3) rather than a Go literal,
// namespace-qualified and with its declared fields.
func TestLoad_BaselineChannelsFromPack(t *testing.T) {
	regs := loadRealCore(t)

	ch, ok := regs.Channels.Get("tapestry-core:ooc")
	if !ok {
		t.Fatalf("baseline channel tapestry-core:ooc not registered (have %d)", regs.Channels.Len())
	}
	if ch.DisplayName != "ooc" {
		t.Errorf("ooc DisplayName = %q, want ooc", ch.DisplayName)
	}
	if ch.Kind != chat.KindPublic {
		t.Errorf("ooc Kind = %q, want public", ch.Kind)
	}
	if !ch.DefaultOn || !ch.Persisted || ch.BufferCap != 100 {
		t.Errorf("ooc flags = {DefaultOn:%v Persisted:%v BufferCap:%d}, want {true true 100}",
			ch.DefaultOn, ch.Persisted, ch.BufferCap)
	}
	// Verb lookup is by display-name.
	if _, ok := regs.Channels.ByDisplayName("ooc"); !ok {
		t.Error("ooc not resolvable by display-name")
	}
}

// TestLoad_BaselineEmotesFromPack confirms the seven baseline emotes load
// from the core pack YAML (M0.3), namespace-qualified, verb-addressable, and
// with both targeted and no-target views populated.
func TestLoad_BaselineEmotesFromPack(t *testing.T) {
	regs := loadRealCore(t)

	want := []string{"smile", "nod", "wave", "bow", "grin", "shrug", "laugh"}
	for _, verb := range want {
		id := "tapestry-core:" + verb
		e, ok := regs.Emotes.Get(id)
		if !ok {
			t.Errorf("baseline emote %s not registered", id)
			continue
		}
		if e.DisplayName != verb {
			t.Errorf("%s DisplayName = %q, want %q", id, e.DisplayName, verb)
		}
		if e.NoTarget.ActorView == "" || e.NoTarget.RoomView == "" {
			t.Errorf("%s missing no-target views: %+v", id, e.NoTarget)
		}
		if e.Targeted.ActorView == "" || e.Targeted.TargetView == "" || e.Targeted.RoomView == "" {
			t.Errorf("%s missing targeted views: %+v", id, e.Targeted)
		}
		if _, ok := regs.Emotes.ByVerb(verb); !ok {
			t.Errorf("%s not resolvable by verb %q", id, verb)
		}
	}
	if regs.Emotes.Len() < len(want) {
		t.Errorf("emote registry has %d, want >= %d", regs.Emotes.Len(), len(want))
	}
}
