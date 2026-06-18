// Package escrow is the shared escrow / atomic-transaction primitive both
// player-trade systems consume (docs/specs/trade-escrow.md). It stages
// item instances and coin from one or more parties into a pending, inert
// hold; commits all staged value as one indivisible unit through a
// cancellable bus event; and rolls every party whole on any veto,
// withdraw, cancel, or failure. Every commit appends a tamper-evident
// audit record (audit.go).
//
// Built once, consumed twice. `direct-trade` (synchronous, transient) and
// `auction-house` (asynchronous, persisted) both commit through here and
// neither redefines the guarantee.
//
// The package stays free of session / entities / economy concrete deps by
// operating on two injected seams (mirroring how economy uses Entity/Sink):
//
//   - Custodian moves the actual value in and out of escrow custody. The
//     composition root implements it over connActor + entities.Store +
//     CurrencyService.
//   - Bus publishes the cancellable trade.committing pre-event and the
//     trade.committed fact. *eventbus.Bus satisfies it.
//
// Atomicity note: staging an item physically removes it from the party's
// reach (Custodian.StageItem), so the same instance cannot be staged into
// two transactions at once — the second stage simply fails because the
// party no longer holds it. Coin is likewise removed at stage (Reserve)
// and only credited at commit/return, so reserved coin cannot be spent
// elsewhere.
package escrow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Sentinel errors. Custodian implementations return the stage-time ones so
// callers can surface a precise reason.
var (
	// ErrItemGone is returned when an item can't be staged because the
	// party no longer holds it (already staged elsewhere, dropped, gone).
	ErrItemGone = errors.New("escrow: item not available to stage")
	// ErrItemBound is returned when a non-tradable/bound item is staged.
	// Refused at staging so a party never sees a bound item in escrow (§6).
	ErrItemBound = errors.New("escrow: item is not tradable")
	// ErrAlreadyStaged is returned when the same item is staged twice into
	// one transaction.
	ErrAlreadyStaged = errors.New("escrow: item already staged in this transaction")
	// ErrInsufficientCoin is returned when a party stages more coin than it
	// holds.
	ErrInsufficientCoin = errors.New("escrow: insufficient coin to stage")
	// ErrNotStaged is returned when a withdraw names value not in escrow.
	ErrNotStaged = errors.New("escrow: nothing matching staged to withdraw")
	// ErrVetoed is returned by Commit when a trade.committing subscriber
	// flips the cancel flag; the transaction is rolled back first.
	ErrVetoed = errors.New("escrow: commit vetoed")
	// ErrAlreadyDone is returned by any operation on a transaction that has
	// already committed or rolled back.
	ErrAlreadyDone = errors.New("escrow: transaction already finalized")
	// ErrNoDestination is returned by Commit when a staged party has no
	// destination in the commit map.
	ErrNoDestination = errors.New("escrow: staged party has no commit destination")
	// ErrAuditFailed is returned by Commit when the value moved and the
	// fact event fired but the audit append failed. The trade STANDS — a
	// caller seeing this must log it, not roll back (the value is gone).
	// Distinct from a hard commit failure so the call site can tell them
	// apart.
	ErrAuditFailed = errors.New("escrow: commit succeeded but audit append failed")
)

// Custodian performs the actual movement of value into and out of escrow
// custody. The escrow never touches inventories or gold directly — it tells
// the Custodian what to move. Implemented at the composition root.
//
// StageItem must remove the item from the party's reach (so it is inert and
// cannot be double-staged) and return one of the stage-time sentinels on
// failure. ReturnItem (rollback) and DeliverItem (commit) move a staged
// item to its original party or a destination respectively; they return an
// error only on an unexpected failure (a staged item should always be
// movable). TakeItem removes an item from a holder UNCONDITIONALLY (no
// tradability check) — it is used only to reverse an already-delivered item
// when a later leg fails mid-commit. Coin Return/Deliver cannot fail (a
// credit always applies).
type Custodian interface {
	StageItem(ctx context.Context, partyID, itemID string) error
	ReturnItem(ctx context.Context, partyID, itemID string) error
	DeliverItem(ctx context.Context, destPartyID, itemID string) error
	TakeItem(ctx context.Context, holderID, itemID string) error

	ReserveCoin(ctx context.Context, partyID string, amount int) error
	ReturnCoin(ctx context.Context, partyID string, amount int)
	DeliverCoin(ctx context.Context, destPartyID string, amount int)
}

