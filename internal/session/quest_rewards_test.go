package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestQuestXPRewardGrantsOnConfiguredTrack is the regression for the
// playtest bug where quest XP rendered an award banner but never landed:
// the dispatcher defaulted to track "main" (spec quest.DefaultTrack)
// while content only registers "adventurer", and progression.GrantExperience
// silently drops grants on an unregistered track. NewQuestRewards now
// binds the real track, so a quest XP reward must actually credit it.
func TestQuestXPRewardGrantsOnConfiguredTrack(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	a.progress = progression.NewProgressionState()
	mgr.Add(a)

	tracks := progression.NewTrackRegistry()
	if err := tracks.Register(&progression.TrackDef{
		Name:     "adventurer",
		MaxLevel: 10,
		XPTable:  []int64{0, 0, 1000}, // level 2 needs 1000 XP
	}); err != nil {
		t.Fatalf("register track: %v", err)
	}
	prog := progression.NewManager(tracks, nil)

	rewards := NewQuestRewards(mgr, prog, nil, item.NewTemplates(), entities.NewStore(), nil, "adventurer")
	rewards.Dispatch(a, quest.Reward{XP: 25})

	if got := a.progress.XP("adventurer"); got != 25 {
		t.Errorf("adventurer XP = %d, want 25 (quest XP must land on the configured track)", got)
	}
	if got := a.progress.XP("main"); got != 0 {
		t.Errorf("main XP = %d, want 0 (quest XP must not leak to the spec-default track)", got)
	}
}

func TestQuestItemRewardGrantsToInventory(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	mgr.Add(a)

	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "core:potion", Name: "a potion", Type: "consumable"})

	// prog/prof nil → XP/ability stay no-op; only the item granter is exercised.
	rewards := NewQuestRewards(mgr, nil, nil, tpls, store, nil, "")
	rewards.Dispatch(a, quest.Reward{Items: []string{"core:potion"}})

	inv := a.Inventory()
	if len(inv) != 1 {
		t.Fatalf("inventory = %d items, want 1", len(inv))
	}
	if ent, ok := store.GetByID(inv[0]); !ok {
		t.Error("granted item not tracked in store")
	} else if inst, ok := ent.(*entities.ItemInstance); !ok || string(inst.TemplateID()) != "core:potion" {
		t.Errorf("granted item template = %v", ent)
	}
}

func TestQuestItemRewardMissingTemplateSilent(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	mgr.Add(a)
	rewards := NewQuestRewards(mgr, nil, nil, item.NewTemplates(), entities.NewStore(), nil, "")
	// unknown template → silent no-op, no panic, empty inventory.
	rewards.Dispatch(a, quest.Reward{Items: []string{"core:nope"}})
	if len(a.Inventory()) != 0 {
		t.Error("missing template should grant nothing")
	}
}

func TestQuestItemRewardOfflinePlayerNoop(t *testing.T) {
	mgr := NewManager() // actor never Added
	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "core:potion", Name: "a potion", Type: "consumable"})
	rewards := NewQuestRewards(mgr, nil, nil, tpls, store, nil, "")
	// recipient not online → GetByPlayerID misses → silent no-op.
	rewards.Dispatch(offlinePlayer{"ghost"}, quest.Reward{Items: []string{"core:potion"}})
	// nothing to assert beyond no panic + nothing spawned for a player.
}

func TestQuestGoldRewardCreditsPlayer(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	mgr.Add(a)

	currency := economy.NewCurrencyService(nil)
	rewards := NewQuestRewards(mgr, nil, nil, item.NewTemplates(), entities.NewStore(), currency, "")
	rewards.Dispatch(a, quest.Reward{Gold: 30})

	if got := a.Gold(); got != 30 {
		t.Errorf("gold = %d, want 30 (quest reward credited through currency service)", got)
	}
}

func TestQuestGoldRewardNoServiceIsNoop(t *testing.T) {
	mgr := NewManager()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	mgr.Add(a)

	// nil currency → no gold granter wired → reward is a silent no-op.
	rewards := NewQuestRewards(mgr, nil, nil, item.NewTemplates(), entities.NewStore(), nil, "")
	rewards.Dispatch(a, quest.Reward{Gold: 30})

	if got := a.Gold(); got != 0 {
		t.Errorf("gold = %d, want 0 (no currency service wired)", got)
	}
}

type offlinePlayer struct{ id string }

func (p offlinePlayer) EntityID() string { return p.id }
func (offlinePlayer) Level(string) int   { return 1 }
func (offlinePlayer) Class() string      { return "" }
func (offlinePlayer) SetClass(string)    {}
func (offlinePlayer) SetRace(string)     {}
