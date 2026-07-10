package trade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/escrow"
)

// Sentinel errors surfaced to the verb layer for player-facing messages.
var (
	// ErrAlreadyTrading — the actor is already in a trade session.
	ErrAlreadyTrading = errors.New("trade: already in a trade")
	// ErrPartnerBusy — the target is already in a trade.
	ErrPartnerBusy = errors.New("trade: partner is already trading")
	// ErrSelf — cannot trade with oneself.
	ErrSelf = errors.New("trade: cannot trade with yourself")
	// ErrNotTrading — the actor has no open trade session.
	ErrNotTrading = errors.New("trade: not currently trading")
	// ErrNotInOffer — withdrawing something not in the offer.
	ErrNotInOffer = errors.New("trade: that is not in your offer")
)

// txnFactory builds a fresh escrow transaction for a session over the given
// custodian. Injected so tests can substitute a fake transaction.
type txnFactory func(id string, cus escrow.Custodian) transaction

// Manager owns all open trade sessions and pending requests. A single mutex
// guards its maps AND each session's mutation, held across the escrow
// commit and the player notifications: the lock order is therefore
// Manager.mu → actor.mu (a notification's Write takes the actor lock). Any
// teardown caller (disconnect / room-change hooks) MUST call CancelFor
// WITHOUT holding an actor lock, or the order inverts.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session // playerID -> session (both parties map to it)
	pending  map[string]string   // requester playerID -> target playerID

	newTxn   txnFactory
	coin     CoinMover
	tradable func(entities.EntityID) bool
	describe func(entities.EntityID) string // item id -> display name (for offers)
	money    economy.CurrencyLabel          // currency-label seam: "5 gold" / "5¥" in offers

	seq atomic.Uint64
}

// NewManager wires a Manager. bus + audit drive every session's escrow
// transaction; coin is the currency seam; tradable gates non-tradable items
// (nil → everything tradable); describe renders item names in offer views
// (nil → the raw id); money is the pack's currency label for coin amounts in
// offer messages (zero value → the "gold" default).
func NewManager(bus escrow.Bus, audit *escrow.AuditStore, coin CoinMover, tradable func(entities.EntityID) bool, describe func(entities.EntityID) string, money economy.CurrencyLabel) *Manager {
	return &Manager{
		sessions: map[string]*Session{},
		pending:  map[string]string{},
		newTxn: func(id string, cus escrow.Custodian) transaction {
			return escrow.New(id, cus, bus, escrow.WithAudit(audit, "direct-trade"))
		},
		coin:     coin,
		tradable: tradable,
		describe: describe,
		money:    money,
	}
}

// InSession reports whether the player currently has an open trade.
func (m *Manager) InSession(playerID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[playerID]
	return ok
}

// Initiate is the symmetric handshake: if the target has already requested a
// trade with the caller, it opens the session; otherwise it records the
// caller's request and notifies the target to `trade` back to accept
// (direct-trade §2). Both players must be present (the verb layer enforces
// same-room) and neither may already be trading.
func (m *Manager) Initiate(ctx context.Context, from, to Party) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if from.ID() == to.ID() {
		return ErrSelf
	}
	if _, ok := m.sessions[from.ID()]; ok {
		return ErrAlreadyTrading
	}
	if _, ok := m.sessions[to.ID()]; ok {
		return ErrPartnerBusy
	}

	// Accept path: the target already asked to trade with us.
	if m.pending[to.ID()] == from.ID() {
		delete(m.pending, to.ID())
		delete(m.pending, from.ID())
		m.openLocked(ctx, from, to)
		return nil
	}

	// Request path: record + notify the target.
	m.pending[from.ID()] = to.ID()
	_ = from.Write(ctx, fmt.Sprintf("You offer to trade with %s.", to.Name()))
	_ = to.Write(ctx, fmt.Sprintf("%s wants to trade. Type `trade %s` to accept.", from.Name(), from.Name()))
	return nil
}

// openLocked creates and registers a session between two parties. Caller
// holds m.mu.
func (m *Manager) openLocked(ctx context.Context, a, b Party) {
	cus := newCustodian(a, b, m.coin, m.tradable)
	id := fmt.Sprintf("dt-%d", m.seq.Add(1))
	s := &Session{
		id:  id,
		txn: m.newTxn(id, cus),
		cus: cus,
		a:   &offerSide{party: a},
		b:   &offerSide{party: b},
	}
	m.sessions[a.ID()] = s
	m.sessions[b.ID()] = s
	_ = a.Write(ctx, fmt.Sprintf("Trade with %s opened. Use `offer`, `confirm`, or `decline`.", b.Name()))
	_ = b.Write(ctx, fmt.Sprintf("Trade with %s opened. Use `offer`, `confirm`, or `decline`.", a.Name()))
}

