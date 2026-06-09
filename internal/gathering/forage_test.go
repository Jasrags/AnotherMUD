package gathering

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// fakeGatherer is an in-memory Gatherer.
type fakeGatherer struct {
	id      string
	inv     []entities.EntityID
	readyAt uint64
}

func (g *fakeGatherer) PlayerID() string                    { return g.id }
func (g *fakeGatherer) ID() string                          { return g.id }
func (g *fakeGatherer) AddToInventory(id entities.EntityID) { g.inv = append(g.inv, id) }
func (g *fakeGatherer) ForageReadyAt() uint64               { return g.readyAt }
func (g *fakeGatherer) SetForageReadyAt(t uint64)           { g.readyAt = t }

func forageFixture(t *testing.T, cooldown uint64) (*Service, *entities.Store, *fakeGatherer) {
	t.Helper()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "core:wild-herb", Name: "a sprig of wild herb", Type: "item"})
	store := entities.NewStore()
	cfg := DefaultConfig()
	cfg.ForageCooldownTicks = cooldown
	s := NewService(coreLadder(), nil, fixedRoller{v: 0}, cfg, nil, store, tpls)
	return s, store, &fakeGatherer{id: "p1"}
}

func herbTable() *ForageTable {
	return &ForageTable{
		ID: "core:forest-forage", Richness: 50, Ceiling: "uncommon",
		Entries: []ForageEntry{{Item: "core:wild-herb", Weight: 1, Qty: 1}},
	}
}

func TestForage_YieldsItemStampsRaritySetsCooldown(t *testing.T) {
	s, store, g := forageFixture(t, 100)
	res := s.Forage(context.Background(), g, herbTable(), 1000)

	if res.Outcome != ForageOK {
		t.Fatalf("outcome = %v, want ForageOK", res.Outcome)
	}
	if res.ItemID != "core:wild-herb" || res.ItemName != "a sprig of wild herb" {
		t.Errorf("result item = %q/%q", res.ItemID, res.ItemName)
	}
	if len(g.inv) != 1 {
		t.Fatalf("inventory = %v, want the foraged herb", g.inv)
	}
	e, ok := store.GetByID(g.inv[0])
	if !ok {
		t.Fatal("foraged item not tracked in store")
	}
	if res.QualityKey != "" {
		if v, ok := e.(*entities.ItemInstance).Property("rarity"); !ok || v != res.QualityKey {
			t.Errorf("rarity stamp = %v, want %q", v, res.QualityKey)
		}
	}
	// Cooldown started at now+cooldown.
	if g.readyAt != 1100 {
		t.Errorf("cooldown readyAt = %d, want 1100", g.readyAt)
	}
}

func TestForage_CoolingDownRefusesAsWait(t *testing.T) {
	s, _, g := forageFixture(t, 100)
	g.readyAt = 1200 // still cooling down at now=1000
	res := s.Forage(context.Background(), g, herbTable(), 1000)
	if res.Outcome != ForageCoolingDown {
		t.Fatalf("outcome = %v, want ForageCoolingDown", res.Outcome)
	}
	if res.RemainingTicks != 200 {
		t.Errorf("remaining = %d, want 200", res.RemainingTicks)
	}
	if len(g.inv) != 0 {
		t.Error("a cooling-down forage must not yield")
	}
}

func TestForage_MissingTemplateIsUndefinedNoCooldown(t *testing.T) {
	s, _, g := forageFixture(t, 100)
	tbl := herbTable()
	tbl.Entries[0].Item = "core:does-not-exist"
	res := s.Forage(context.Background(), g, tbl, 1000)
	if res.Outcome != ForageOutputUndefined {
		t.Fatalf("outcome = %v, want ForageOutputUndefined", res.Outcome)
	}
	if g.readyAt != 0 {
		t.Error("a failed forage must NOT start the cooldown")
	}
	if len(g.inv) != 0 {
		t.Error("a failed forage must not yield")
	}
}
