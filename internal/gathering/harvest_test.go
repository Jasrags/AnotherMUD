package gathering

import (
	"context"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// harvestFixture wires a service, a node entity (3 charges, requires a
// "pick", yields an ore via a forage table), and a gatherer.
func harvestFixture(t *testing.T, requiredTool string) (*Service, *entities.Store, *entities.ItemInstance, *ForageTable, *fakeGatherer) {
	t.Helper()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "core:iron-ore", Name: "a chunk of iron ore", Type: "item"})
	store := entities.NewStore()
	s := NewService(coreLadder(), nil, fixedRoller{v: 0}, DefaultConfig(), nil, store, tpls)

	node, err := store.SpawnContainer("an iron ore vein", []string{NodeTag, NoGetTag}, []string{"vein", "ore"}, map[string]any{
		PropNodeCharges:      3,
		PropNodeYieldTable:   "core:iron-yield",
		PropNodeRequiredTool: requiredTool,
	})
	if err != nil {
		t.Fatalf("spawn node: %v", err)
	}
	yield := &ForageTable{
		ID: "core:iron-yield", Richness: 50, Ceiling: "uncommon",
		Entries: []ForageEntry{{Item: "core:iron-ore", Weight: 1, Qty: 1}},
	}
	return s, store, node, yield, &fakeGatherer{id: "p1"}
}

func TestHarvest_NeedsToolRefuses(t *testing.T) {
	s, _, node, yield, g := harvestFixture(t, "pick")
	res := s.Harvest(context.Background(), g, node, yield)
	if res.Outcome != HarvestNeedsTool || res.RequiredTool != "pick" {
		t.Fatalf("outcome = %v (tool %q), want HarvestNeedsTool/pick", res.Outcome, res.RequiredTool)
	}
	// No yield, no charge consumed.
	if len(g.inv) != 0 {
		t.Error("a tool-refused harvest must not yield")
	}
	if nodeCharges(node) != 3 {
		t.Errorf("charges = %d, want 3 (refused harvest consumes none)", nodeCharges(node))
	}
}

func TestHarvest_WithToolYieldsAndDecrements(t *testing.T) {
	s, store, node, yield, g := harvestFixture(t, "pick")
	// Give the gatherer a pick.
	pick, _ := store.Spawn(&item.Template{ID: "core:pick", Name: "a pickaxe", Type: "item", Tags: []string{"pick"}})
	g.inv = append(g.inv, pick.ID())

	res := s.Harvest(context.Background(), g, node, yield)
	if res.Outcome != HarvestOK || res.ItemID != "core:iron-ore" {
		t.Fatalf("outcome = %v item %q, want HarvestOK/iron-ore", res.Outcome, res.ItemID)
	}
	if res.Depleted {
		t.Error("not depleted after the first of 3 charges")
	}
	if nodeCharges(node) != 2 {
		t.Errorf("charges = %d, want 2 (one consumed)", nodeCharges(node))
	}
	// Yield is in the bag (pick + ore).
	if len(g.inv) != 2 {
		t.Errorf("inventory = %v, want pick + ore", g.inv)
	}
}

func TestHarvest_DepletesOnLastCharge(t *testing.T) {
	s, store, node, yield, g := harvestFixture(t, "") // no tool required
	_ = store
	node.SetProperty(PropNodeCharges, 1) // one charge left
	res := s.Harvest(context.Background(), g, node, yield)
	if res.Outcome != HarvestOK || !res.Depleted {
		t.Fatalf("outcome = %v depleted=%v, want HarvestOK + depleted", res.Outcome, res.Depleted)
	}
	if nodeCharges(node) != 0 {
		t.Errorf("charges = %d, want 0", nodeCharges(node))
	}
}

func TestHarvest_NoToolRequiredNeverRefuses(t *testing.T) {
	s, _, node, yield, g := harvestFixture(t, "") // empty required tool
	res := s.Harvest(context.Background(), g, node, yield)
	if res.Outcome != HarvestOK {
		t.Errorf("outcome = %v, want HarvestOK (no tool requirement → never refused)", res.Outcome)
	}
}

// Two harvesters racing for the last charge of a 1-charge node: exactly one
// yields, the other gets HarvestNoCharges. Guards the §8 no-dupe property
// (TakeCharge is the single-winner claim). -race catches a regression.
func TestHarvest_ConcurrentLastChargeNoDupe(t *testing.T) {
	s, store, node, yield, _ := harvestFixture(t, "") // no tool required
	node.SetProperty(PropNodeCharges, 1)              // one charge for two harvesters

	g1 := &fakeGatherer{id: "p1"}
	g2 := &fakeGatherer{id: "p2"}

	var wg sync.WaitGroup
	results := make([]HarvestResult, 2)
	wg.Add(2)
	go func() { defer wg.Done(); results[0] = s.Harvest(context.Background(), g1, node, yield) }()
	go func() { defer wg.Done(); results[1] = s.Harvest(context.Background(), g2, node, yield) }()
	wg.Wait()

	oks, empties := 0, 0
	for _, r := range results {
		switch r.Outcome {
		case HarvestOK:
			oks++
		case HarvestNoCharges:
			empties++
		}
	}
	if oks != 1 || empties != 1 {
		t.Fatalf("outcomes ok=%d empty=%d, want exactly one of each (no double-yield)", oks, empties)
	}
	// Exactly one ore exists in the store + one gatherer's bag (the loser's
	// staged yield was untracked).
	total := len(g1.inv) + len(g2.inv)
	if total != 1 {
		t.Errorf("total yielded items = %d, want 1 (the dupe was prevented)", total)
	}
	_ = store
	if nodeCharges(node) != 0 {
		t.Errorf("charges = %d, want 0", nodeCharges(node))
	}
}
