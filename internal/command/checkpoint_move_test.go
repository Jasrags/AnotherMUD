package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// passRoller yields d20 face 20 (natural 20 → the scan always passes).
type passRoller struct{}

func (passRoller) IntN(int) int { return 19 }

// failRoller yields d20 face 1 (natural 1 → the scan always fails).
type failRoller struct{}

func (failRoller) IntN(int) int { return 0 }

// checkpointWorld builds A --north--> B where B is a SIN checkpoint requiring a
// `corporate` permit at DC 14 (sin-and-legality.md §7.1).
func checkpointWorld() (*world.World, *world.Room) {
	a := &world.Room{ID: "a", Name: "Avenue", Description: "The corporate strip.", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "b"}}}
	b := &world.Room{ID: "b", Name: "Enclave", Description: "Behind the turnstiles.", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "a"}},
		Properties: map[string]any{
			"checkpoint_permit":  "corporate",
			"checkpoint_scanner": 14,
		}}
	w := world.New()
	w.AddRoom(a)
	w.AddRoom(b)
	return w, a
}

// credentialInst spawns a credential item (a fake SIN) into the store and hands
// it to the actor, returning the live instance so a test can read its burned flag.
func credentialInst(t *testing.T, store *entities.Store, a *testActor, name string, rating int, permits ...string) *entities.ItemInstance {
	t.Helper()
	perms := make([]any, len(permits))
	for i, p := range permits {
		perms[i] = p
	}
	tpl := &item.Template{
		ID: item.TemplateID("cred-" + name), Name: name, Type: "item",
		Tags:       []string{economy.TagCredential},
		Properties: map[string]any{economy.PropPermits: perms, economy.PropCredentialRating: rating},
	}
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("spawn credential: %v", err)
	}
	a.AddToInventory(inst.ID())
	return inst
}

func checkpointDispatch(w *world.World, store *entities.Store, place *entities.Placement, roller passOrFail, a *testActor, line string) error {
	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		return err
	}
	svc := economy.NewShopService(item.NewTemplates(), store, economy.NewCurrencyService(nil), economy.DefaultEconomyConfig(), nil)
	env := command.Env{World: w, Items: store, Placement: place, Shop: svc, SkillRoller: roller}
	return reg.Dispatch(context.Background(), env, a, line)
}

// passOrFail is the roller shape both test rollers satisfy.
type passOrFail interface{ IntN(int) int }

func TestMove_CheckpointRefusesSINless(t *testing.T) {
	w, a := checkpointWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)

	if err := checkpointDispatch(w, store, place, passRoller{}, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "a" {
		t.Fatalf("SINless mover crossed the checkpoint; room = %q, want a", actor.Room().ID)
	}
	if got := strings.Join(actorLines(actor), "\n"); !strings.Contains(got, "no valid credentials") {
		t.Fatalf("expected a no-credentials refusal, got %q", got)
	}
}

func TestMove_CheckpointRefusesWrongPermit(t *testing.T) {
	w, a := checkpointWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	inst := credentialInst(t, store, actor, "a firearms SIN", 4, "firearms")

	if err := checkpointDispatch(w, store, place, failRoller{}, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "a" {
		t.Fatalf("wrong-permit mover crossed; room = %q, want a", actor.Room().ID)
	}
	// No permit match ⇒ no scan ⇒ the fake must not burn.
	if b, _ := inst.Property(economy.PropBurned); b == true {
		t.Error("credential burned though its permit never matched")
	}
}

func TestMove_CheckpointPassClears(t *testing.T) {
	w, a := checkpointWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	credentialInst(t, store, actor, "a premium SIN", 4, "corporate")

	if err := checkpointDispatch(w, store, place, passRoller{}, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "b" {
		t.Fatalf("valid mover blocked at the checkpoint; room = %q, want b", actor.Room().ID)
	}
}

func TestMove_CheckpointFailBurnsAndRefuses(t *testing.T) {
	w, a := checkpointWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	inst := credentialInst(t, store, actor, "a premium SIN", 4, "corporate")

	if err := checkpointDispatch(w, store, place, failRoller{}, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "a" {
		t.Fatalf("caught mover crossed; room = %q, want a", actor.Room().ID)
	}
	if b, _ := inst.Property(economy.PropBurned); b != true {
		t.Error("credential not burned after a failed checkpoint scan")
	}
	if got := strings.Join(actorLines(actor), "\n"); !strings.Contains(got, "burned") {
		t.Fatalf("expected a burn message, got %q", got)
	}
}
