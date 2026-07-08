package corpse

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// decayHarness wires a Service over real entities + bus for sweep tests.
type decayHarness struct {
	svc       *Service
	store     *entities.Store
	contents  *entities.Contents
	placement *entities.Placement
	bus       *eventbus.Bus
}

func newDecayHarness() *decayHarness {
	h := &decayHarness{
		store:     entities.NewStore(),
		contents:  entities.NewContents(),
		placement: entities.NewPlacement(),
		bus:       eventbus.New(),
	}
	h.svc = New(Config{Store: h.store, Contents: h.contents, Placement: h.placement, Bus: h.bus})
	return h
}

// placeCorpse mints a corpse at createdTick with coins + item contents.
func (h *decayHarness) placeCorpse(t *testing.T, createdTick uint64, coins int, items int) *entities.ItemInstance {
	t.Helper()
	cor, err := h.store.SpawnContainer("the corpse of a goblin",
		[]string{TagCorpse, TagNoGet, TagNoPut},
		[]string{"corpse"},
		map[string]any{PropCreatedTick: createdTick, PropCoins: coins, PropOwners: []string{}})
	if err != nil {
		t.Fatalf("SpawnContainer: %v", err)
	}
	h.placement.Place(cor.ID(), "core:room")
	for range items {
		it, err := h.store.Spawn(&item.Template{ID: "core:thing", Name: "a thing", Type: "misc"})
		if err != nil {
			t.Fatalf("Spawn: %v", err)
		}
		h.contents.Put(cor.ID(), it.ID())
	}
	// Surface the corpse tag to the read index the sweep queries.
	h.store.SwapTagIndex()
	return cor
}

func TestDecaySweep_RemovesExpiredCorpseAndContents(t *testing.T) {
	h := newDecayHarness()
	cor := h.placeCorpse(t, 100, 7, 2)
	itemIDs := h.contents.In(cor.ID())

	var decayed *eventbus.CorpseDecayed
	h.bus.Subscribe(eventbus.EventCorpseDecayed, func(_ context.Context, ev eventbus.Event) {
		e := ev.(eventbus.CorpseDecayed)
		decayed = &e
	})

	// now=300, created=100, lifetime=100 → elapsed (200 >= 100).
	n := h.svc.DecaySweep(context.Background(), 300, 100)
	if n != 1 {
		t.Fatalf("decayed = %d, want 1", n)
	}
	if _, ok := h.store.GetByID(cor.ID()); ok {
		t.Error("expired corpse should be untracked")
	}
	if got, ok := h.placement.RoomOf(cor.ID()); ok {
		t.Errorf("expired corpse still placed in %q", got)
	}
	// Contents destroyed (untracked), not spilled to the room.
	for _, id := range itemIDs {
		if _, ok := h.store.GetByID(id); ok {
			t.Errorf("content %s should be destroyed", id)
		}
		if _, ok := h.placement.RoomOf(id); ok {
			t.Errorf("content %s should not spill to the room", id)
		}
	}
	if decayed == nil || decayed.ItemCount != 2 || decayed.Coins != 7 {
		t.Errorf("corpse.decayed = %+v", decayed)
	}
}

func TestDecaySweep_KeepsUnexpiredCorpse(t *testing.T) {
	h := newDecayHarness()
	cor := h.placeCorpse(t, 100, 0, 1)
	// now=150, created=100, lifetime=100 → 50 < 100, not expired.
	if n := h.svc.DecaySweep(context.Background(), 150, 100); n != 0 {
		t.Fatalf("decayed = %d, want 0", n)
	}
	if _, ok := h.store.GetByID(cor.ID()); !ok {
		t.Error("unexpired corpse should remain")
	}
}

func TestDecaySweep_ClockSkewNotExpired(t *testing.T) {
	h := newDecayHarness()
	cor := h.placeCorpse(t, 500, 0, 0)
	// now < created (skew) → must not be treated as expired.
	if n := h.svc.DecaySweep(context.Background(), 100, 50); n != 0 {
		t.Fatalf("decayed = %d, want 0 (clock skew)", n)
	}
	if _, ok := h.store.GetByID(cor.ID()); !ok {
		t.Error("corpse should remain under clock skew")
	}
}

func TestDecaySweep_NoCorpses(t *testing.T) {
	h := newDecayHarness()
	if n := h.svc.DecaySweep(context.Background(), 1000, 10); n != 0 {
		t.Fatalf("decayed = %d, want 0", n)
	}
}
