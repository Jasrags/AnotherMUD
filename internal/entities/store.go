package entities

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/pool"
)

// Errors callers may distinguish at the boundary.
var (
	ErrNotFound           = errors.New("entity not found")
	ErrAlreadyTracked     = errors.New("entity already tracked")
	ErrNotTracked         = errors.New("entity is not tracked")
	ErrUnknownTemplate    = errors.New("item template unknown")
	ErrUnknownMobTemplate = errors.New("mob template unknown")
)

// Store is the runtime entity tracker. It owns the by-id index and the
// tag double-buffer required by world-rooms-movement §4. By-type is
// served by scanning the by-id index (spec §4.3: "No secondary index
// is required").
//
// Mutations (Track, Untrack, Spawn) and the SwapTagIndex tick handler
// take the write lock; queries (GetByID, GetByTag, GetByType) take the
// read lock. The tag write index can mutate freely under writers — it
// is the read index that queries see, and the read index changes only
// at SwapTagIndex.
type Store struct {
	mu sync.RWMutex

	byID map[EntityID]Entity
	// Two tag indices: queries read tagsRead; mutations write tagsWrite.
	// SwapTagIndex copies tagsWrite into tagsRead at the tick boundary
	// (spec §3.7).
	tagsRead  map[string]map[EntityID]Entity
	tagsWrite map[string]map[EntityID]Entity

	// idGen produces sequentially numbered EntityIDs (atomic so future
	// Spawn-from-non-Store paths still produce unique ids).
	idGen atomic.Uint64

	// Optional fallback consulted by GetByID when the tracked index
	// misses (spec §4.2 step 2). Nil by default in M5.2 because rooms
	// don't carry entity lists yet — wires in with M5.4 get/drop.
	roomScan func(EntityID) (Entity, bool)

	// channelMap is the active ruleset's stat→combat-channel derivation,
	// stamped onto every spawned MobInstance so its Stats() derives
	// HitMod/AC through the mapping. Nil by default (tests) → mobs read
	// the stat keys directly, which the baseline mapping reproduces, so
	// behavior is identical either way. Set once at composition via
	// SetChannelMap before any spawn.
	channelMap *channel.Mapping

	// mobPoolDecls is the active world's mob-seed pool declarations
	// (shadowrun-mvp SR-M3a). Each spawned mob's pool.Set is seeded from these
	// (a Shadowrun mob's Stun monitor); nil/empty in fantasy/WoT worlds leaves
	// every mob set empty. Set once at composition via SetMobPools (which also
	// retro-seeds mobs spawned during Load), mirroring channelMap.
	mobPoolDecls []*pool.Decl
}

// SetChannelMap installs the ruleset's combat-channel derivation, applied
// to every subsequently spawned mob AND retro-stamped onto mobs already
// tracked (those spawned during pack Load, before the mapping was built
// from content). Called once at composition root after Load and before the
// tick loop starts, so the field writes on tracked mobs race nothing.
func (s *Store) SetChannelMap(m *channel.Mapping) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelMap = m
	for _, e := range s.byID {
		if mob, ok := e.(*MobInstance); ok {
			mob.channelMap = m
		}
	}
}

// SetMobPools installs the ruleset's mob-seed pool declarations (shadowrun-mvp
// SR-M3a), applied to every subsequently spawned mob AND retro-seeded onto mobs
// already tracked (those spawned during pack Load, before the decls were built
// from content). Called once at composition root after Load and before the tick
// loop starts, so the writes on tracked mobs race nothing. Mirrors
// SetChannelMap. A nil/empty decl set leaves every mob's pool.Set empty (the
// fantasy/WoT default). The retro-seed holds the write lock across a
// seedMobPools call per tracked mob (each taking that mob's statBlock + pool
// sub-locks) — immaterial as a one-shot startup cost, but a latency spike for
// concurrent readers if this is ever promoted to a hot-reload path (the same
// caveat SetChannelMap carries).
func (s *Store) SetMobPools(decls []*pool.Decl) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mobPoolDecls = decls
	for _, e := range s.byID {
		if mob, ok := e.(*MobInstance); ok {
			seedMobPools(mob, decls)
		}
	}
}