// OfferItem stages an item into the caller's offer and resets both
// confirmations.
func (m *Manager) OfferItem(ctx context.Context, p Party, id entities.EntityID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, mine, _ := m.lookup(p.ID())
	if s == nil {
		return ErrNotTrading
	}
	if err := s.txn.StageItem(ctx, p.ID(), string(id)); err != nil {
		return err
	}
	mine.items = append(mine.items, id)
	m.changedLocked(ctx, s, fmt.Sprintf("%s adds %s to the offer.", p.Name(), m.name(id)))
	return nil
}

// WithdrawItem removes a staged item (by id) from the caller's offer and
// resets both confirmations.
func (m *Manager) WithdrawItem(ctx context.Context, p Party, id entities.EntityID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, mine, _ := m.lookup(p.ID())
	if s == nil {
		return ErrNotTrading
	}
	if !mine.hasItem(id) {
		return ErrNotInOffer
	}
	return m.withdrawItemLocked(ctx, s, mine, p, id)
}

// WithdrawItemByQuery removes a staged item the caller named by keyword. The
// item is no longer in inventory (remove-at-stage), so the verb resolves it
// against the staged offer here rather than via the inventory arg resolver.
func (m *Manager) WithdrawItemByQuery(ctx context.Context, p Party, query string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, mine, _ := m.lookup(p.ID())
	if s == nil {
		return ErrNotTrading
	}
	id, ok := m.matchStaged(mine, query)
	if !ok {
		return ErrNotInOffer
	}
	return m.withdrawItemLocked(ctx, s, mine, p, id)
}

// withdrawItemLocked performs the shared withdraw + offer-mirror update +
// confirm reset. Caller holds m.mu and has verified the item is staged.
func (m *Manager) withdrawItemLocked(ctx context.Context, s *Session, mine *offerSide, p Party, id entities.EntityID) error {
	if err := s.txn.WithdrawItem(ctx, p.ID(), string(id)); err != nil {
		return err
	}
	mine.removeItem(id)
	m.changedLocked(ctx, s, fmt.Sprintf("%s removes %s from the offer.", p.Name(), m.name(id)))
	return nil
}

// matchStaged finds a staged item in the side whose display name matches the
// keyword query (case-insensitive substring; first match wins — the offer is
// small). Caller holds m.mu.
func (m *Manager) matchStaged(side *offerSide, query string) (entities.EntityID, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return "", false
	}
	for _, id := range side.items {
		if strings.Contains(strings.ToLower(m.name(id)), q) {
			return id, true
		}
	}
	return "", false
}

// OfferCoin adds coin to the caller's offer and resets both confirmations.
func (m *Manager) OfferCoin(ctx context.Context, p Party, amount int) error {
	if amount <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, mine, _ := m.lookup(p.ID())
	if s == nil {
		return ErrNotTrading
	}
	if err := s.txn.StageCoin(ctx, p.ID(), amount); err != nil {
		return err
	}
	mine.coin += amount
	m.changedLocked(ctx, s, fmt.Sprintf("%s adds %s to the offer.", p.Name(), m.money.Format(amount)))
	return nil
}

// WithdrawCoin removes coin from the caller's offer and resets both
// confirmations. Clamps to the staged amount.
func (m *Manager) WithdrawCoin(ctx context.Context, p Party, amount int) error {
	if amount <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, mine, _ := m.lookup(p.ID())
	if s == nil {
		return ErrNotTrading
	}
	if mine.coin == 0 {
		return ErrNotInOffer
	}
	if amount > mine.coin {
		amount = mine.coin
	}
	if err := s.txn.WithdrawCoin(ctx, p.ID(), amount); err != nil {
		return err
	}
	mine.coin -= amount
	m.changedLocked(ctx, s, fmt.Sprintf("%s removes %s from the offer.", p.Name(), m.money.Format(amount)))
	return nil
}

// Confirm marks the caller confirmed; when both sides are confirmed against
// an unchanged pair, it commits the swap atomically through escrow. On a
// veto/failure both are made whole, the reason is surfaced, and the session
// stays open to retry; on success the session closes.
func (m *Manager) Confirm(ctx context.Context, p Party) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, mine, theirs := m.lookup(p.ID())
	if s == nil {
		return ErrNotTrading
	}
	mine.confirmed = true
	if !s.bothConfirmed() {
		_ = p.Write(ctx, "You confirm the trade. Waiting on the other party.")
		_ = theirs.party.Write(ctx, fmt.Sprintf("%s has confirmed.", p.Name()))
		return nil
	}

	dest := map[string]string{
		s.a.party.ID(): s.b.party.ID(),
		s.b.party.ID(): s.a.party.ID(),
	}
	err := s.txn.Commit(ctx, dest)
	switch {
	case err == nil:
		// Success: tell each what they received, then close.
		_ = s.a.party.Write(ctx, "Trade complete. "+m.receivedLine(s.b))
		_ = s.b.party.Write(ctx, "Trade complete. "+m.receivedLine(s.a))
		m.removeLocked(s)
		return nil

	case errors.Is(err, escrow.ErrAuditFailed):
		// The swap committed; only the audit write failed. Treat as success
		// (the value moved) but note it — do NOT leave the session open, or a
		// second confirm would hit a finalized transaction.
		_ = s.a.party.Write(ctx, "Trade complete. "+m.receivedLine(s.b)+" (audit log write failed — report this.)")
		_ = s.b.party.Write(ctx, "Trade complete. "+m.receivedLine(s.a)+" (audit log write failed — report this.)")
		m.removeLocked(s)
		return nil

	default:
		// Veto or failure: escrow has returned all value to inventory and
		// finalized the transaction. Rebuild a fresh transaction over the same
		// parties and clear the offer mirrors so the players can re-stage and
		// retry (direct-trade §5).
		s.resetOffers()
		s.txn = m.newTxn(s.id, s.cus)
		reason := "the trade could not be completed"
		if errors.Is(err, escrow.ErrVetoed) {
			reason = "the trade was refused (a pack may be full)"
		}
		_ = s.a.party.Write(ctx, fmt.Sprintf("Trade failed: %s. Offers cleared — re-add and confirm to retry.", reason))
		_ = s.b.party.Write(ctx, fmt.Sprintf("Trade failed: %s. Offers cleared — re-add and confirm to retry.", reason))
		return nil
	}
}