// Bus is the publish seam the commit rides. *eventbus.Bus satisfies it.
type Bus interface {
	Publish(ctx context.Context, e eventbus.Event)
	PublishCancellable(ctx context.Context, e eventbus.CancellableEvent) bool
}

// legKind tags a staged leg.
type legKind int

const (
	legItem legKind = iota
	legCoin
)

// leg is one staged unit of value owned by a party.
type leg struct {
	party  string
	kind   legKind
	item   string // legItem
	amount int    // legCoin
}

// Transaction is a single pending escrow transaction. It is safe for
// concurrent use: every operation takes the transaction mutex, so a
// withdraw cannot interleave a commit. Commit holds the lock across the
// cancellable publish and all leg moves, so the commit is atomic against a
// concurrent withdraw; subscribers MUST NOT call back into this transaction
// from a trade.committing handler (the bus is synchronous — re-entry would
// deadlock).
type Transaction struct {
	mu  sync.Mutex
	id  string
	cus Custodian
	bus Bus

	audit  *AuditStore // optional
	source string      // consumer name recorded in the audit

	legs []leg
	done bool
}

// Option configures a Transaction at construction.
type Option func(*Transaction)

// WithAudit records every successful commit to store, tagged with the
// source consumer name (e.g. "direct-trade", "auction-sale"). Without it,
// commits are not audited (acceptable for tests; production wires it).
func WithAudit(store *AuditStore, source string) Option {
	return func(t *Transaction) {
		t.audit = store
		t.source = source
	}
}

// New builds a Transaction with the given id, custodian, and bus. A nil bus
// means no veto is possible (commits never cancel) — useful in tests that
// only exercise stage/withdraw.
func New(id string, cus Custodian, bus Bus, opts ...Option) *Transaction {
	t := &Transaction{id: id, cus: cus, bus: bus}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// ID returns the transaction id.
func (t *Transaction) ID() string { return t.id }

// StageItem stages an item instance owned by party into escrow. The item
// becomes inert (removed from the party's reach by the Custodian). Returns
// ErrAlreadyStaged if the item is already in this transaction, or whatever
// the Custodian reports (ErrItemGone / ErrItemBound).
func (t *Transaction) StageItem(ctx context.Context, party, itemID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return ErrAlreadyDone
	}
	for _, l := range t.legs {
		if l.kind == legItem && l.item == itemID {
			return ErrAlreadyStaged
		}
	}
	if err := t.cus.StageItem(ctx, party, itemID); err != nil {
		return err
	}
	t.legs = append(t.legs, leg{party: party, kind: legItem, item: itemID})
	return nil
}

// StageCoin reserves amount of coin from party into escrow. A non-positive
// amount is a no-op (nothing to stage). Returns ErrInsufficientCoin if the
// party cannot cover it. Repeated calls accumulate into a single coin leg
// per party.
func (t *Transaction) StageCoin(ctx context.Context, party string, amount int) error {
	if amount <= 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return ErrAlreadyDone
	}
	if err := t.cus.ReserveCoin(ctx, party, amount); err != nil {
		return err
	}
	for i := range t.legs {
		if t.legs[i].kind == legCoin && t.legs[i].party == party {
			t.legs[i].amount += amount
			return nil
		}
	}
	t.legs = append(t.legs, leg{party: party, kind: legCoin, amount: amount})
	return nil
}

// WithdrawItem removes a staged item from escrow and returns it to its
// staging party before commit. Returns ErrNotStaged if no such item is
// staged.
func (t *Transaction) WithdrawItem(ctx context.Context, party, itemID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return ErrAlreadyDone
	}
	for i, l := range t.legs {
		if l.kind == legItem && l.item == itemID && l.party == party {
			if err := t.cus.ReturnItem(ctx, party, itemID); err != nil {
				return fmt.Errorf("escrow withdraw item %q: %w", itemID, err)
			}
			t.legs = append(t.legs[:i], t.legs[i+1:]...)
			return nil
		}
	}
	return ErrNotStaged
}

