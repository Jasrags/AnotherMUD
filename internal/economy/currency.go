// Package economy owns the M11 survival-and-economy services. The
// first slice (M11.1) is currency: a single integer gold property on
// an entity, plus the CurrencyService that mutates it and emits
// observable events.
//
// Spec: economy-survival §2.
//
// The service mirrors progression.AlignmentManager's seam shape: it
// operates on a small Entity interface (the connActor satisfies it)
// and reports state changes through a Sink the composition root
// bridges to eventbus.Publish, so this package stays free of any
// bus / session dependency.
package economy

import (
	"context"
	"errors"
	"sync"
)

// ErrNegativeAmount is returned by SetGold when given a negative
// target (spec §2.2: "Throws on negative input"). AddGold has no
// such error — negative deltas are the debit path.
var ErrNegativeAmount = errors.New("economy: gold amount cannot be negative")

// Entity is the holder a CurrencyService reads and writes gold on
// (spec §2.1 — gold lives directly on the holder, no wallet entity).
// The connActor satisfies it; mobs do not (only players have gold
// accounts in practice, §2.3).
//
// Implementations own their own locking: Gold / SetGold may be
// called from the command-dispatch goroutine and must be safe
// against the autosave path reading the same field.
type Entity interface {
	// ID returns the stable identity used in event payloads. Bare
	// ids (no combat prefix) per the engine convention.
	ID() string
	// Gold returns the current balance. Zero is the valid default
	// for a holder that has never been credited (§2.1 "missing
	// entries are treated as zero").
	Gold() int
	// SetGold writes the balance. The service only ever passes a
	// value already floored at zero, so implementations need not
	// re-clamp.
	SetGold(value int)
}

// Sink is the event seam, mirroring progression.AlignmentSink and
// combat.EventSink. cmd/anothermud bridges it to eventbus.Publish.
// Methods fire synchronously from inside the mutating call; a
// blocking implementation stalls the gameplay path.
//
// amount semantics differ by caller: for AddGold it is the absolute
// magnitude of the requested delta (a debit of 5 reports amount=5,
// not -5; a debit of 100 against a balance of 30 still reports
// amount=100 even though only 30 was removed). For SetGold there is
// no delta — amount equals newTotal (the target balance the gold was
// forced to). Subscribers that need the true change for SetGold must
// track prior balance themselves. newTotal is always the
// post-mutation balance.
type Sink interface {
	// OnGoldCredited fires for a non-negative AddGold delta and for
	// every SetGold (spec §2.2 — Set "always emits currency.credited
	// regardless of direction").
	OnGoldCredited(ctx context.Context, entityID string, amount int, reason string, newTotal int)
	// OnGoldDebited fires for a negative AddGold delta (§2.2).
	OnGoldDebited(ctx context.Context, entityID string, amount int, reason string, newTotal int)
}

// nopSink discards every event. Default when NewCurrencyService
// receives a nil sink — keeps tests free of bus boilerplate.
type nopSink struct{}

func (nopSink) OnGoldCredited(context.Context, string, int, string, int) {}
func (nopSink) OnGoldDebited(context.Context, string, int, string, int)  {}

// CurrencyService owns the spec §2.2 operations. Gold lives on the
// entity; the service holds a single mutex that makes each mutation's
// read-modify-write atomic — without it two concurrent AddGold calls
// on the same entity (e.g. a quest reward firing on the tick
// goroutine while the player picks up a coin on the command
// goroutine) could both read the same balance and one write would be
// lost. The lock is process-wide rather than per-entity because gold
// mutations are infrequent and the contention is negligible for a
// MUD; the sink fires OUTSIDE the lock so a slow or re-entrant
// subscriber cannot stall or deadlock the mutation path.
//
// Sink callbacks MUST NOT call back into AddGold / SetGold (the
// publish runs after the lock is released, so a re-entrant call
// would not deadlock, but it would interleave a second mutation
// mid-event and surprise observers). Mirrors the AlignmentSink
// re-entrancy contract.
type CurrencyService struct {
	mu   sync.Mutex
	sink Sink
}

