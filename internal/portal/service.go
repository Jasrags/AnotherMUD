package portal

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fallbackSeq is the monotonic counter the newPortalID fallback
// uses when crypto/rand fails. Mirrors the M14.2 fix in
// internal/notifications (which had the same zeroed-buffer
// fallback bug); shared package-level state because portal ids
// are not bound to a Service instance.
var fallbackSeq atomic.Uint64

// Portal is a single live keyword exit record. The Service owns
// the registry; world.Room.KeywordExits holds the on-the-room
// registration that actually services movement.
//
// PairedID is non-empty for both halves of a symmetric pair; the
// two records cross-reference each other via that field. On
// removal the Service tears down both halves atomically.
type Portal struct {
	ID          string
	SourceRoom  world.RoomID
	TargetRoom  world.RoomID
	Keyword     string // lowercased
	DisplayName string
	ExpiryTick  uint64 // 0 means "no expiry"; >0 expires when area tick reaches this value
	PairedID    string // empty for one-way portals
}

// Service is the in-memory portal registry. Safe for concurrent
// use; the mutex serializes create / remove / expire so paired
// operations are atomic (spec §5.6 "no orphaned half-pair"). Read
// methods (Get, All, Len) take the read lock so a busy area-tick
// expire pass does not block status queries.
//
// The service holds a *World reference because every create /
// remove must register the keyword exit on the room itself.
//
// Lock order: portal.Service.mu → world.World.mu. Service writes
// always call into AddKeywordExit / RemoveKeywordExit (which take
// world.mu) while holding s.mu. Any future caller that needs to
// query the portal service from inside a world-locked path MUST
// preserve this order — taking s.mu while holding w.mu inverts
// it and would deadlock under contention.
type Service struct {
	mu    sync.RWMutex
	world *world.World
	byID  map[string]*Portal
	// byRoomKeyword indexes (room, keyword) → portal id for fast
	// lookup-by-location (used to refuse double-register on the
	// same keyword at a room).
	byRoomKeyword map[roomKeyword]string

	sink Sink
}

type roomKeyword struct {
	room    world.RoomID
	keyword string
}

// Sink is the optional bus-bridge surface. The composition root
// implements it to publish portal.opened / portal.closed events.
// nil-safe; a Service with no sink simply doesn't emit.
type Sink interface {
	OnPortalOpened(p Portal)
	OnPortalClosed(p Portal)
}

// NewService returns a Service wired to w. Pass nil for sink in
// tests / contexts where event emission is not desired.
func NewService(w *world.World, sink Sink) *Service {
	return &Service{
		world:         w,
		byID:          make(map[string]*Portal),
		byRoomKeyword: make(map[roomKeyword]string),
		sink:          sink,
	}
}

// Create installs a one-way keyword exit on srcRoom mapping
// keyword to targetRoom, with expiryTick (0 disables expiry) and
// an optional displayName. Returns the new portal's id or "" on
// refusal:
//
//   - srcRoom or targetRoom unregistered in the world
//   - keyword already taken on srcRoom (either by an existing portal
//     OR by an unrelated keyword exit)
//   - empty keyword
//
// Spec: §5.6 "create single-direction exit".
func (s *Service) Create(srcRoom world.RoomID, keyword string, targetRoom world.RoomID, expiryTick uint64, displayName string) string {
	key := strings.ToLower(strings.TrimSpace(keyword))
	if key == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Defer to world.AddKeywordExit for the actual registration —
	// it also catches missing rooms + collisions with non-portal
	// keyword exits. Returns false if the keyword is taken.
	if !s.world.AddKeywordExit(srcRoom, key, targetRoom) {
		return ""
	}
	p := &Portal{
		ID:          newPortalID(),
		SourceRoom:  srcRoom,
		TargetRoom:  targetRoom,
		Keyword:     key,
		DisplayName: displayName,
		ExpiryTick:  expiryTick,
	}
	s.byID[p.ID] = p
	s.byRoomKeyword[roomKeyword{srcRoom, key}] = p.ID
	if s.sink != nil {
		s.sink.OnPortalOpened(*p)
	}
	return p.ID
}