// WithdrawCoin returns up to amount of a party's staged coin before commit.
// A non-positive amount is a no-op. Returns ErrNotStaged if the party has
// no staged coin; clamps to the staged amount if amount exceeds it.
func (t *Transaction) WithdrawCoin(ctx context.Context, party string, amount int) error {
	if amount <= 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return ErrAlreadyDone
	}
	for i := range t.legs {
		if t.legs[i].kind == legCoin && t.legs[i].party == party {
			if amount > t.legs[i].amount {
				amount = t.legs[i].amount
			}
			t.cus.ReturnCoin(ctx, party, amount)
			t.legs[i].amount -= amount
			if t.legs[i].amount == 0 {
				t.legs = append(t.legs[:i], t.legs[i+1:]...)
			}
			return nil
		}
	}
	return ErrNotStaged
}

// Commit moves every staged leg to its destination as one indivisible unit.
// dest maps each staging party to the party that receives its legs (e.g. a
// two-party trade passes {A:B, B:A}). The commit:
//
//  1. validates every staged party has a destination;
//  2. publishes the cancellable trade.committing pre-event (the validation
//     seam — capacity/weight/eligibility veto here);
//  3. on a veto, rolls everyone whole and returns ErrVetoed;
//  4. on no veto, delivers every leg, fires trade.committed, and audits.
//
// A pre-flight rejection (ErrNoDestination) leaves the transaction LIVE —
// nothing was published and nothing moved, so the caller may fix the
// destination map and retry, or roll back. After a veto or a successful
// commit the transaction is finalized and further operations return
// ErrAlreadyDone.
//
// Delivery order is items-first, coin-last. DeliverItem is the only leg that
// can fail; coin delivery cannot. By moving every item before any coin, a
// mid-commit failure can only happen while no coin has moved yet — so the
// unwind never has to reverse a coin credit (which would double-credit,
// since coin delivery is a credit with no escrow-side debit).
func (t *Transaction) Commit(ctx context.Context, dest map[string]string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return ErrAlreadyDone
	}

	// Every staged party must have a destination, or we cannot move its
	// legs — fail before publishing so a misconfigured commit costs nothing
	// and leaves the transaction live for a corrected retry.
	for _, l := range t.legs {
		if _, ok := dest[l.party]; !ok {
			return fmt.Errorf("%w: %q", ErrNoDestination, l.party)
		}
	}

	busLegs := t.busLegs(dest)

	// Validation seam: any subscriber may veto before a single leg moves.
	if t.bus != nil {
		if cancelled := t.bus.PublishCancellable(ctx, eventbus.NewTradeCommitting(t.id, busLegs)); cancelled {
			t.rollbackLocked(ctx)
			return ErrVetoed
		}
	}

	if err := t.deliverLocked(ctx, dest); err != nil {
		t.legs = nil
		t.done = true
		return err
	}

	t.legs = nil
	t.done = true

	if t.bus != nil {
		t.bus.Publish(ctx, eventbus.TradeCommitted{TxnID: t.id, Legs: busLegs})
	}
	if t.audit != nil {
		if err := t.audit.Append(t.auditRecord(busLegs)); err != nil {
			// The trade committed; a failed audit write must not undo it
			// (that would lose real value to a bookkeeping error). Surface a
			// distinct sentinel so the caller logs rather than rolls back.
			logging.From(ctx).Error("escrow: audit append failed after commit",
				slog.String("txn", t.id), slog.String("err", err.Error()))
			return fmt.Errorf("%w: %v", ErrAuditFailed, err)
		}
	}
	return nil
}