// NewStore returns an empty Store with no room-scan fallback.
func NewStore() *Store {
	return &Store{
		byID:      make(map[EntityID]Entity),
		tagsRead:  make(map[string]map[EntityID]Entity),
		tagsWrite: make(map[string]map[EntityID]Entity),
	}
}

// SetRoomScan installs the room-scan fallback used by GetByID when the
// tracked index misses (spec §4.2 step 2). Pass nil to clear. Safe to
// call at any time; must be set before serving traffic in production.
func (s *Store) SetRoomScan(fn func(EntityID) (Entity, bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roomScan = fn
}

// Track adds e to the id index and the tag write-side index. Returns
// ErrAlreadyTracked if an entity with the same id is already present.
// Spec §4.1.
func (s *Store) Track(e Entity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byID[e.ID()]; exists {
		return fmt.Errorf("%w: %q", ErrAlreadyTracked, e.ID())
	}
	s.byID[e.ID()] = e
	s.addTagsLocked(e)
	return nil
}

// Untrack removes e from the id index and the tag write-side index.
// Returns ErrNotTracked if the entity isn't present. Spec §4.1.
func (s *Store) Untrack(id EntityID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotTracked, id)
	}
	delete(s.byID, id)
	s.removeTagsLocked(e)
	return nil
}

// Retag refreshes the tag index for id after its underlying tag
// slice has been mutated in place (e.g. ApplyRacialFlags adding
// racial flags, SetAlignmentTag swapping bucket tags). Without
// this, the store's tag index only reflects the tag state at
// Track time and stale-by-construction once any in-place mutator
// runs.
//
// Closes the m8-5 deferred fix: SetAlignmentTag on MobInstance
// (and the equivalent path for racial flags from M8.3) modify
// m.tags directly; this method republishes the entity into the
// correct buckets so a subsequent GetByTag("alignment_evil")
// returns it.
//
// Sweeps every bucket on the WRITE side and removes id, then
// re-adds via addTagsLocked which uses the entity's current
// Tags(). The cost is O(num_distinct_tags); typical engines have
// O(10s) of tags so the sweep is cheap. Read index is unaffected
// until the next SwapTagIndex tick — readers see the prior tag
// set until then, matching how Track / Untrack already publish.
//
// Returns ErrNotTracked when id is not in the store.
func (s *Store) Retag(id EntityID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotTracked, id)
	}
	// Sweep write-side buckets removing this id wherever it
	// appears. Cannot use removeTagsLocked because it iterates the
	// entity's CURRENT tags and the index may carry stale entries
	// from a prior tag set.
	for tag, bucket := range s.tagsWrite {
		if _, present := bucket[id]; present {
			delete(bucket, id)
			if len(bucket) == 0 {
				delete(s.tagsWrite, tag)
			}
		}
	}
	s.addTagsLocked(e)
	return nil
}

// Count returns the number of currently-tracked entities.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byID)
}

// GetByID resolves id per spec §4.2: tracked index, then room-scan
// fallback if installed. Returns the entity and true on hit. The
// pending-player side index (§4.2 step 3) lands with character
// creation; until then the second fallback is unused.
//
// On room-scan hit, the entity is opportunistically promoted into the
// tracked index per §4.2.
func (s *Store) GetByID(id EntityID) (Entity, bool) {
	s.mu.RLock()
	if e, ok := s.byID[id]; ok {
		s.mu.RUnlock()
		return e, true
	}
	scan := s.roomScan
	s.mu.RUnlock()

	if scan == nil {
		return nil, false
	}
	e, ok := scan(id)
	if !ok {
		return nil, false
	}
	// Promote to tracked index. Two concurrent GetByIDs that both miss
	// can race here: both call scan (which returns the same pointer),
	// both call Track. One wins; the other gets ErrAlreadyTracked and
	// is ignored. Both callers still return the *same* entity pointer
	// the scan produced, so no caller sees inconsistency.
	_ = s.Track(e)
	return e, true
}

