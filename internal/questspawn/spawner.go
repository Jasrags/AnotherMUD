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

// OwnerProperty is the per-instance property key holding the owning player's
// id on every quest-spawned entity, and Tag is the marker tag stamped
// alongside it (quest-spawns.md §4/§9). The visibility layer (Phase 2) reads
// OwnerProperty to gate a spawn to its owner: an entity carrying a non-empty
// OwnerProperty is invisible to every observer except that player. Exported so
// the composition-root primitive stamps them and the command/visibility layer
// reads them off the same contract.
const (
	OwnerProperty = "quest_spawn_owner"
	Tag           = "quest_spawn"
)

// Primitive is the engine-side create/destroy seam the spawner drives. The
// composition root backs it with the same boot spawn pipeline the admin
// `spawn` verb uses, plus placement/store removal for despawn. owner is the
// spawning player's id, stamped onto the entity as OwnerProperty so the
// Phase-2 visibility gate can scope the spawn to its owner (§4).
type Primitive interface {
	// SpawnMob runs the full mob pipeline into roomID (gear, pools, AI).
	SpawnMob(ctx context.Context, templateID string, roomID world.RoomID, owner string) (entities.EntityID, error)
	// SpawnItem mints an item and places it on roomID's floor.
	SpawnItem(ctx context.Context, templateID string, roomID world.RoomID, owner string) (entities.EntityID, error)
	// Despawn removes an entity from the world + store; best-effort and
	// idempotent (a mob already killed is simply gone).
	Despawn(id entities.EntityID)
}

// Definitions resolves a quest id to its definition (the stages carry the
// spawn declarations). *quest.Registry satisfies it.
type Definitions interface {
	Lookup(questID string) (*quest.Definition, bool)
}

// QuestState reads a player's live quest progress for login re-derivation
// (quest-spawns.md §7). *quest.Service satisfies it. Late-bound via
// SetQuestState because the service is constructed after the spawner (the
// spawner is one of the service's own event sinks).
type QuestState interface {
	Snapshot(playerID string) *quest.State
}

// Spawner implements quest.EventSink for the spawn lifecycle.
type Spawner struct {
	ctx   context.Context
	prim  Primitive
	defs  Definitions
	state QuestState // late-bound (SetQuestState); nil disables login re-derive
	log   *slog.Logger

	mu     sync.Mutex
	owned  map[string]map[string][]entities.EntityID // player -> quest -> spawned ids
	active map[string]struct{}                       // "player|quest|stage" -> spawned once
}

// SetQuestState wires the quest-progress source for login re-derivation (§7).
// Called once from the composition root after the quest service exists.
func (s *Spawner) SetQuestState(state QuestState) { s.state = state }

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
			id, err := s.spawnOne(sp, playerID)
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

func (s *Spawner) spawnOne(sp quest.Spawn, owner string) (entities.EntityID, error) {
	room := world.RoomID(sp.Room)
	switch sp.Kind {
	case "item":
		return s.prim.SpawnItem(s.ctx, sp.Template, room, owner)
	default: // "mob" (kinds are validated at load)
		return s.prim.SpawnMob(s.ctx, sp.Template, room, owner)
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

// ReactivatePlayer re-derives a player's active-stage spawns on login
// (quest-spawns.md §7). For each of the player's active quests it re-activates
// the currently-active stage; activate is idempotent, so if the spawns already
// exist (a link-dead reconnect that was never cleaned) this is a no-op. Called
// from the composition root's session-Add hook. No-op if no quest state is
// wired (headless/tests).
func (s *Spawner) ReactivatePlayer(playerID string) {
	if s.state == nil {
		return
	}
	st := s.state.Snapshot(playerID)
	if st == nil {
		return
	}
	for _, aq := range st.Active {
		s.activate(playerID, aq.QuestID, aq.StageIndex)
	}
}

// CleanupPlayer despawns every entity a player owns across all their quests and
// forgets their activation keys — the logout / link-dead-reap cleanup
// (quest-spawns.md §5/§7) so the shared world does not accumulate orphaned quest
// content while the player is offline. Their active-stage spawns are recreated
// by ReactivatePlayer on next login.
func (s *Spawner) CleanupPlayer(playerID string) {
	s.mu.Lock()
	var ids []entities.EntityID
	for _, qids := range s.owned[playerID] {
		ids = append(ids, qids...)
	}
	delete(s.owned, playerID)
	prefix := playerID + "|"
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
