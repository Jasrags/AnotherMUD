package questspawn

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

type spawnRec struct {
	kind, template, room string
	id                   entities.EntityID
}

// fakePrim records what the spawner asked it to create/destroy, and can be told
// to fail on a specific template to exercise the error path.
type fakePrim struct {
	mu           sync.Mutex
	n            int
	spawned      []spawnRec
	despawned    []entities.EntityID
	failTemplate string
}

func (f *fakePrim) mint(kind, tid string, room world.RoomID) (entities.EntityID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if tid == f.failTemplate {
		return "", errors.New("spawn boom")
	}
	f.n++
	id := entities.EntityID(fmt.Sprintf("e%d", f.n))
	f.spawned = append(f.spawned, spawnRec{kind, tid, string(room), id})
	return id, nil
}

func (f *fakePrim) SpawnMob(_ context.Context, tid string, room world.RoomID) (entities.EntityID, error) {
	return f.mint("mob", tid, room)
}
func (f *fakePrim) SpawnItem(_ context.Context, tid string, room world.RoomID) (entities.EntityID, error) {
	return f.mint("item", tid, room)
}
func (f *fakePrim) Despawn(id entities.EntityID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.despawned = append(f.despawned, id)
}

type fakeDefs map[string]*quest.Definition

func (d fakeDefs) Lookup(id string) (*quest.Definition, bool) { def, ok := d[id]; return def, ok }

func runDef() *quest.Definition {
	return &quest.Definition{
		ID: "q1",
		Stages: []quest.Stage{
			{ID: "site"}, // stage 0: no spawns (a visit stage)
			{ID: "job", Spawns: []quest.Spawn{
				{Kind: "mob", Template: "ganger", Room: "avondale", Count: 2},
				{Kind: "item", Template: "chip", Room: "avondale", Count: 1},
			}},
		},
	}
}

func newSpawner(defs fakeDefs) (*Spawner, *fakePrim) {
	prim := &fakePrim{}
	return New(context.Background(), prim, defs, nil), prim
}

func TestSpawner_StageAdvancedSpawnsThatStage(t *testing.T) {
	s, prim := newSpawner(fakeDefs{"q1": runDef()})

	// Accept activates stage 0 (no spawns) — nothing happens.
	s.Started(quest.StartedEvent{PlayerID: "p1", QuestID: "q1"})
	if len(prim.spawned) != 0 {
		t.Fatalf("stage 0 has no spawns; got %d", len(prim.spawned))
	}

	// Advancing into the job stage spawns its 2 gangers + 1 chip.
	s.StageAdvanced(quest.StageAdvancedEvent{PlayerID: "p1", QuestID: "q1", StageIndex: 1})
	if len(prim.spawned) != 3 {
		t.Fatalf("job stage should spawn 3 entities, got %d: %+v", len(prim.spawned), prim.spawned)
	}
	mobs, items := 0, 0
	for _, r := range prim.spawned {
		if r.room != "avondale" {
			t.Errorf("spawn in wrong room: %+v", r)
		}
		switch r.kind {
		case "mob":
			mobs++
		case "item":
			items++
		}
	}
	if mobs != 2 || items != 1 {
		t.Errorf("want 2 mobs + 1 item, got %d mobs %d items", mobs, items)
	}
}

func TestSpawner_Idempotent(t *testing.T) {
	s, prim := newSpawner(fakeDefs{"q1": runDef()})
	e := quest.StageAdvancedEvent{PlayerID: "p1", QuestID: "q1", StageIndex: 1}
	s.StageAdvanced(e)
	s.StageAdvanced(e) // re-fire: must not duplicate
	if len(prim.spawned) != 3 {
		t.Fatalf("re-firing stage activation duplicated spawns: got %d, want 3", len(prim.spawned))
	}
}