// GetByTag returns every entity carrying tag, from the read-side tag
// index. The returned slice is freshly allocated and safe to mutate;
// internal index entries are never exposed (spec §4.3 "read-only
// relative to internal state").
func (s *Store) GetByTag(tag string) []Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket := s.tagsRead[tag]
	if len(bucket) == 0 {
		return nil
	}
	out := make([]Entity, 0, len(bucket))
	for _, e := range bucket {
		out = append(out, e)
	}
	return out
}

// GetByType filters the tracked id index by entity type
// (case-insensitive per spec §4.3). No secondary index — the spec
// explicitly waives one. The returned slice is freshly allocated.
func (s *Store) GetByType(typ string) []Entity {
	want := strings.ToLower(typ)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Entity
	for _, e := range s.byID {
		if strings.ToLower(e.Type()) == want {
			out = append(out, e)
		}
	}
	return out
}

// GuidesOwnedBy returns the entity ids of every tracked onboarding-guide mob
// currently owned by ownerID (onboarding-guide.md). The guide-spawn path uses it
// to sweep a guide stranded by a PRIOR session before materializing a fresh one,
// enforcing the one-guide-per-owner invariant even when a re-entry built a new
// session without draining the old guide. Filters on the engine-authoritative
// IsGuide() + OwnerID() (set at materialization), not a content tag, so it holds
// across worlds. Reads the id index directly (not the double-buffered tag index),
// so a guide spawned earlier this tick is still found. Empty ownerID returns nil.
//
// Mob pointers are gathered under s.mu, then the IsGuide()/OwnerID() predicates
// (which take each mob's own ownerMu) run AFTER releasing s.mu, so the two locks
// are never held nested.
func (s *Store) GuidesOwnedBy(ownerID string) []EntityID {
	if ownerID == "" {
		return nil
	}
	s.mu.RLock()
	mobs := make([]*MobInstance, 0, len(s.byID))
	for _, e := range s.byID {
		if m, ok := e.(*MobInstance); ok {
			mobs = append(mobs, m)
		}
	}
	s.mu.RUnlock()

	var out []EntityID
	for _, m := range mobs {
		if m.IsGuide() && m.OwnerID() == ownerID {
			out = append(out, m.ID())
		}
	}
	return out
}

// SwapTagIndex publishes the write-side tag index to readers and
// initializes a fresh write side seeded from the new read side. Called
// at every tick boundary by the registered tick handler (spec §3.7
// "swap operation" / §4.3 read consistency).
func (s *Store) SwapTagIndex() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tagsRead = s.tagsWrite
	// Fresh write side starts as a deep-enough copy of the read side
	// so subsequent mutations land on a buffer that already reflects
	// committed state. Buckets are copied; entities inside are
	// pointers, intentionally shared.
	next := make(map[string]map[EntityID]Entity, len(s.tagsRead))
	for tag, bucket := range s.tagsRead {
		dup := make(map[EntityID]Entity, len(bucket))
		maps.Copy(dup, bucket)
		next[tag] = dup
	}
	s.tagsWrite = next
}

// Spawn instantiates an item from tpl per spec §2.3, assigning a fresh
// EntityID and tracking the result before returning. Returns
// ErrUnknownTemplate if tpl is nil so callers can pipeline a registry
// lookup straight into Spawn without an extra nil check.
//
// Concurrency: id generation is atomic via the idGen counter (no lock
// needed); Track acquires the store's write lock for the index
// insertion. The two steps are NOT in a single critical section, but
// the atomic counter guarantees unique ids and Track guarantees the
// id→entity mapping is consistent once it returns — a concurrent
// reader either sees the new entity or does not, never a half-built
// index entry.
func (s *Store) Spawn(tpl *item.Template) (*ItemInstance, error) {
	if tpl == nil {
		return nil, ErrUnknownTemplate
	}
	id := s.nextID()
	inst := buildInstanceFromTemplate(tpl, id)
	if err := s.Track(inst); err != nil {
		// Track failure on a freshly minted id means the id generator
		// is broken; surface immediately rather than swallow.
		return nil, fmt.Errorf("spawn: tracking new instance: %w", err)
	}
	return inst, nil
}