// NewCurrencyService returns a service wired to sink. A nil sink
// becomes a nop so callers that don't observe events (tests) need
// no boilerplate.
func NewCurrencyService(sink Sink) *CurrencyService {
	if sink == nil {
		sink = nopSink{}
	}
	return &CurrencyService{sink: sink}
}

// Read returns the entity's balance, defaulting to zero for a nil
// entity (spec §2.2 Read — "default to zero").
func (s *CurrencyService) Read(e Entity) int {
	if e == nil {
		return 0
	}
	return e.Gold()
}

// AddGold applies delta to the entity's balance, floored at zero
// (spec §2.2). A non-negative delta emits currency.credited; a
// negative delta emits currency.debited. The emitted amount is the
// absolute magnitude of the requested delta, NOT the clamped change
// — a caller debiting 100 against a balance of 30 sees a debit of
// 100 reported even though only 30 was removed, matching the spec's
// "absolute amount" payload. Returns the new total. Nil-safe: a nil
// entity is a no-op returning zero.
//
// Gold floors at zero: a debit that would go negative silently
// clamps. Callers that need "insufficient funds" as a distinct
// outcome MUST gate on Read before calling (§2.2).
func (s *CurrencyService) AddGold(ctx context.Context, e Entity, delta int, reason string) int {
	if e == nil {
		return 0
	}
	// Critical section: the read-compute-write must be atomic against
	// a concurrent mutation on the same entity (see type doc).
	s.mu.Lock()
	next := e.Gold() + delta
	if next < 0 {
		next = 0
	}
	e.SetGold(next)
	s.mu.Unlock()

	// Emit outside the lock so the bus dispatch can't stall the
	// mutation path or re-enter under the held lock.
	amount := delta
	if amount < 0 {
		amount = -amount
	}
	if delta < 0 {
		s.sink.OnGoldDebited(ctx, e.ID(), amount, reason, next)
	} else {
		s.sink.OnGoldCredited(ctx, e.ID(), amount, reason, next)
	}
	return next
}

// Debit atomically charges amount (a positive magnitude) only if the
// entity can afford it. The funds check and the subtraction happen
// under the same lock, so two concurrent debits on the same entity
// can't both pass an "affordable?" gate and over-spend — exactly the
// double-charge a separate Read-then-AddGold sequence is exposed to.
// Returns the resulting balance and whether the charge applied; on
// insufficient funds the balance is returned unchanged with ok=false
// and NO event fires. A negative amount is treated as its magnitude.
// Nil-safe: a nil entity returns (0, false).
//
// This is the primitive shop Buy uses at its charge step (spec §3.5
// step 6): the pre-flight gold gate (step 4) is an early-out for the
// InsufficientGold message, but Debit is the authoritative guard that
// closes the gap between the gate and the actual charge.
func (s *CurrencyService) Debit(ctx context.Context, e Entity, amount int, reason string) (int, bool) {
	if e == nil {
		return 0, false
	}
	if amount < 0 {
		amount = -amount
	}
	s.mu.Lock()
	cur := e.Gold()
	if cur < amount {
		s.mu.Unlock()
		return cur, false
	}
	next := cur - amount
	e.SetGold(next)
	s.mu.Unlock()

	s.sink.OnGoldDebited(ctx, e.ID(), amount, reason, next)
	return next, true
}

// SetGold forces the entity's balance to amount, which MUST be
// non-negative (spec §2.2 — "Throws on negative input"). Always
// emits currency.credited regardless of whether the balance rose or
// fell. Returns ErrNegativeAmount without mutating on negative
// input. Nil-safe: a nil entity is a no-op returning nil.
func (s *CurrencyService) SetGold(ctx context.Context, e Entity, amount int, reason string) error {
	if amount < 0 {
		return ErrNegativeAmount
	}
	if e == nil {
		return nil
	}
	s.mu.Lock()
	e.SetGold(amount)
	s.mu.Unlock()
	s.sink.OnGoldCredited(ctx, e.ID(), amount, reason, amount)
	return nil
}