func TestSpawner_CleanupDespawnsAndAllowsRespawn(t *testing.T) {
	for _, end := range []string{"completed", "abandoned"} {
		t.Run(end, func(t *testing.T) {
			s, prim := newSpawner(fakeDefs{"q1": runDef()})
			s.StageAdvanced(quest.StageAdvancedEvent{PlayerID: "p1", QuestID: "q1", StageIndex: 1})
			if len(prim.spawned) != 3 {
				t.Fatalf("setup: want 3 spawns, got %d", len(prim.spawned))
			}

			switch end {
			case "completed":
				s.Completed(quest.CompletedEvent{PlayerID: "p1", QuestID: "q1"})
			case "abandoned":
				s.Abandoned(quest.AbandonedEvent{PlayerID: "p1", QuestID: "q1"})
			}
			if len(prim.despawned) != 3 {
				t.Fatalf("cleanup should despawn all 3 owned entities, got %d", len(prim.despawned))
			}

			// After cleanup the activation key is forgotten, so re-accepting
			// the same stage spawns fresh.
			s.StageAdvanced(quest.StageAdvancedEvent{PlayerID: "p1", QuestID: "q1", StageIndex: 1})
			if len(prim.spawned) != 6 {
				t.Fatalf("re-activation after cleanup should spawn again: got %d total spawns, want 6", len(prim.spawned))
			}
		})
	}
}

func TestSpawner_PerPlayerOwnership(t *testing.T) {
	s, prim := newSpawner(fakeDefs{"q1": runDef()})
	s.StageAdvanced(quest.StageAdvancedEvent{PlayerID: "p1", QuestID: "q1", StageIndex: 1})
	s.StageAdvanced(quest.StageAdvancedEvent{PlayerID: "p2", QuestID: "q1", StageIndex: 1})
	if len(prim.spawned) != 6 {
		t.Fatalf("two players should each get their own set: got %d, want 6", len(prim.spawned))
	}
	// Cleaning up p1 removes only p1's 3.
	s.Completed(quest.CompletedEvent{PlayerID: "p1", QuestID: "q1"})
	if len(prim.despawned) != 3 {
		t.Fatalf("cleanup of one player should despawn only their 3, got %d", len(prim.despawned))
	}
}

func TestSpawner_NoSpawnsIsNoop(t *testing.T) {
	s, prim := newSpawner(fakeDefs{"q1": runDef()})
	// Stage 0 (site) has no spawns.
	s.StageAdvanced(quest.StageAdvancedEvent{PlayerID: "p1", QuestID: "q1", StageIndex: 0})
	if len(prim.spawned) != 0 {
		t.Fatalf("a stage with no spawns must spawn nothing, got %d", len(prim.spawned))
	}
}

func TestSpawner_SpawnErrorSkipsEntryWithoutCrashing(t *testing.T) {
	s, prim := newSpawner(fakeDefs{"q1": runDef()})
	prim.failTemplate = "ganger" // both mobs fail; the chip still spawns
	s.StageAdvanced(quest.StageAdvancedEvent{PlayerID: "p1", QuestID: "q1", StageIndex: 1})
	if len(prim.spawned) != 1 || prim.spawned[0].kind != "item" {
		t.Fatalf("failed mob spawns should be skipped, chip should remain: got %+v", prim.spawned)
	}
	// Only the surviving owned entity is cleaned up.
	s.Abandoned(quest.AbandonedEvent{PlayerID: "p1", QuestID: "q1"})
	if len(prim.despawned) != 1 {
		t.Fatalf("cleanup should despawn only the 1 successfully-spawned entity, got %d", len(prim.despawned))
	}
}

func TestSpawner_UnknownQuestIsNoop(t *testing.T) {
	s, prim := newSpawner(fakeDefs{})
	s.Started(quest.StartedEvent{PlayerID: "p1", QuestID: "nope"})
	s.StageAdvanced(quest.StageAdvancedEvent{PlayerID: "p1", QuestID: "nope", StageIndex: 5})
	if len(prim.spawned) != 0 {
		t.Fatalf("unknown quest / out-of-range stage must be a no-op, got %d", len(prim.spawned))
	}
}
