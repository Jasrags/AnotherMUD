package session

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// A mob can be a follow leader (follow.md §3 mob-leader following): the graph
// keys on opaque ids, so a mob entity id is stored and reported just like a
// player leader id.
func TestFollowGraph_MobLeader(t *testing.T) {
	m := NewManager()
	const (
		follower = "abc123hex" // a player id
		mobID    = "entity-7"  // a mob entity id — never collides with hex player ids
	)
	if err := m.Follow(follower, mobID); err != nil {
		t.Fatalf("Follow(player, mob): %v", err)
	}
	if l, ok := m.Following(follower); !ok || l != mobID {
		t.Fatalf("Following = %q, %v; want %q, true", l, ok, mobID)
	}
	// Lose(mobID) drops the trailing player — the teardown path a mob death uses.
	lost := m.Lose(mobID)
	if len(lost) != 1 || lost[0] != follower {
		t.Fatalf("Lose(mob) = %v, want [%s]", lost, follower)
	}
	if _, ok := m.Following(follower); ok {
		t.Error("follower should be released after the mob leader is lost")
	}
}

// DropMobLeader releases trailing players when a followed mob dies and notifies
// each (follow.md §3/§4).
func TestDropMobLeader(t *testing.T) {
	m := NewManager()
	follower := &connActor{id: "c-f", playerID: "p-follower", room: &world.Room{ID: "z:a"},
		save: &player.Save{Name: "Trailer"}, conn: &fakeConn{id: "p-follower"}}
	m.Add(follower)

	const mobID = entities.EntityID("entity-42")
	if err := m.Follow("p-follower", string(mobID)); err != nil {
		t.Fatalf("Follow: %v", err)
	}

	m.DropMobLeader(context.Background(), mobID, "a dire boar")

	if _, ok := m.Following("p-follower"); ok {
		t.Error("follower should be released after the mob leader dies")
	}
	// A second drop is a harmless no-op (nothing left to lose).
	m.DropMobLeader(context.Background(), mobID, "a dire boar")
}

// leaderDisplayName resolves either kind of leader: an online player actor by
// player id, a mob by entity id via the store, else the generic fallback.
func TestLeaderDisplayName(t *testing.T) {
	store := entities.NewStore()
	mb, err := store.SpawnMob(&mob.Template{ID: "core:guard", Name: "a town guard", Type: "npc"})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}

	m := NewManager()
	m.actionEnv = command.Env{Items: store}
	pa := &connActor{id: "c-1", playerID: "p-hex", room: &world.Room{ID: "z:a"},
		save: &player.Save{Name: "Alice"}}
	m.Add(pa)

	if got := m.leaderDisplayName("p-hex"); got != "Alice" {
		t.Errorf("player leader name = %q, want Alice", got)
	}
	if got := m.leaderDisplayName(mb.EntityID()); got != "a town guard" {
		t.Errorf("mob leader name = %q, want %q", got, "a town guard")
	}
	if got := m.leaderDisplayName("entity-does-not-exist"); got != "someone" {
		t.Errorf("unknown leader name = %q, want someone", got)
	}
}