// deliverLocked moves every staged leg to its destination, items first then
// coin, and makes everyone whole on a delivery failure. DeliverItem is the
// only failable leg; delivering all items before any coin means a mid-commit
// failure happens while no coin has moved, so the unwind never reverses a
// coin credit (which would double-credit). Caller holds t.mu and finalizes
// the transaction regardless of the result.
func (t *Transaction) deliverLocked(ctx context.Context, dest map[string]string) error {
	var items, coins []leg
	for _, l := range t.legs {
		if l.kind == legItem {
			items = append(items, l)
		} else {
			coins = append(coins, l)
		}
	}

	// Phase 1: deliver items. On a failure at item k, make everyone whole:
	// pull back the items already delivered, return the still-in-custody
	// items (k and after) to their owners, and return every still-reserved
	// coin. No coin has been delivered yet, so there is no coin double-credit.
	for k, l := range items {
		if err := t.cus.DeliverItem(ctx, dest[l.party], l.item); err != nil {
			t.makeWholeAfterItemFailureLocked(ctx, dest, items, k, coins)
			return fmt.Errorf("escrow commit deliver item %q: %w", l.item, err)
		}
	}

	// Phase 2: deliver coin (cannot fail).
	for _, l := range coins {
		t.cus.DeliverCoin(ctx, dest[l.party], l.amount)
	}
	return nil
}

// Rollback returns every staged leg to its owner and finalizes the
// transaction. Safe to call on an already-finalized transaction (no-op).
// This is the explicit cancel/teardown path (abandoned trade, expired
// listing).
func (t *Transaction) Rollback(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return nil
	}
	t.rollbackLocked(ctx)
	return nil
}

// rollbackLocked returns all staged legs to their owners and marks the
// transaction done. Caller holds t.mu. A return failure breaks the
// make-whole guarantee, so it is logged at ERROR rather than swallowed.
func (t *Transaction) rollbackLocked(ctx context.Context) {
	for _, l := range t.legs {
		if l.kind == legItem {
			if err := t.cus.ReturnItem(ctx, l.party, l.item); err != nil {
				t.logReturnFailure(ctx, l.party, l.item, err)
			}
		} else {
			t.cus.ReturnCoin(ctx, l.party, l.amount)
		}
	}
	t.legs = nil
	t.done = true
}

// makeWholeAfterItemFailureLocked restores everyone after a mid-commit item
// delivery fails at items[k]. Items [0,k) were delivered to their
// destinations — take them back and return them to their owners. Item k and
// after are still in escrow custody (delivery failed / not attempted) —
// return them to their owners. No coin has been delivered yet, so every coin
// leg is still reserved and is simply returned. Caller holds t.mu.
func (t *Transaction) makeWholeAfterItemFailureLocked(ctx context.Context, dest map[string]string, items []leg, k int, coins []leg) {
	for i, l := range items {
		if i < k {
			// Delivered to dest — pull it back (unconditionally, no
			// tradability re-check) and return to the owner.
			if err := t.cus.TakeItem(ctx, dest[l.party], l.item); err != nil {
				t.logReturnFailure(ctx, dest[l.party], l.item, err)
				continue
			}
			if err := t.cus.ReturnItem(ctx, l.party, l.item); err != nil {
				t.logReturnFailure(ctx, l.party, l.item, err)
			}
		} else {
			// Still in custody (k failed; >k never attempted).
			if err := t.cus.ReturnItem(ctx, l.party, l.item); err != nil {
				t.logReturnFailure(ctx, l.party, l.item, err)
			}
		}
	}
	for _, l := range coins {
		t.cus.ReturnCoin(ctx, l.party, l.amount)
	}
}

// logReturnFailure records a broken make-whole return at ERROR — a staged
// item that could not be returned to its owner is real value stuck in limbo
// and an operator must intervene (the audit log is the recovery aid).
func (t *Transaction) logReturnFailure(ctx context.Context, party, item string, err error) {
	logging.From(ctx).Error("escrow: failed to return staged item — value may be stranded",
		slog.String("txn", t.id), slog.String("party", party),
		slog.String("item", item), slog.String("err", err.Error()))
}

// busLegs projects the internal legs onto the event wire shape.
func (t *Transaction) busLegs(dest map[string]string) []eventbus.TradeLeg {
	out := make([]eventbus.TradeLeg, 0, len(t.legs))
	for _, l := range t.legs {
		bl := eventbus.TradeLeg{PartyID: l.party, DestPartyID: dest[l.party]}
		if l.kind == legItem {
			bl.Kind = eventbus.TradeLegItem
			bl.ItemID = l.item
		} else {
			bl.Kind = eventbus.TradeLegCoin
			bl.Amount = l.amount
		}
		out = append(out, bl)
	}
	return out
}
