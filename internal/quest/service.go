package quest

import (
	"fmt"
	"strings"
	"sync"
)

// DefaultActiveCap is the default ceiling on abandonable active quests
// (§3.3) when Config.Cap is not set.
const DefaultActiveCap = 20

// Player is the per-player view the service needs: an identity, prereq
// inputs (level on a track, class), and the class/race unlock setters
// (§3.2, §5.4). The session layer adapts its actor to this interface.
type Player interface {
	EntityID() string
	Level(track string) int
	Class() string
	SetClass(classID string)
	SetRace(raceID string)
}

// Persister writes a player's full quest state on every mutation (§6.2).
// The default is a no-op; the M10.8 persistence service supplies the
// file-writing implementation.
//
// Save is called OFF the service lock with a private deep-copy snapshot
// the persister owns, so it may take as long as a synchronous disk write
// needs without stalling other quest operations, and may retain the
// pointer freely.
type Persister interface {
	Save(playerID string, state *State)
}

// NopPersister discards saves.
type NopPersister struct{}

// Save implements Persister.
func (NopPersister) Save(string, *State) {}

// AcceptStatus enumerates the outcomes of an acceptance attempt (§3.1).
type AcceptStatus int

const (
	Accepted AcceptStatus = iota
	NotFound
	AlreadyActive
	AlreadyCompleted
	PrereqNotMet
	CapReached
)

// AcceptResult is the structured outcome of Accept.
type AcceptResult struct {
	Status AcceptStatus
	Banner string // populated on Accepted when not suppressed
}

// Config configures a Service. Registry is required; the rest default to
// no-ops / sensible values.
type Config struct {
	Registry *Registry
	Rewards  *Dispatcher
	Events   EventSink
	Persist  Persister
	Cap      int    // abandonable active-quest cap; <=0 → DefaultActiveCap
	Track    string // main progression track for prereq level; "" → DefaultTrack
}

// Service owns per-player quest state and the accept/advance/abandon
// operations (§3-§4). All operations serialize on a single mutex; the
// injected Dispatcher / EventSink / Persister run while the lock is held
// and MUST NOT call back into the service.
type Service struct {
	mu       sync.Mutex
	registry *Registry
	rewards  *Dispatcher
	events   EventSink
	persist  Persister
	states   map[string]*State
	players  map[string]Player // cache populated on Accept (§4.3)
	cap      int
	track    string
}

// NewService builds a service from cfg. cfg.Registry is required.
func NewService(cfg Config) *Service {
	if cfg.Registry == nil {
		panic("quest.NewService: nil Registry")
	}
	s := &Service{
		registry: cfg.Registry,
		rewards:  cfg.Rewards,
		events:   cfg.Events,
		persist:  cfg.Persist,
		states:   make(map[string]*State),
		players:  make(map[string]Player),
		cap:      cfg.Cap,
		track:    cfg.Track,
	}
	if s.rewards == nil {
		s.rewards = NewDispatcher()
	}
	if s.events == nil {
		s.events = NopEventSink{}
	}
	if s.persist == nil {
		s.persist = NopPersister{}
	}
	if s.cap <= 0 {
		s.cap = DefaultActiveCap
	}
	if s.track == "" {
		s.track = DefaultTrack
	}
	return s
}

// stateLocked returns the player's state, creating an empty one if
// absent. Caller holds s.mu.
func (s *Service) stateLocked(playerID string) *State {
	st, ok := s.states[playerID]
	if !ok {
		st = &State{}
		s.states[playerID] = st
	}
	return st
}

// LoadState installs a player's persisted state (M10.8 login load),
// overwriting any existing in-memory state. The state is cloned so the
// service owns its copy outright — a caller (e.g. the persistence layer)
// that retains the pointer it passed in cannot then observe the
// service's in-place mutations.
func (s *Service) LoadState(playerID string, state *State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[playerID] = state.clone() // clone handles nil → empty State
}

// DropState releases a player's in-memory state + cache entry (logout).
func (s *Service) DropState(playerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, playerID)
	delete(s.players, playerID)
}

// Definition returns the quest definition for id (read-only — see
// Registry.Lookup). Exposed so the quests command can render objective
// descriptions from the definition alongside the player's progress.
func (s *Service) Definition(id string) (*Definition, bool) {
	return s.registry.Lookup(id)
}

// ResolveID maps a player-supplied term (bare id / namespaced id / name)
// to a registered quest id. Used by the accept command.
func (s *Service) ResolveID(term string) (string, bool) {
	return s.registry.ResolveID(term)
}

// Snapshot returns a deep copy of a player's state, or nil if absent.
// Used by the persistence layer and the quests command.
func (s *Service) Snapshot(playerID string) *State {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[playerID]
	if !ok {
		return nil
	}
	return st.clone()
}