// Cancel ends the caller's trade, returning all staged value to both.
func (m *Manager) Cancel(ctx context.Context, p Party) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, _, _ := m.lookup(p.ID())
	if s == nil {
		// Also clear any outstanding request the caller made.
		delete(m.pending, p.ID())
		return ErrNotTrading
	}
	_ = s.txn.Rollback(ctx)
	_ = s.a.party.Write(ctx, fmt.Sprintf("Trade with %s cancelled.", s.b.party.Name()))
	_ = s.b.party.Write(ctx, fmt.Sprintf("Trade with %s cancelled.", s.a.party.Name()))
	m.removeLocked(s)
	return nil
}

// CancelFor tears down the player's trade (and any pending request) from a
// lifecycle hook (disconnect, link-death, room change). It returns all
// staged value. The caller MUST NOT hold an actor lock (lock order:
// Manager.mu → actor.mu). reason is surfaced to the still-present partner.
func (m *Manager) CancelFor(ctx context.Context, playerID, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Drop the player's own outstanding request AND any request that named
	// this player as its target, so a departed player leaves no stale pending
	// slot pointing at (or from) them.
	delete(m.pending, playerID)
	for requester, target := range m.pending {
		if target == playerID {
			delete(m.pending, requester)
		}
	}
	s, _, theirs := m.lookup(playerID)
	if s == nil {
		return
	}
	_ = s.txn.Rollback(ctx)
	if theirs != nil && theirs.party != nil {
		_ = theirs.party.Write(ctx, fmt.Sprintf("The trade ended: %s.", reason))
	}
	m.removeLocked(s)
}

// lookup returns the caller's session and the two sides, or nils. Caller
// holds m.mu.
func (m *Manager) lookup(playerID string) (s *Session, mine, theirs *offerSide) {
	s = m.sessions[playerID]
	if s == nil {
		return nil, nil, nil
	}
	mine, theirs = s.sideOf(playerID)
	return s, mine, theirs
}

// removeLocked drops a session from the index for both parties. Caller holds
// m.mu.
func (m *Manager) removeLocked(s *Session) {
	delete(m.sessions, s.a.party.ID())
	delete(m.sessions, s.b.party.ID())
}

// changedLocked resets confirmations on any offer change and shows both
// players the new state (direct-trade §4). Caller holds m.mu.
func (m *Manager) changedLocked(ctx context.Context, s *Session, what string) {
	wasConfirmed := s.a.confirmed || s.b.confirmed
	s.clearConfirmations()
	suffix := ""
	if wasConfirmed {
		suffix = " Confirmations reset."
	}
	for _, side := range []*offerSide{s.a, s.b} {
		_ = side.party.Write(ctx, what+suffix+"\n"+m.renderOffers(s, side.party.ID()))
	}
}

// renderOffers formats both offers from the viewer's perspective.
func (m *Manager) renderOffers(s *Session, viewerID string) string {
	mine, theirs := s.sideOf(viewerID)
	return fmt.Sprintf("  Your offer: %s\n  %s's offer: %s",
		m.offerText(mine), theirs.party.Name(), m.offerText(theirs))
}

func (m *Manager) offerText(o *offerSide) string {
	parts := make([]string, 0, len(o.items)+1)
	for _, id := range o.items {
		parts = append(parts, m.name(id))
	}
	if o.coin > 0 {
		parts = append(parts, m.money.Format(o.coin))
	}
	if len(parts) == 0 {
		return "(nothing)"
	}
	return strings.Join(parts, ", ")
}

// receivedLine describes what the viewer received from the other side.
func (m *Manager) receivedLine(from *offerSide) string {
	got := m.offerText(from)
	if got == "(nothing)" {
		return "You received nothing."
	}
	return "You received: " + got + "."
}

// name resolves an item id to a display name via the injected describer,
// falling back to the raw id.
func (m *Manager) name(id entities.EntityID) string {
	if m.describe != nil {
		if n := m.describe(id); n != "" {
			return n
		}
	}
	return string(id)
}
