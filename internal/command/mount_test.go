package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// --- testActor mountOwner capability (mounts.md §2.2) ---

func (a *testActor) OwnedMountTemplates() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.ownedMounts...)
}

func (a *testActor) AddMount(templateID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ownedMounts = append(a.ownedMounts, templateID)
}

func (a *testActor) RemoveMount(templateID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, t := range a.ownedMounts {
		if t == templateID {
			a.ownedMounts = append(a.ownedMounts[:i], a.ownedMounts[i+1:]...)
			return true
		}
	}
	return false
}

func (a *testActor) TrackLiveMount(id entities.EntityID, templateID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.liveMountSet == nil {
		a.liveMountSet = map[entities.EntityID]string{}
	}
	a.liveMountSet[id] = templateID
}

func (a *testActor) UntrackLiveMount(id entities.EntityID) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.liveMountSet[id]
	if ok {
		delete(a.liveMountSet, id)
	}
	return t, ok
}

func (a *testActor) LiveMountTemplates() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.liveMountSet))
	for _, t := range a.liveMountSet {
		out = append(out, t)
	}
	return out
}

// --- fake MountService over the inv fixture's store ---

type fakeMountService struct {
	store *entities.Store
	place *entities.Placement
	names map[string]string // templateID → display name
}

func (f *fakeMountService) MountName(templateID string) (string, bool) {
	n, ok := f.names[templateID]
	return n, ok
}

func (f *fakeMountService) Materialize(ctx context.Context, ownerID, templateID string, roomID world.RoomID) (entities.EntityID, error) {
	inst, err := f.store.SpawnMob(&mob.Template{
		ID: mob.TemplateID(templateID), Name: f.names[templateID], Type: "npc",
		Mount: &mob.MountSpec{Temperament: "skittish", TravelMax: 60},
	})
	if err != nil {
		return "", err
	}
	inst.SetOwner(ownerID)
	f.place.Place(inst.ID(), roomID)
	return inst.ID(), nil
}

func (f *fakeMountService) Dematerialize(ctx context.Context, id entities.EntityID) bool {
	if _, ok := f.store.GetByID(id); !ok {
		return false
	}
	f.place.Remove(id)
	_ = f.store.Untrack(id)
	return true
}

// stablemasterTpl is a stable access point selling the given mount at a price.
func stablemasterTpl(mountID string, price int) *mob.Template {
	return &mob.Template{
		ID: "tapestry-core:stablemaster", Name: "a stablemaster", Type: "npc",
		Tags:     []string{"stable"},
		Keywords: []string{"stablemaster"},
		Properties: map[string]any{
			"stable": map[string]any{"sells": map[string]any{mountID: price}},
		},
	}
}

// mountFixture wires a room with a stablemaster, a fake mount service, and a
// currency service, returning the env + actor.
func mountFixture(t *testing.T) (command.Env, *testActor, *fakeMountService) {
	t.Helper()
	inv := newInvFixture(t)
	const mountID = "tapestry-core:riding-horse"
	sm, err := inv.store.SpawnMob(stablemasterTpl(mountID, 200))
	if err != nil {
		t.Fatalf("SpawnMob stablemaster: %v", err)
	}
	inv.place.Place(sm.ID(), inv.room.ID)

	svc := &fakeMountService{store: inv.store, place: inv.place, names: map[string]string{mountID: "a riding horse"}}
	a := newTestActor(inv.room)
	a.playerID = "p-1"
	a.SetGold(1000)
	env := command.Env{
		World: inv.world, Items: inv.store, Placement: inv.place,
		Broadcaster: nil, Currency: economy.NewCurrencyService(nil), Mounts: svc,
	}
	return env, a, svc
}

func TestBuyMount_Success(t *testing.T) {
	env, a, _ := mountFixture(t)
	dispatchBuiltin(t, env, a, "buymount horse")
	if a.Gold() != 800 {
		t.Errorf("gold = %d, want 800 after a 200 purchase", a.Gold())
	}
	if got := a.OwnedMountTemplates(); len(got) != 1 || got[0] != "tapestry-core:riding-horse" {
		t.Errorf("owned = %v, want the bought mount", got)
	}
	if !strings.Contains(a.lastLine(), "stabled here") {
		t.Errorf("reply = %q, want a buy-and-stabled confirmation", a.lastLine())
	}
}

func TestBuyMount_InsufficientGold(t *testing.T) {
	env, a, _ := mountFixture(t)
	a.SetGold(50) // below 200
	dispatchBuiltin(t, env, a, "buymount horse")
	if a.Gold() != 50 {
		t.Errorf("gold = %d, want 50 (no charge on a failed buy)", a.Gold())
	}
	if len(a.OwnedMountTemplates()) != 0 {
		t.Errorf("owned = %v, want none", a.OwnedMountTemplates())
	}
	if !strings.Contains(a.lastLine(), "only have 50") {
		t.Errorf("reply = %q, want an insufficient-gold message", a.lastLine())
	}
}

func TestBuyMount_NoStableInRoom(t *testing.T) {
	inv := newInvFixture(t)
	a := newTestActor(inv.room)
	a.SetGold(1000)
	env := command.Env{
		World: inv.world, Items: inv.store, Placement: inv.place,
		Currency: economy.NewCurrencyService(nil),
		Mounts:   &fakeMountService{store: inv.store, place: inv.place, names: map[string]string{}},
	}
	dispatchBuiltin(t, env, a, "buymount horse")
	if !strings.Contains(a.lastLine(), "no stable here") {
		t.Errorf("reply = %q, want a no-stable message", a.lastLine())
	}
}

// unstable materializes an owned stabled mount into the room, owned by the
// retriever and tracked live; stable then removes it again.
func TestUnstableThenStable(t *testing.T) {
	env, a, _ := mountFixture(t)
	a.AddMount("tapestry-core:riding-horse") // already owned (stabled)

	dispatchBuiltin(t, env, a, "unstable horse")
	if got := a.LiveMountTemplates(); len(got) != 1 {
		t.Fatalf("live mounts = %v, want 1 after unstable", got)
	}
	// The mount is now a live, owned creature in the room.
	var found *entities.MobInstance
	for _, id := range env.Placement.InRoom(a.Room().ID) {
		if e, ok := env.Items.GetByID(id); ok {
			if m, ok := e.(*entities.MobInstance); ok && m.IsMount() {
				found = m
			}
		}
	}
	if found == nil {
		t.Fatal("no live mount placed in the room after unstable")
	}
	if !found.IsOwnedBy("p-1") {
		t.Error("materialized mount is not owned by the retriever")
	}

	dispatchBuiltin(t, env, a, "stable horse")
	if got := a.LiveMountTemplates(); len(got) != 0 {
		t.Errorf("live mounts = %v, want 0 after stable", got)
	}
	if _, ok := env.Items.GetByID(found.ID()); ok {
		t.Error("mount still in the store after stable")
	}
	// Ownership is preserved across the stable (never deleted, §9).
	if len(a.OwnedMountTemplates()) != 1 {
		t.Errorf("owned = %v, want the mount still owned after stabling", a.OwnedMountTemplates())
	}
}

func TestMountsList(t *testing.T) {
	env, a, _ := mountFixture(t)
	a.AddMount("tapestry-core:riding-horse")
	dispatchBuiltin(t, env, a, "mounts")
	if !strings.Contains(a.lastLine(), "a riding horse") || !strings.Contains(a.lastLine(), "stabled") {
		t.Errorf("mounts list = %q, want the owned horse shown as stabled", a.lastLine())
	}
}
