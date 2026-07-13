// Package questspawn owns the Phase-1 quest-scoped spawn lifecycle
// (quest-spawns.md): it creates a quest stage's declared mobs/items when the
// stage becomes a player's active stage, and removes the survivors when the
// quest ends. It is a quest.EventSink — the composition root fans quest
// lifecycle events into it alongside the session notifier via quest.MultiSink.
//
// Phase 1 is shared-world, per-player-owned: spawns go into the real room and
// everyone sees them, but each (player, quest) owns its own set for cleanup.
// Per-observer visibility, login re-derivation, and cleanup of an already-
// collected item sitting in a player's session inventory are Phase 2 / 1b and
// are out of scope here (see the spec's §7/§10).
//
// Re-entrancy note: the service emits these events while holding its own lock,
// so a sink MUST NOT call back into the quest service. This spawner only
// touches the entity store / placement (through Primitive) and its own maps —
// and no spawn side effect routes back into a quest advance (the watcher keys
// off mob.killed / item.picked_up / player.moved, none of which a spawn emits).
package questspawn

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Primitive is the engine-side create/destroy seam the spawner drives. The
// composition root backs it with the same boot spawn pipeline the admin
// `spawn` verb uses, plus placement/store removal for despawn.
type Primitive interface {
	// SpawnMob runs the full mob pipeline into roomID (gear, pools, AI).
	SpawnMob(ctx context.Context, templateID string, roomID world.RoomID) (entities.EntityID, error)
	// SpawnItem mints an item and places it on roomID's floor.
	SpawnItem(ctx context.Context, templateID string, roomID world.RoomID) (entities.EntityID, error)
	// Despawn removes an entity from the world + store; best-effort and
	// idempotent (a mob already killed is simply gone).
	Despawn(id entities.EntityID)
}

// Definitions resolves a quest id to its definition (the stages carry the
// spawn declarations). *quest.Registry satisfies it.
type Definitions interface {
	Lookup(questID string) (*quest.Definition, bool)
}

// Spawner implements quest.EventSink for the spawn lifecycle.
type Spawner struct {
	ctx  context.Context
	prim Primitive
	defs Definitions
	log  *slog.Logger

	mu     sync.Mutex
	owned  map[string]map[string][]entities.EntityID // player -> quest -> spawned ids
	active map[string]struct{}                       // "player|quest|stage" -> spawned once
}

// New builds a Spawner. ctx carries the logger/cancellation for spawn calls.
func New(ctx context.Context, prim Primitive, defs Definitions, log *slog.Logger) *Spawner {
	if log == nil {
		log = slog.Default()
	}
	return &Spawner{
		ctx:    ctx,
		prim:   prim,
		defs:   defs,
		log:    log,
		owned:  make(map[string]map[string][]entities.EntityID),
		active: make(map[string]struct{}),
	}
}

// --- quest.EventSink ---

// Started spawns the first stage's content on accept (quest-spawns.md §3).
func (s *Spawner) Started(e quest.StartedEvent) { s.activate(e.PlayerID, e.QuestID, 0) }

// StageAdvanced spawns the newly-active stage's content (quest-spawns.md §3).
func (s *Spawner) StageAdvanced(e quest.StageAdvancedEvent) {
	s.activate(e.PlayerID, e.QuestID, e.StageIndex)
}

// Completed removes the quest's surviving spawns (quest-spawns.md §5).
func (s *Spawner) Completed(e quest.CompletedEvent) { s.cleanup(e.PlayerID, e.QuestID) }

// Abandoned removes the quest's surviving spawns (quest-spawns.md §5).
func (s *Spawner) Abandoned(e quest.AbandonedEvent) { s.cleanup(e.PlayerID, e.QuestID) }

// ObjectiveAdvanced / ReadyToTurnIn are not spawn triggers.
func (s *Spawner) ObjectiveAdvanced(quest.ObjectiveAdvancedEvent) {}
func (s *Spawner) ReadyToTurnIn(quest.ReadyToTurnInEvent)         {}

// --- lifecycle ---

// activate spawns stageIndex's declared content for the player, once. The
// idempotency guard means a redundant trigger (or a re-fire) does not
// duplicate. Spawning runs outside the lock; only the small map ops hold it.
func (s *Spawner) activate(playerID, questID string, stageIndex int) {
	def, ok := s.defs.Lookup(questID)
	if !ok || stageIndex < 0 || stageIndex >= len(def.Stages) {
		return
	}
	spawns := def.Stages[stageIndex].Spawns
	if len(spawns) == 0 {
		return
	}

	key := activationKey(playerID, questID, stageIndex)
	s.mu.Lock()
	if _, done := s.active[key]; done {
		s.mu.Unlock()
		return
	}
	s.active[key] = struct{}{}
	s.mu.Unlock()

	var ids []entities.EntityID
	for _, sp := range spawns {
		n := sp.Count
		if n < 1 {
			n = 1
		}
		for i := 0; i < n; i++ {
			id, err := s.spawnOne(sp)
			if err != nil {
				s.log.WarnContext(s.ctx, "quest spawn failed",
					"quest", questID, "player", playerID,
					"kind", sp.Kind, "template", sp.Template, "room", sp.Room, "err", err)
				continue
			}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		// Every spawn failed — release the activation claim so a later re-fire
		// (login re-derivation, Phase 1b) can retry, rather than leaving the
		// stage permanently marked done with nothing spawned (a non-abandonable
		// quest would otherwise strand uncompletable).
		s.mu.Lock()
		delete(s.active, key)
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	if s.owned[playerID] == nil {
		s.owned[playerID] = make(map[string][]entities.EntityID)
	}
	s.owned[playerID][questID] = append(s.owned[playerID][questID], ids...)
	s.mu.Unlock()
}

func (s *Spawner) spawnOne(sp quest.Spawn) (entities.EntityID, error) {
	room := world.RoomID(sp.Room)
	switch sp.Kind {
	case "item":
		return s.prim.SpawnItem(s.ctx, sp.Template, room)
	default: // "mob" (kinds are validated at load)
		return s.prim.SpawnMob(s.ctx, sp.Template, room)
	}
}

// cleanup despawns the player's surviving owned entities for the quest and
// forgets its activation keys so a later re-accept re-spawns. Despawn is
// best-effort: an entity the player already killed/consumed is a no-op.
func (s *Spawner) cleanup(playerID, questID string) {
	s.mu.Lock()
	var ids []entities.EntityID
	if byQuest, ok := s.owned[playerID]; ok {
		ids = byQuest[questID]
		delete(byQuest, questID)
		if len(byQuest) == 0 {
			delete(s.owned, playerID)
		}
	}
	prefix := playerID + "|" + questID + "|"
	for k := range s.active {
		if strings.HasPrefix(k, prefix) {
			delete(s.active, k)
		}
	}
	s.mu.Unlock()

	for _, id := range ids {
		s.prim.Despawn(id)
	}
}

func activationKey(playerID, questID string, stageIndex int) string {
	return playerID + "|" + questID + "|" + strconv.Itoa(stageIndex)
}
