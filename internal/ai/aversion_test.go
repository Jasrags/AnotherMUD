package ai

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// mobRefusesEntry: a Shadowspawn mob refuses a stedding-tagged room; everything
// else (non-stedding room, non-shadowspawn mob) is allowed.
func TestMobRefusesEntry(t *testing.T) {
	store := entities.NewStore()
	shadow := mustSpawn(t, store, &mob.Template{ID: "core:trolloc", Name: "a Trolloc", Type: "npc", Tags: []string{"shadowspawn"}})
	plain := mustSpawn(t, store, &mob.Template{ID: "core:dog", Name: "a dog", Type: "npc", Tags: []string{"animal"}})
	stedding := &world.Room{ID: "core:stump", Tags: []string{"stedding"}}
	open := &world.Room{ID: "core:field"}

	if !mobRefusesEntry(shadow, stedding) {
		t.Error("a Shadowspawn should refuse a stedding room")
	}
	if mobRefusesEntry(shadow, open) {
		t.Error("a Shadowspawn should NOT refuse a non-stedding room")
	}
	if mobRefusesEntry(plain, stedding) {
		t.Error("a non-Shadowspawn should NOT refuse a stedding room")
	}
	if mobRefusesEntry(nil, stedding) || mobRefusesEntry(shadow, nil) {
		t.Error("nil mob/room must never refuse (defensive)")
	}
}

func mustSpawn(t *testing.T, store *entities.Store, tpl *mob.Template) *entities.MobInstance {
	t.Helper()
	m, err := store.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob %s: %v", tpl.ID, err)
	}
	return m
}

// A Shadowspawn mob whose only wander exit leads into a stedding recoils at the
// bound and stays put; a plain mob in the same spot crosses freely.
func TestBehaviorWander_ShadowspawnRefusesStedding(t *testing.T) {
	// Shadowspawn case: tag B as a stedding, spawn a Trolloc in A.
	f := newWanderFixture(t)
	if b, err := f.world.Room("core:b"); err == nil {
		b.Tags = []string{"stedding"}
	}
	trolloc := mustSpawn(t, f.store, &mob.Template{
		ID: "core:trolloc", Name: "a Trolloc", Type: "npc",
		Behavior: BehaviorNameWander, Tags: []string{"shadowspawn"},
	})
	f.place.Place(trolloc.ID(), "core:a")

	if err := BehaviorWander(context.Background(), trolloc, f.deps()); err != nil {
		t.Fatalf("wander: %v", err)
	}
	if got, _ := f.place.RoomOf(trolloc.ID()); got != "core:a" {
		t.Errorf("Shadowspawn crossed into the stedding (now %q); want core:a (recoiled)", got)
	}
	if len(f.bcast.sent) != 0 {
		t.Errorf("Shadowspawn unexpectedly broadcast a move: %+v", f.bcast.sent)
	}

	// Control: a plain mob in the same layout DOES enter the stedding-tagged room.
	f2 := newWanderFixture(t)
	if b, err := f2.world.Room("core:b"); err == nil {
		b.Tags = []string{"stedding"}
	}
	dog := mustSpawn(t, f2.store, &mob.Template{
		ID: "core:dog", Name: "a dog", Type: "npc", Behavior: BehaviorNameWander,
	})
	f2.place.Place(dog.ID(), "core:a")
	if err := BehaviorWander(context.Background(), dog, f2.deps()); err != nil {
		t.Fatalf("control wander: %v", err)
	}
	if got, _ := f2.place.RoomOf(dog.ID()); got != "core:b" {
		t.Errorf("a non-Shadowspawn should enter the stedding; got %q, want core:b", got)
	}
}
