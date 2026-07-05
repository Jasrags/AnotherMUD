package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fakeSpawnService is a command.SpawnService backed by the fixture's real store
// + placement, so a spawned item/mob is assertable through them. Unknown ids
// return an error (the handler maps that to a "No … template" refusal).
type fakeSpawnService struct {
	store *entities.Store
	place *entities.Placement
	items map[string]*item.Template
	mobs  map[string]*mob.Template
}

var errUnknownSpawnTpl = errors.New("unknown template")

func (s *fakeSpawnService) SpawnItem(_ context.Context, templateID string) (entities.EntityID, string, error) {
	tpl, ok := s.items[templateID]
	if !ok {
		return "", "", errUnknownSpawnTpl
	}
	inst, err := s.store.Spawn(tpl)
	if err != nil {
		return "", "", err
	}
	return inst.ID(), inst.Name(), nil
}

func (s *fakeSpawnService) SpawnMob(_ context.Context, templateID string, roomID world.RoomID) (entities.EntityID, string, error) {
	tpl, ok := s.mobs[templateID]
	if !ok {
		return "", "", errUnknownSpawnTpl
	}
	inst, err := s.store.SpawnMob(tpl)
	if err != nil {
		return "", "", err
	}
	s.place.Place(inst.ID(), roomID)
	return inst.ID(), inst.Name(), nil
}

func spawnFixtureService(f *considerFixture) *fakeSpawnService {
	return &fakeSpawnService{
		store: f.store,
		place: f.place,
		items: map[string]*item.Template{"sword": sword()},
		mobs:  map[string]*mob.Template{"guard": guardTplForConsider()},
	}
}

// spawn item … me mints the item into the actor's inventory and audits.
func TestSpawn_ItemToInventory(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	env.Spawn = spawnFixtureService(f)

	dispatchRole(t, env, admin, "spawn item sword me")

	inv := admin.Inventory()
	if len(inv) != 1 {
		t.Fatalf("inventory = %v, want 1 spawned item", inv)
	}
	if _, ok := f.store.GetByID(inv[0]); !ok {
		t.Error("spawned item not tracked in the store")
	}
	if !strings.Contains(admin.lastLine(), "a short sword") {
		t.Errorf("confirmation = %q", admin.lastLine())
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.AdminAction); ev.Verb != "spawn" || !strings.HasPrefix(ev.Args, "item:") {
		t.Errorf("event = %+v, want verb=spawn args=item:*", ev)
	}
}

// A bare id resolves against the current room's pack namespace: pack loading
// namespaces every template id (ashandarei → wot:ashandarei), so `spawn item
// ashandarei` must find it while standing in a wot-namespaced room.
func TestSpawn_BareIDResolvesRoomNamespace(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	// The fixture room is "tapestry-core:town-square"; register the template
	// only under its qualified id, so a bare "sword" must be namespace-resolved.
	svc := spawnFixtureService(f)
	svc.items = map[string]*item.Template{"tapestry-core:sword": sword()}
	env.Spawn = svc

	dispatchRole(t, env, admin, "spawn item sword me")

	if len(admin.Inventory()) != 1 {
		t.Fatalf("inventory = %v, want the namespace-resolved item", admin.Inventory())
	}
	if !strings.Contains(admin.lastLine(), "a short sword") {
		t.Errorf("confirmation = %q", admin.lastLine())
	}
}

// spawn item … here drops the item on the room floor (not the inventory).
func TestSpawn_ItemToRoom(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Spawn = spawnFixtureService(f)

	dispatchRole(t, env, admin, "spawn item sword here")

	if len(admin.Inventory()) != 0 {
		t.Errorf("inventory = %v, want empty (item went to the room)", admin.Inventory())
	}
	inRoom := f.place.InRoom(f.room.ID)
	found := false
	for _, id := range inRoom {
		if e, ok := f.store.GetByID(id); ok {
			if it, ok := e.(*entities.ItemInstance); ok && it.Name() == "a short sword" {
				found = true
			}
		}
	}
	if !found {
		t.Error("spawned item not placed in the room")
	}
	if !strings.Contains(admin.lastLine(), "onto the ground") {
		t.Errorf("confirmation = %q", admin.lastLine())
	}
}

// spawn mob … mints the mob into the current room through the service.
func TestSpawn_MobIntoRoom(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	env.Spawn = spawnFixtureService(f)

	// Fixture already holds one guard; a spawn adds a second.
	before := len(f.place.InRoom(f.room.ID))
	dispatchRole(t, env, admin, "spawn mob guard")

	if after := len(f.place.InRoom(f.room.ID)); after != before+1 {
		t.Errorf("room occupants = %d, want %d", after, before+1)
	}
	if !strings.Contains(admin.lastLine(), "You spawn a village guard") {
		t.Errorf("confirmation = %q", admin.lastLine())
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.AdminAction); ev.Verb != "spawn" || ev.Args != "mob:guard" {
		t.Errorf("event = %+v, want verb=spawn args=mob:guard", ev)
	}
}