// CreatePaired installs symmetric keyword exits on both rooms,
// cross-referencing the two records via PairedID. Refuses if
// either side fails the precondition checks Create runs. On
// refusal nothing is registered (atomicity guarantee — spec
// "no orphaned half-pair").
//
// A single portal.opened event fires for the primary (srcRoom)
// side; the paired-partner registration is silent to avoid
// duplicate events for the same logical portal.
func (s *Service) CreatePaired(srcRoom world.RoomID, srcKeyword string, targetRoom world.RoomID, targetKeyword string, expiryTick uint64, displayName string) (primaryID, partnerID string) {
	srcKey := strings.ToLower(strings.TrimSpace(srcKeyword))
	dstKey := strings.ToLower(strings.TrimSpace(targetKeyword))
	if srcKey == "" || dstKey == "" {
		return "", ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.world.AddKeywordExit(srcRoom, srcKey, targetRoom) {
		return "", ""
	}
	if !s.world.AddKeywordExit(targetRoom, dstKey, srcRoom) {
		// Roll back the source-side registration so the pair is
		// all-or-nothing (spec §5.6 atomicity).
		s.world.RemoveKeywordExit(srcRoom, srcKey)
		return "", ""
	}
	primary := &Portal{
		ID:          newPortalID(),
		SourceRoom:  srcRoom,
		TargetRoom:  targetRoom,
		Keyword:     srcKey,
		DisplayName: displayName,
		ExpiryTick:  expiryTick,
	}
	partner := &Portal{
		ID:          newPortalID(),
		SourceRoom:  targetRoom,
		TargetRoom:  srcRoom,
		Keyword:     dstKey,
		DisplayName: displayName,
		ExpiryTick:  expiryTick,
	}
	primary.PairedID = partner.ID
	partner.PairedID = primary.ID

	s.byID[primary.ID] = primary
	s.byID[partner.ID] = partner
	s.byRoomKeyword[roomKeyword{srcRoom, srcKey}] = primary.ID
	s.byRoomKeyword[roomKeyword{targetRoom, dstKey}] = partner.ID
	if s.sink != nil {
		s.sink.OnPortalOpened(*primary)
	}
	return primary.ID, partner.ID
}

// Remove tears down the portal with id. If the portal is paired,
// its partner is also removed under the same lock so concurrent
// expirations / removals cannot leave an orphaned half-pair.
//
// Emits portal.closed for the primary side only. Paired-partner
// removal is silent on the event side per spec §5.6.
//
// Returns true when something was removed.
func (s *Service) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeLocked(id, true)
}

// removeLocked is the lock-held removal core. emitForPrimary
// controls whether portal.closed fires; the auto-expiry path
// sets it to false for partners so the pair only emits one
// portal.closed event.
func (s *Service) removeLocked(id string, emitForPrimary bool) bool {
	p, ok := s.byID[id]
	if !ok {
		return false
	}
	// Tear down the world-side registration; if the partner is
	// still tracked we remove it too (recurse with emit=false so
	// only the primary side emits).
	s.world.RemoveKeywordExit(p.SourceRoom, p.Keyword)
	delete(s.byID, id)
	delete(s.byRoomKeyword, roomKeyword{p.SourceRoom, p.Keyword})
	if emitForPrimary && s.sink != nil {
		s.sink.OnPortalClosed(*p)
	}
	if p.PairedID != "" {
		s.removeLocked(p.PairedID, false)
	}
	return true
}

// ExpireUpTo sweeps portals whose ExpiryTick is non-zero AND <=
// currentTick AND whose SourceRoom lives in areaID (id-prefix
// match like ResetDoorsInArea). For each, the Service tears down
// the portal and its paired partner (regardless of which area the
// partner lives in — spec §5.6 "removes paired partners regardless
// of partner's area").
//
// Returns the number of portals (primary sides) removed. The
// composition root subscribes the service to area.tick events
// and routes the tick count + area id through this method.
func (s *Service) ExpireUpTo(areaID world.AreaID, currentTick uint64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := string(areaID) + ":"
	bare := string(areaID)

	// Collect first so we don't mutate the map while iterating.
	expired := make([]string, 0)
	for id, p := range s.byID {
		if p.ExpiryTick == 0 || p.ExpiryTick > currentTick {
			continue
		}
		room := string(p.SourceRoom)
		if room != bare && !strings.HasPrefix(room, prefix) {
			continue
		}
		// Only collect the primary side; the paired removal
		// inside removeLocked handles the partner. Skip
		// partners by checking PairedID + ordering: if both
		// sides are in this area's expired set we collect the
		// "smaller" id to break the tie.
		if p.PairedID != "" {
			if partner, ok := s.byID[p.PairedID]; ok {
				partnerInArea := string(partner.SourceRoom) == bare ||
					strings.HasPrefix(string(partner.SourceRoom), prefix)
				if partnerInArea && partner.ID < p.ID {
					continue
				}
			}
		}
		expired = append(expired, id)
	}

	for _, id := range expired {
		s.removeLocked(id, true)
	}
	return len(expired)
}

// Get returns a snapshot of the portal under id. The returned
// value is a copy; mutating it does not affect the registry.
func (s *Service) Get(id string) (Portal, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.byID[id]
	if !ok {
		return Portal{}, false
	}
	return *p, true
}

// All returns every active portal in arbitrary order. Fresh
// slice; callers may mutate it.
func (s *Service) All() []Portal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Portal, 0, len(s.byID))
	for _, p := range s.byID {
		out = append(out, *p)
	}
	return out
}

// Len returns the number of active portals.
func (s *Service) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byID)
}

// newPortalID returns an opaque process-unique portal id. The
// happy path uses 64 bits of crypto/rand. The fallback path
// (crypto/rand returning an error — vanishingly rare in practice)
// uses an atomic counter so two concurrent fallbacks still get
// distinct ids. Mirrors the M14.2 notifications.newID fix.
func newPortalID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "p-" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("p-fallback-%d", fallbackSeq.Add(1))
}
