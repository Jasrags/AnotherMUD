package command_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// --- testActor hirelingOwner capability (hireable-mobs.md §2) ---

func (a *testActor) OwnedHirelingTemplates() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.ownedHirelings...)
}

func (a *testActor) HirelingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.ownedHirelings)
}

func (a *testActor) AddHireling(templateID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ownedHirelings = append(a.ownedHirelings, templateID)
}

func (a *testActor) RemoveHireling(templateID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, t := range a.ownedHirelings {
		if t == templateID {
			a.ownedHirelings = append(a.ownedHirelings[:i], a.ownedHirelings[i+1:]...)
			return true
		}
	}
	return false
}

func (a *testActor) TrackLiveHireling(id entities.EntityID, templateID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.liveHirelingSet == nil {
		a.liveHirelingSet = map[entities.EntityID]string{}
	}
	a.liveHirelingSet[id] = templateID
}

func (a *testActor) UntrackLiveHireling(id entities.EntityID) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.liveHirelingSet[id]
	if ok {
		delete(a.liveHirelingSet, id)
	}
	return t, ok
}

func (a *testActor) LiveHireling(templateID string) (entities.EntityID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, t := range a.liveHirelingSet {
		if t == templateID {
			return id, true
		}
	}
	return "", false
}

func (a *testActor) LiveHirelings() []command.LiveHirelingRef {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]command.LiveHirelingRef, 0, len(a.liveHirelingSet))
	for id, t := range a.liveHirelingSet {
		out = append(out, command.LiveHirelingRef{ID: id, TemplateID: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (a *testActor) SetHirelingStance(id entities.EntityID, stance string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.hirelingStance == nil {
		a.hirelingStance = map[entities.EntityID]string{}
	}
	a.hirelingStance[id] = stance
}

func (a *testActor) stanceOf(id entities.EntityID) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.hirelingStance[id]
}

// fakeHirelingService is a scriptable command.HirelingService: one hireable
// template, and counters for the materialize/dematerialize calls.
type fakeHirelingService struct {
	templateID    string
	name          string
	cost          int
	nextID        int
	materialized  int
	dematerialize int
}

func (f *fakeHirelingService) FindHireable(query string) (string, string, int, bool) {
	if query == "" || strings.Contains(strings.ToLower(f.name), strings.ToLower(query)) {
		return f.templateID, f.name, f.cost, true
	}
	return "", "", 0, false
}

func (f *fakeHirelingService) HirelingName(templateID string) (string, bool) {
	if templateID == f.templateID {
		return f.name, true
	}
	return "", false
}

func (f *fakeHirelingService) Materialize(_ context.Context, _, _ string, _ world.RoomID) (entities.EntityID, error) {
	f.nextID++
	f.materialized++
	return entities.EntityID("h-" + string(rune('0'+f.nextID))), nil
}

func (f *fakeHirelingService) Dematerialize(context.Context, entities.EntityID) bool {
	f.dematerialize++
	return true
}

func hirelingFixture(t *testing.T) (command.Env, *testActor, *fakeHirelingService) {
	t.Helper()
	inv := newInvFixture(t)
	svc := &fakeHirelingService{templateID: "starter-world:sellsword", name: "a grizzled sellsword", cost: 50}
	a := newTestActor(inv.room)
	a.playerID = "p-1"
	a.SetGold(1000)
	env := command.Env{
		World: inv.world, Items: inv.store, Placement: inv.place,
		Currency: economy.NewCurrencyService(nil), Hirelings: svc, HirelingCap: 1,
	}
	return env, a, svc
}

func TestHire_Success(t *testing.T) {
	env, a, svc := hirelingFixture(t)
	dispatchBuiltin(t, env, a, "hire sellsword")
	if a.Gold() != 950 {
		t.Errorf("gold = %d, want 950 after a 50 hire", a.Gold())
	}
	if got := a.OwnedHirelingTemplates(); len(got) != 1 || got[0] != "starter-world:sellsword" {
		t.Errorf("owned = %v, want the hired sellsword", got)
	}
	if svc.materialized != 1 {
		t.Errorf("materialized = %d, want 1", svc.materialized)
	}
	if !strings.Contains(a.lastLine(), "hire a grizzled sellsword") {
		t.Errorf("reply = %q, want a hire confirmation", a.lastLine())
	}
}

func TestHire_InsufficientGold(t *testing.T) {
	env, a, svc := hirelingFixture(t)
	a.SetGold(10) // below 50
	dispatchBuiltin(t, env, a, "hire sellsword")
	if a.HirelingCount() != 0 || svc.materialized != 0 {
		t.Error("a broke character should not have hired anyone")
	}
	if !strings.Contains(a.lastLine(), "gold") {
		t.Errorf("reply = %q, want a too-poor message", a.lastLine())
	}
}

func TestHire_AtCap(t *testing.T) {
	env, a, _ := hirelingFixture(t) // cap 1
	a.AddHireling("starter-world:sellsword")
	dispatchBuiltin(t, env, a, "hire sellsword")
	if a.HirelingCount() != 1 {
		t.Errorf("count = %d, want 1 (cap blocks a second hire)", a.HirelingCount())
	}
	if !strings.Contains(a.lastLine(), "all the help") {
		t.Errorf("reply = %q, want an at-cap message", a.lastLine())
	}
}

func TestHire_ZeroCapMeansUnlimited(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	env.HirelingCap = 0      // unconfigured zero-value must not block hiring
	a.AddHireling("x:other") // already holds one
	dispatchBuiltin(t, env, a, "hire sellsword")
	if a.HirelingCount() != 2 {
		t.Errorf("count = %d, want 2 (cap 0 = no limit)", a.HirelingCount())
	}
}

func TestHire_UnknownName(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	dispatchBuiltin(t, env, a, "hire dragon")
	if a.HirelingCount() != 0 {
		t.Error("hiring an unknown name should not form a contract")
	}
	if !strings.Contains(a.lastLine(), "no \"dragon\" to hire") {
		t.Errorf("reply = %q, want a no-such-hireling message", a.lastLine())
	}
}

func TestDismiss_RemovesContract(t *testing.T) {
	env, a, svc := hirelingFixture(t)
	dispatchBuiltin(t, env, a, "hire sellsword")
	dispatchBuiltin(t, env, a, "dismiss sellsword")
	if a.HirelingCount() != 0 {
		t.Errorf("count after dismiss = %d, want 0", a.HirelingCount())
	}
	if svc.dematerialize != 1 {
		t.Errorf("dematerialize calls = %d, want 1", svc.dematerialize)
	}
	if !strings.Contains(a.lastLine(), "dismiss a grizzled sellsword") {
		t.Errorf("reply = %q, want a dismiss confirmation", a.lastLine())
	}
}

func TestOrder_SetsStanceByName(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	dispatchBuiltin(t, env, a, "hire sellsword")
	dispatchBuiltin(t, env, a, "order sellsword stay")
	if got := a.stanceOf("h-1"); got != command.HirelingStanceStay {
		t.Errorf("stance = %q, want stay", got)
	}
	if !strings.Contains(a.lastLine(), "hold this position") {
		t.Errorf("reply = %q, want a stay confirmation", a.lastLine())
	}
}

func TestOrder_UnnamedSoleHireling(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	dispatchBuiltin(t, env, a, "hire sellsword")
	dispatchBuiltin(t, env, a, "order guard") // no name → the sole live hireling
	if got := a.stanceOf("h-1"); got != command.HirelingStanceGuard {
		t.Errorf("stance = %q, want guard", got)
	}
}

func TestOrder_FollowResumes(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	dispatchBuiltin(t, env, a, "hire sellsword")
	dispatchBuiltin(t, env, a, "order sellsword stay")
	dispatchBuiltin(t, env, a, "order sellsword follow")
	if got := a.stanceOf("h-1"); got != command.HirelingStanceFollow {
		t.Errorf("stance = %q, want follow after re-order", got)
	}
}

func TestOrder_UnknownHireling(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	dispatchBuiltin(t, env, a, "hire sellsword")
	dispatchBuiltin(t, env, a, "order wolfhound stay")
	if !strings.Contains(a.lastLine(), `no "wolfhound" to order`) {
		t.Errorf("reply = %q, want unknown-hireling refusal", a.lastLine())
	}
}

func TestOrder_UnknownKeyword(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	dispatchBuiltin(t, env, a, "hire sellsword")
	dispatchBuiltin(t, env, a, "order sellsword dance")
	if !strings.Contains(a.lastLine(), "follow, stay, guard") {
		t.Errorf("reply = %q, want usage hint", a.lastLine())
	}
}

// attack requires the hireling to be co-located with the owner; the fake
// Materialize never calls place.Place, so RoomOf returns not-found and the
// co-location gate fires (the gate is only skipped when c.Placement is nil).
func TestOrder_AttackRequiresColocation(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	env.Combat = combat.NewManager(combat.MapLocator{}, nil)
	dispatchBuiltin(t, env, a, "hire sellsword")
	dispatchBuiltin(t, env, a, "order sellsword attack rat")
	if !strings.Contains(a.lastLine(), "isn't here to take that order") {
		t.Errorf("reply = %q, want co-location refusal", a.lastLine())
	}
}

// seedTwoHirelings gives the actor two live hirelings of the SAME template (the
// ambiguous duplicate case cap>1 must handle), with stable ids h-1 < h-2.
func seedTwoHirelings(a *testActor) {
	for _, id := range []entities.EntityID{"h-1", "h-2"} {
		a.AddHireling("starter-world:sellsword")
		a.TrackLiveHireling(id, "starter-world:sellsword")
	}
}

func TestHirelings_NumberedRoster(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	seedTwoHirelings(a)
	dispatchBuiltin(t, env, a, "hirelings")
	out := a.lastLine()
	if !strings.Contains(out, "1) a grizzled sellsword") || !strings.Contains(out, "2) a grizzled sellsword") {
		t.Errorf("roster not numbered:\n%s", out)
	}
}

func TestOrder_ByNumber(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	seedTwoHirelings(a)
	dispatchBuiltin(t, env, a, "order 2 guard")
	if got := a.stanceOf("h-2"); got != command.HirelingStanceGuard {
		t.Errorf("h-2 stance = %q, want guard", got)
	}
	if got := a.stanceOf("h-1"); got == command.HirelingStanceGuard {
		t.Error("h-1 should be untouched by `order 2`")
	}
}

func TestOrder_AmbiguousNameRefused(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	seedTwoHirelings(a)
	dispatchBuiltin(t, env, a, "order sellsword guard")
	if !strings.Contains(a.lastLine(), "more than one") {
		t.Errorf("ambiguous name not refused: %q", a.lastLine())
	}
	if a.stanceOf("h-1") == command.HirelingStanceGuard || a.stanceOf("h-2") == command.HirelingStanceGuard {
		t.Error("an ambiguous order should set no stance")
	}
}

func TestOrder_All(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	seedTwoHirelings(a)
	dispatchBuiltin(t, env, a, "order all stay")
	if a.stanceOf("h-1") != command.HirelingStanceStay || a.stanceOf("h-2") != command.HirelingStanceStay {
		t.Errorf("order all did not set both: h-1=%q h-2=%q", a.stanceOf("h-1"), a.stanceOf("h-2"))
	}
	if !strings.Contains(a.lastLine(), "Your 2 hirelings hold this position") {
		t.Errorf("band confirm = %q", a.lastLine())
	}
}

func TestDismiss_ByNumber(t *testing.T) {
	env, a, svc := hirelingFixture(t)
	seedTwoHirelings(a)
	dispatchBuiltin(t, env, a, "dismiss 2")
	if a.HirelingCount() != 1 {
		t.Errorf("count after dismiss = %d, want 1", a.HirelingCount())
	}
	if svc.dematerialize != 1 {
		t.Errorf("dematerialize calls = %d, want 1", svc.dematerialize)
	}
	if _, live := a.liveHirelingSet["h-2"]; live {
		t.Error("h-2 should be gone after `dismiss 2`")
	}
	if _, live := a.liveHirelingSet["h-1"]; !live {
		t.Error("h-1 should remain after `dismiss 2`")
	}
}

func TestDismiss_AmbiguousNameRefused(t *testing.T) {
	env, a, svc := hirelingFixture(t)
	seedTwoHirelings(a)
	dispatchBuiltin(t, env, a, "dismiss sellsword")
	if !strings.Contains(a.lastLine(), "more than one") {
		t.Errorf("ambiguous dismiss not refused: %q", a.lastLine())
	}
	if a.HirelingCount() != 2 || svc.dematerialize != 0 {
		t.Errorf("ambiguous dismiss removed something: count=%d demat=%d", a.HirelingCount(), svc.dematerialize)
	}
}

func TestHirelings_ListsContracts(t *testing.T) {
	env, a, _ := hirelingFixture(t)
	dispatchBuiltin(t, env, a, "hirelings")
	if !strings.Contains(a.lastLine(), "no hirelings") {
		t.Errorf("empty list = %q, want a none message", a.lastLine())
	}
	dispatchBuiltin(t, env, a, "hire sellsword")
	dispatchBuiltin(t, env, a, "hirelings")
	if got := a.lastLine(); !strings.Contains(got, "a grizzled sellsword") || !strings.Contains(got, "with you") {
		t.Errorf("list = %q, want the materialized sellsword", got)
	}
}
