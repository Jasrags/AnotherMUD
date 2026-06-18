package trade

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/escrow"
)

// offerSide is one player's half of an open trade: their handle, the items
// and coin currently staged into their offer, and whether they have
// confirmed the current pair.
type offerSide struct {
	party     Party
	items     []entities.EntityID
	coin      int
	confirmed bool
}

func (o *offerSide) hasItem(id entities.EntityID) bool {
	for _, have := range o.items {
		if have == id {
			return true
		}
	}
	return false
}

func (o *offerSide) removeItem(id entities.EntityID) bool {
	for i, have := range o.items {
		if have == id {
			o.items = append(o.items[:i], o.items[i+1:]...)
			return true
		}
	}
	return false
}

// Session is one open direct trade between two present players. It is
// transient — never persisted — and always accessed under the Manager mutex,
// so it carries no lock of its own.
type Session struct {
	id  string
	txn transaction      // the escrow transaction holding both offers in custody
	cus escrow.Custodian // retained so a vetoed txn can be rebuilt for a retry
	a   *offerSide
	b   *offerSide
}

// resetOffers clears both sides' staged-offer mirrors and confirmations —
// used after a veto, when escrow has already returned all value to inventory,
// so the retry starts from a clean baseline.
func (s *Session) resetOffers() {
	s.a.items, s.a.coin = nil, 0
	s.b.items, s.b.coin = nil, 0
	s.clearConfirmations()
}

// transaction is the slice of escrow.Transaction the session drives. A
// narrow interface keeps the session testable without a live escrow + bus.
type transaction interface {
	StageItem(ctx context.Context, party, itemID string) error
	WithdrawItem(ctx context.Context, party, itemID string) error
	StageCoin(ctx context.Context, party string, amount int) error
	WithdrawCoin(ctx context.Context, party string, amount int) error
	Commit(ctx context.Context, dest map[string]string) error
	Rollback(ctx context.Context) error
}

// sideOf returns the caller's side and the other side, or (nil, nil) if the
// player id is not part of this session.
func (s *Session) sideOf(playerID string) (mine, theirs *offerSide) {
	switch playerID {
	case s.a.party.ID():
		return s.a, s.b
	case s.b.party.ID():
		return s.b, s.a
	}
	return nil, nil
}

// clearConfirmations resets both sides — invoked on any offer change so a
// swap can never fire on a stale confirmation (direct-trade §4, the
// bait-and-switch guard).
func (s *Session) clearConfirmations() {
	s.a.confirmed = false
	s.b.confirmed = false
}

func (s *Session) bothConfirmed() bool { return s.a.confirmed && s.b.confirmed }