// SpawnMob instantiates a mob from tpl per spec mobs-ai-spawning §2.3
// (instantiation) and tracks the result. Mirrors Spawn for items in
// shape and concurrency model — see Spawn's doc for the atomic-id +
// Track-locked invariant. Spec steps §2.3 1-5 happen inside
// buildMobFromTemplate; the remaining spawn-pipeline steps (§3.1
// step 5 "set the entity's location and add it to the room", step 10
// "emit a mob spawned event") are the caller's responsibility because
// they require placement + bus refs that this package can't hold
// without a cycle (eventbus → entities).
func (s *Store) SpawnMob(tpl *mob.Template) (*MobInstance, error) {
	if tpl == nil {
		return nil, ErrUnknownMobTemplate
	}
	id := s.nextID()
	inst := buildMobFromTemplate(tpl, id)
	// Stamp the ruleset combat-channel derivation (nil ⇒ direct stat
	// reads). Read under RLock so a concurrent SetChannelMap (today only
	// at composition, but a future hot-reload must not data-race here)
	// publishes a consistent pointer.
	s.mu.RLock()
	inst.channelMap = s.channelMap
	decls := s.mobPoolDecls
	s.mu.RUnlock()
	// Seed the mob's resource pools from the world's mob-seed decls (SR-M3a) —
	// a Shadowrun mob's Stun monitor. Empty decls leave the set empty (fantasy/
	// WoT). decls is a slice header over an immutable array (set once by
	// SetMobPools), and inst is not yet tracked, so this needs no lock.
	seedMobPools(inst, decls)
	if err := s.Track(inst); err != nil {
		return nil, fmt.Errorf("spawn mob: tracking new instance: %w", err)
	}
	return inst, nil
}

// ContainerType is the entity type for synthetic container instances
// (corpses, and any future runtime container). It matches the
// item-template "container" type so the existing container-access
// machinery (look-in, get-from, capacity) applies unchanged.
const ContainerType = "container"

// SpawnContainer mints a template-less container ItemInstance with a
// runtime display name, tags, keywords, and properties, then tracks it
// (mirroring Spawn's atomic-id + Track-locked invariant). Used for
// runtime-created containers — e.g. corpses (loot-and-corpses §2) —
// that derive their identity from gameplay rather than a content
// template. The instance carries no template id, so the stacking
// service treats each as a unique singleton
// (inventory-equipment-items §5.1) and persistence/loot listeners that
// key off PropTemplateID simply see none.
func (s *Store) SpawnContainer(name string, tags, keywords []string, props map[string]any) (*ItemInstance, error) {
	id := s.nextID()
	inst := &ItemInstance{
		id:       id,
		typ:      ContainerType,
		name:     name,
		tags:     append([]string(nil), tags...),
		keywords: append([]string(nil), keywords...),
	}
	if len(props) > 0 {
		inst.properties = normalizeProperties(props)
	}
	// Track failure on a freshly minted atomic id means the id
	// generator is broken; surface it like Spawn rather than return an
	// untracked entity the caller would place in the world unseen.
	if err := s.Track(inst); err != nil {
		return nil, fmt.Errorf("spawn container: tracking new instance: %w", err)
	}
	return inst, nil
}

func (s *Store) nextID() EntityID {
	n := s.idGen.Add(1)
	return EntityID("entity-" + strconv.FormatUint(n, 10))
}

// addTagsLocked inserts e into every tag bucket on the write side. The
// caller MUST hold s.mu for writing.
func (s *Store) addTagsLocked(e Entity) {
	for _, t := range e.Tags() {
		bucket, ok := s.tagsWrite[t]
		if !ok {
			bucket = make(map[EntityID]Entity)
			s.tagsWrite[t] = bucket
		}
		bucket[e.ID()] = e
	}
}

// removeTagsLocked deletes e from every tag bucket on the write side
// and prunes empty buckets. The caller MUST hold s.mu for writing.
func (s *Store) removeTagsLocked(e Entity) {
	for _, t := range e.Tags() {
		bucket, ok := s.tagsWrite[t]
		if !ok {
			continue
		}
		delete(bucket, e.ID())
		if len(bucket) == 0 {
			delete(s.tagsWrite, t)
		}
	}
}