// spawn item <id> <count> me mints N instances into the inventory, audits each,
// and reports the multiplier.
func TestSpawn_ItemCountToInventory(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	env.Spawn = spawnFixtureService(f)

	dispatchRole(t, env, admin, "spawn item sword 3 me")

	if n := len(admin.Inventory()); n != 3 {
		t.Fatalf("inventory = %d items, want 3", n)
	}
	if !strings.Contains(admin.lastLine(), "a short sword (x3)") {
		t.Errorf("confirmation = %q, want the (x3) multiplier", admin.lastLine())
	}
	if len(*got) != 3 {
		t.Errorf("admin.action count = %d, want one per spawned item", len(*got))
	}
}

// The count is position-independent: it can precede the destination keyword too.
func TestSpawn_ItemCountBeforeDest(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Spawn = spawnFixtureService(f)

	dispatchRole(t, env, admin, "spawn item sword 2 here")

	if n := len(admin.Inventory()); n != 0 {
		t.Errorf("inventory = %d, want empty (items went to the room)", n)
	}
	swords := 0
	for _, id := range f.place.InRoom(f.room.ID) {
		if e, ok := f.store.GetByID(id); ok {
			if it, ok := e.(*entities.ItemInstance); ok && it.Name() == "a short sword" {
				swords++
			}
		}
	}
	if swords != 2 {
		t.Errorf("swords on the floor = %d, want 2", swords)
	}
	if !strings.Contains(admin.lastLine(), "a short sword (x2) onto the ground") {
		t.Errorf("confirmation = %q", admin.lastLine())
	}
}

// spawn mob <id> <count> mints N mobs into the room, auditing each.
func TestSpawn_MobCount(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	env.Spawn = spawnFixtureService(f)

	before := len(f.place.InRoom(f.room.ID))
	dispatchRole(t, env, admin, "spawn mob guard 4")

	if after := len(f.place.InRoom(f.room.ID)); after != before+4 {
		t.Errorf("room occupants = %d, want %d", after, before+4)
	}
	if !strings.Contains(admin.lastLine(), "You spawn a village guard (x4)") {
		t.Errorf("confirmation = %q", admin.lastLine())
	}
	if len(*got) != 4 {
		t.Errorf("admin.action count = %d, want one per spawned mob", len(*got))
	}
}

// An out-of-range count is rejected before anything is minted.
func TestSpawn_CountOutOfRange(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Spawn = spawnFixtureService(f)

	dispatchRole(t, env, admin, "spawn item sword 0 me")

	if !strings.Contains(admin.lastLine(), "Spawn how many?") {
		t.Errorf("message = %q, want the count-range refusal", admin.lastLine())
	}
	if len(admin.Inventory()) != 0 {
		t.Error("a rejected count still minted an item")
	}
}

// spawn gold <n> adds to the actor's purse through the currency service.
func TestSpawn_Gold(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	admin.SetGold(10)
	env := f.env()
	env.Bus = bus
	env.Currency = economy.NewCurrencyService(nil)

	dispatchRole(t, env, admin, "spawn gold 250")

	if admin.Gold() != 260 {
		t.Errorf("gold = %d, want 260 (10 + 250)", admin.Gold())
	}
	if !strings.Contains(admin.lastLine(), "260") {
		t.Errorf("confirmation = %q, want the new balance", admin.lastLine())
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.AdminAction); ev.Verb != "spawn" || ev.Args != "gold:250" {
		t.Errorf("event = %+v, want verb=spawn args=gold:250", ev)
	}
}

// An unknown item template reports the miss and audits nothing.
func TestSpawn_UnknownTemplate(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	env.Spawn = spawnFixtureService(f)

	dispatchRole(t, env, admin, "spawn item nonesuch")

	if !strings.Contains(admin.lastLine(), `No item template "nonesuch"`) {
		t.Errorf("message = %q, want an unknown-template refusal", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a failed spawn must not audit, got %d", len(*got))
	}
}

// A bare `spawn` renders the usage panel and audits nothing.
func TestSpawn_BareShowsUsage(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Spawn = spawnFixtureService(f)

	dispatchRole(t, env, admin, "spawn")

	if !strings.Contains(admin.lastLine(), "Spawn what?") {
		t.Errorf("message = %q, want the usage panel", admin.lastLine())
	}
}

// spawn is admin-gated: a non-admin gets "Huh?" and nothing spawns.
func TestSpawn_RefusedForNonAdmin(t *testing.T) {
	f := newConsiderFixture(t)
	bob := newRoleActor("Bob", "p-bob") // no admin role
	bob.SetRoom(f.room)
	env := f.env()
	env.Spawn = spawnFixtureService(f)

	dispatchRole(t, env, bob, "spawn item sword me")

	if bob.lastLine() != "Huh?" {
		t.Errorf("refusal = %q, want 'Huh?'", bob.lastLine())
	}
	if len(bob.Inventory()) != 0 {
		t.Error("a non-admin spawn produced an item")
	}
}
