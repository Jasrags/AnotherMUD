package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// White-box tests for the inventory glue on connActor + the session
// helpers respawnInventory / untrackInventory. End-to-end coverage
// (login → drop → restart → verify) lives in session_test.go's
// dialer-driven suite once the integration harness is extended.

func newInvActor(t *testing.T, store *entities.Store) *connActor {
	t.Helper()
	return &connActor{
		save:  &player.Save{Version: player.CurrentVersion, Name: "Tester"},
		items: store,
	}
}

func swordTpl() *item.Template {
	return &item.Template{
		ID:       "tapestry-core:short-sword",
		Name:     "a short sword",
		Type:     "weapon",
		Keywords: []string{"sword"},
	}
}

func TestActor_AddInventory_SyncsSaveTemplateIDs(t *testing.T) {
	store := entities.NewStore()
	a := newInvActor(t, store)
	tpl := swordTpl()
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	a.AddToInventory(inst.ID())

	if !a.dirty {
		t.Error("dirty not set after AddToInventory")
	}
	if got := a.save.Inventory; len(got) != 1 || got[0] != string(tpl.ID) {
		t.Errorf("save.Inventory = %v, want [%q]", got, tpl.ID)
	}
}

func TestActor_RemoveInventory_ClearsSaveEntry(t *testing.T) {
	store := entities.NewStore()
	a := newInvActor(t, store)
	tpl := swordTpl()
	inst, _ := store.Spawn(tpl)

	a.AddToInventory(inst.ID())
	a.dirty = false // reset so we can confirm Remove flips it

	if !a.RemoveFromInventory(inst.ID()) {
		t.Error("RemoveFromInventory returned false")
	}
	if !a.dirty {
		t.Error("dirty not set after RemoveFromInventory")
	}
	if got := a.save.Inventory; len(got) != 0 {
		t.Errorf("save.Inventory = %v, want empty", got)
	}
}

func TestRespawnInventory_RehydratesFromTemplates(t *testing.T) {
	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(swordTpl())

	a := newInvActor(t, store)
	respawnInventory(context.Background(), a, store, tpls,
		[]string{"tapestry-core:short-sword"})

	if got := a.Inventory(); len(got) != 1 {
		t.Fatalf("inventory = %v, want one entry", got)
	}
	// Save shouldn't be marked dirty when nothing was dropped.
	if a.dirty {
		t.Error("dirty=true after clean respawn; should only flip on template drops")
	}
}

func TestRespawnInventory_DropsUnknownTemplate(t *testing.T) {
	store := entities.NewStore()
	tpls := item.NewTemplates()
	tpls.Add(swordTpl())

	a := newInvActor(t, store)
	respawnInventory(context.Background(), a, store, tpls,
		[]string{"tapestry-core:short-sword", "tapestry-core:does-not-exist"})

	if got := a.Inventory(); len(got) != 1 {
		t.Errorf("inventory = %v, want only the survivor", got)
	}
	if got := a.save.Inventory; len(got) != 1 || got[0] != "tapestry-core:short-sword" {
		t.Errorf("save.Inventory = %v, want trimmed to survivor", got)
	}
	if !a.dirty {
		t.Error("dirty not set after trimming dead reference")
	}
}

func TestUntrackInventory_RemovesEntitiesFromStore(t *testing.T) {
	store := entities.NewStore()
	a := newInvActor(t, store)
	inst, _ := store.Spawn(swordTpl())
	a.AddToInventory(inst.ID())

	untrackInventory(context.Background(), store, a)

	if _, ok := store.GetByID(inst.ID()); ok {
		t.Error("entity still tracked after untrackInventory")
	}
}