// Accept attempts to grant questID to player (§3.1).
func (s *Service) Accept(player Player, questID string, silent bool) AcceptResult {
	// Persist a snapshot AFTER the lock is released (§6.2): defers run
	// LIFO, so the Unlock below runs before this closure, keeping the
	// synchronous disk write off the service mutex.
	var pid string
	var snap *State
	defer func() {
		if snap != nil {
			s.persist.Save(pid, snap)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()

	def, ok := s.registry.Lookup(questID)
	if !ok {
		return AcceptResult{Status: NotFound}
	}
	pid = player.EntityID()
	st := s.stateLocked(pid)

	if st.findActive(questID) != nil {
		return AcceptResult{Status: AlreadyActive}
	}
	if !def.Repeatable && st.hasCompleted(questID) {
		return AcceptResult{Status: AlreadyCompleted}
	}
	if !s.prereqMet(player, st, def.Prereq) {
		return AcceptResult{Status: PrereqNotMet}
	}
	// Cap applies only when the quest being accepted is abandonable, and
	// counts only abandonable active quests (§3.3).
	if def.Abandonable && s.countAbandonableLocked(st) >= s.cap {
		return AcceptResult{Status: CapReached}
	}

	active := newActiveQuest(questID, 0, def.Stages[0])
	st.Active = append(st.Active, active)
	s.players[pid] = player // cache for reward dispatch at completion

	banner := ""
	if !def.Secret && !silent {
		banner = buildBanner(def, &active)
	}
	s.events.Started(StartedEvent{PlayerID: pid, QuestID: questID, Banner: banner})
	snap = st.clone() // persisted after unlock by the deferred closure
	return AcceptResult{Status: Accepted, Banner: banner}
}

// prereqMet checks the four prerequisite gates (§3.2). Caller holds s.mu.
func (s *Service) prereqMet(player Player, st *State, p Prerequisite) bool {
	if p.MinLevel > 0 && player.Level(s.track) < p.MinLevel {
		return false
	}
	if p.Class != "" && player.Class() != p.Class {
		return false
	}
	for _, q := range p.QuestsCompleted {
		if !st.hasCompleted(q) {
			return false
		}
	}
	for _, q := range p.QuestsNotCompleted {
		if st.hasCompleted(q) {
			return false
		}
	}
	return true
}

// countAbandonableLocked counts active quests whose definition is
// abandonable (§3.3). A missing definition counts as abandonable.
func (s *Service) countAbandonableLocked(st *State) int {
	n := 0
	for i := range st.Active {
		def, ok := s.registry.Lookup(st.Active[i].QuestID)
		if !ok || def.Abandonable {
			n++
		}
	}
	return n
}

// snapshotLocked deep-copies the player's state for an out-of-lock Save,
// or nil when the player has no state. Caller holds s.mu.
func (s *Service) snapshotLocked(playerID string) *State {
	if st, ok := s.states[playerID]; ok {
		return st.clone()
	}
	return nil
}

// AdvanceObjective moves a single objective forward (§4.1).
func (s *Service) AdvanceObjective(playerID, questID, objectiveID string, amount int) {
	var snap *State
	defer func() {
		if snap != nil {
			s.persist.Save(playerID, snap)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.advanceObjectiveLocked(playerID, questID, objectiveID, amount) {
		snap = s.snapshotLocked(playerID)
	}
}

// advanceObjectiveLocked is the §4.1 primitive. It returns whether it
// mutated state (so the caller knows to persist). Caller holds s.mu; it
// does NOT persist (the public wrapper does, off-lock).
func (s *Service) advanceObjectiveLocked(playerID, questID, objectiveID string, amount int) bool {
	st, ok := s.states[playerID]
	if !ok {
		return false
	}
	active := st.findActive(questID)
	if active == nil {
		return false
	}
	var obj *ObjectiveProgress
	for i := range active.Objectives {
		if active.Objectives[i].ObjectiveID == objectiveID {
			obj = &active.Objectives[i]
			break
		}
	}
	if obj == nil || obj.Complete() {
		return false // absent or already complete → no-op
	}
	obj.Current += amount
	if obj.Current > obj.Required {
		obj.Current = obj.Required
	}
	s.events.ObjectiveAdvanced(ObjectiveAdvancedEvent{
		PlayerID: playerID, QuestID: questID, ObjectiveID: objectiveID,
		Current: obj.Current, Required: obj.Required,
	})

	if !active.stageComplete() {
		return true
	}
	def, ok := s.registry.Lookup(questID)
	if ok && active.StageIndex+1 < len(def.Stages) {
		s.advanceStageLocked(playerID, active, def)
		return true
	}
	// Final stage done. A turn-in quest parks in AwaitingTurnIn until the
	// player returns to the giver (the TurnIn path dispatches its reward);
	// an auto-grant quest completes immediately. A quest with no resolved
	// definition can't declare turn_in, so it auto-completes (matches the
	// pre-turn-in behavior for orphaned-but-active quests).
	if ok && def.TurnIn {
		active.AwaitingTurnIn = true
		s.events.ReadyToTurnIn(ReadyToTurnInEvent{PlayerID: playerID, QuestID: questID, Giver: def.Giver})
		return true
	}
	s.completeLocked(playerID, st, questID)
	return true
}

// advanceStageLocked moves active to its next stage (§4.2). Caller holds
// s.mu.
func (s *Service) advanceStageLocked(playerID string, active *ActiveQuest, def *Definition) {
	active.StageIndex++
	next := def.Stages[active.StageIndex]
	active.Objectives = newActiveQuest(active.QuestID, active.StageIndex, next).Objectives
	s.events.StageAdvanced(StageAdvancedEvent{
		PlayerID: playerID, QuestID: active.QuestID, StageIndex: active.StageIndex,
	})
}

// completeLocked removes the quest, records completion, dispatches
// rewards, and emits the completed event (§4.3). Caller holds s.mu; it
// does NOT persist (the public wrapper does, off-lock).
//
// LOCK ORDER (load-bearing): reward Dispatch runs here under s.mu and
// reaches into the recipient's session — questXP/questItems take the
// Manager lock then the connActor lock, and SetClass/SetRace take the
// connActor lock. The acquire order is therefore s.mu → Manager.mu →
// connActor.mu. To avoid deadlock, NO caller may hold the Manager or a
// connActor lock while calling a Service method that can reach here
// (Accept/AdvanceObjective/AdvanceMatching) or that takes s.mu
// (HasMarker/Snapshot/Abandon). Today all such calls originate from the
// command/login/teardown paths with no session lock held — keep it so.
func (s *Service) completeLocked(playerID string, st *State, questID string) {
	st.removeActive(questID)
	st.Completed = append(st.Completed, questID)

	var reward Reward
	if def, ok := s.registry.Lookup(questID); ok {
		reward = def.Reward
		if player, cached := s.players[playerID]; cached {
			s.rewards.Dispatch(player, reward)
		}
	}
	s.events.Completed(CompletedEvent{
		PlayerID: playerID, QuestID: questID,
		XP: reward.XP, Gold: reward.Gold,
		Items: reward.Items, Abilities: reward.Abilities,
		ClassUnlock: reward.ClassUnlock, RaceUnlock: reward.RaceUnlock,
	})
}

// AdvanceMatching advances every current-stage objective of the given
// type whose definition satisfies predicate, for the named player (§4.4).
// The watcher (M10.9) uses this.
func (s *Service) AdvanceMatching(playerID, objType string, predicate func(Objective) bool) {
	var snap *State
	defer func() {
		if snap != nil {
			s.persist.Save(playerID, snap)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.states[playerID]
	if !ok {
		return
	}
	// Snapshot the (questID, objectiveID) pairs to advance BEFORE
	// mutating, so stage-advance / completion during advancement can't
	// disturb iteration (§4.4).
	type hit struct{ questID, objectiveID string }
	var hits []hit
	for i := range st.Active {
		active := &st.Active[i]
		def, ok := s.registry.Lookup(active.QuestID)
		if !ok || active.StageIndex >= len(def.Stages) {
			continue
		}
		for _, objDef := range def.Stages[active.StageIndex].Objectives {
			if !strings.EqualFold(objDef.Type, objType) {
				continue
			}
			if predicate(objDef) {
				hits = append(hits, hit{active.QuestID, objDef.ID})
			}
		}
	}
	mutated := false
	for _, h := range hits {
		if s.advanceObjectiveLocked(playerID, h.questID, h.objectiveID, 1) {
			mutated = true
		}
	}
	if mutated {
		snap = s.snapshotLocked(playerID)
	}
}

// Abandon removes an abandonable quest from the player's active list
// (§4.5). Silently no-ops when the quest is missing, not abandonable, or
// not active.
func (s *Service) Abandon(playerID, questID string) {
	var snap *State
	defer func() {
		if snap != nil {
			s.persist.Save(playerID, snap)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()

	def, ok := s.registry.Lookup(questID)
	if !ok || !def.Abandonable {
		return
	}
	st, ok := s.states[playerID]
	if !ok {
		return
	}
	if !st.removeActive(questID) {
		return
	}
	s.events.Abandoned(AbandonedEvent{PlayerID: playerID, QuestID: questID})
	snap = st.clone()
}

// TurnInStatus enumerates the outcomes of a TurnIn attempt (§4.3).
type TurnInStatus int

const (
	// TurnedIn — rewards dispatched, quest moved to completed.
	TurnedIn TurnInStatus = iota
	// TurnInNotActive — the player is not on that quest.
	TurnInNotActive
	// TurnInNotReady — active but not awaiting turn-in (objectives
	// outstanding, or it is an auto-grant quest that never parks here).
	TurnInNotReady
	// TurnInNotFound — no such quest definition.
	TurnInNotFound
)

// TurnInResult is the structured outcome of TurnIn.
type TurnInResult struct {
	Status TurnInStatus
}

// TurnIn claims an awaiting-turn-in quest's reward and completes it
// (§4.3). The CALLER (command layer) is responsible for verifying the
// player is co-located with the quest's giver before calling — the
// service is room-agnostic. Reward dispatch + the Completed event happen
// here via completeLocked, so the player-visible completion banner flows
// through the event sink exactly as it does for auto-grant quests (no
// double messaging).
func (s *Service) TurnIn(player Player, questID string) TurnInResult {
	var pid string
	var snap *State
	defer func() {
		if snap != nil {
			s.persist.Save(pid, snap)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.registry.Lookup(questID); !ok {
		return TurnInResult{Status: TurnInNotFound}
	}
	pid = player.EntityID()
	st, ok := s.states[pid]
	if !ok {
		return TurnInResult{Status: TurnInNotActive}
	}
	active := st.findActive(questID)
	if active == nil {
		return TurnInResult{Status: TurnInNotActive}
	}
	if !active.AwaitingTurnIn {
		return TurnInResult{Status: TurnInNotReady}
	}
	// Refresh the reward-dispatch cache: a turn-in after a relog runs on
	// state loaded from disk, with no Accept this session to populate it.
	s.players[pid] = player
	s.completeLocked(pid, st, questID)
	snap = st.clone()
	return TurnInResult{Status: TurnedIn}
}

// Offer is a quest a giver can offer a specific player right now —
// eligible to accept (§3). Pitch is the giver's spoken line.
type Offer struct {
	QuestID string
	Name    string
	Pitch   string
}

// OffersFrom returns the non-secret quests the giver template can offer
// the player: those the player could accept (not active, not already
// completed unless repeatable, prerequisites met). The active-quest cap
// is NOT applied — an over-cap player still sees the offer; Accept
// enforces the cap when they take it.
func (s *Service) OffersFrom(player Player, giverTemplateID string) []Offer {
	if giverTemplateID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pid := player.EntityID()
	st := s.stateLocked(pid)
	var out []Offer
	for _, def := range s.registry.All() {
		if def.Giver != giverTemplateID || def.Secret {
			continue
		}
		if !s.eligibleLocked(player, st, def) {
			continue
		}
		out = append(out, Offer{QuestID: def.ID, Name: offerName(def), Pitch: offerPitch(def)})
	}
	return out
}

// eligibleLocked reports whether the player could accept def right now,
// ignoring the active-quest cap (§3.2). It mirrors the gates Accept
// applies, minus the per-reason distinctions Accept needs for messaging.
// Caller holds s.mu.
func (s *Service) eligibleLocked(player Player, st *State, def *Definition) bool {
	if st.findActive(def.ID) != nil {
		return false
	}
	if !def.Repeatable && st.hasCompleted(def.ID) {
		return false
	}
	return s.prereqMet(player, st, def.Prereq)
}

// offerName / offerPitch resolve the giver-facing display strings for an
// offer, with sensible fallbacks.
func offerName(def *Definition) string {
	if def.Name != "" {
		return def.Name
	}
	return def.ID
}

func offerPitch(def *Definition) string {
	if def.Offer != "" {
		return def.Offer
	}
	if len(def.Stages) > 0 {
		return def.Stages[0].Description
	}
	return ""
}

// buildBanner renders the player-visible acceptance banner (§3.4). It
// reflects the initial state (stage 0, all objectives at zero) and names
// the quest by display name + classification. Emits semantic color tags
// for the render pipeline; richer layout can move to the command surface.
func buildBanner(def *Definition, active *ActiveQuest) string {
	name := def.Name
	if name == "" {
		name = def.ID
	}
	var b strings.Builder
	if def.Classification != "" {
		fmt.Fprintf(&b, "<title>New quest: %s (%s)</title>\r\n", name, def.Classification)
	} else {
		fmt.Fprintf(&b, "<title>New quest: %s</title>\r\n", name)
	}
	stage := def.Stages[active.StageIndex]
	if stage.Description != "" {
		b.WriteString("<subtle>" + stage.Description + "</subtle>\r\n")
	}
	for i, op := range active.Objectives {
		desc := stage.Objectives[i].Description
		if desc == "" {
			desc = stage.Objectives[i].Type
		}
		fmt.Fprintf(&b, "  - %s (%d/%d)\r\n", desc, op.Current, op.Required)
	}
	return b.String()
}
