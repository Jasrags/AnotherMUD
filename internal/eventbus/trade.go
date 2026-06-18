package eventbus

// Trade transaction events — the escrow/atomic-transaction primitive
// (docs/specs/trade-escrow.md §3). Both player-trade consumers
// (direct-trade, auction-house) commit through these:
//
//   - trade.committing — cancellable pre-event. The escrow publishes it
//     after all value is staged and BEFORE any leg moves, carrying every
//     party and leg with its destination. Any subscriber may veto by
//     flipping the cancel flag — this is the validation seam (recipient
//     capacity/weight, a late tradability change, eligibility), not a
//     separate validation pass (§3).
//   - trade.committed — non-cancellable fact. Fired after the indivisible
//     commit; subscribers that react to a completed trade (quests, stats,
//     GMCP) listen here (§3).
//
// The leg shape here is self-contained (plain fields) so eventbus stays
// free of any dependency on the escrow package — the escrow constructs
// these from its own internal model.
const (
	// EventTradeCommitting is the cancellable pre-event (§3).
	EventTradeCommitting = "trade.committing"
	// EventTradeCommitted is the non-cancellable fact event (§3).
	EventTradeCommitted = "trade.committed"
)

// TradeLegKind distinguishes an item leg from a coin leg in a trade
// transaction. Spec trade-escrow §2 (items and coin both stage).
const (
	TradeLegItem = "item"
	TradeLegCoin = "coin"
)

// TradeLeg is one staged unit of value moving in a transaction: a party's
// item instance or coin amount, and the party it moves to on commit.
// Kind is TradeLegItem or TradeLegCoin; ItemID is set for an item leg,
// Amount for a coin leg.
type TradeLeg struct {
	PartyID     string
	DestPartyID string
	Kind        string
	ItemID      string
	Amount      int
}

// TradeCommitting is the cancellable pre-event carrying the full pending
// transaction (every party, every leg, each leg's destination) before any
// value moves. A listener calls Cancel() to veto; the escrow checks
// Cancelled() after dispatch and rolls everyone whole on a veto (§3/§4).
//
// The CancelFlag is a pointer so siblings later in the dispatch loop
// observe a prior cancel; NewTradeCommitting wires it up (a zero-value
// struct would carry a nil flag and panic on Cancel).
type TradeCommitting struct {
	*CancelFlag
	TxnID string
	Legs  []TradeLeg
}

// Name implements Event.
func (TradeCommitting) Name() string { return EventTradeCommitting }

// NewTradeCommitting wires the cancel flag so the publisher gets a usable
// veto (mirrors NewEntityEquipping / NewContainerItemAdding).
func NewTradeCommitting(txnID string, legs []TradeLeg) *TradeCommitting {
	return &TradeCommitting{
		CancelFlag: &CancelFlag{},
		TxnID:      txnID,
		Legs:       legs,
	}
}

// TradeCommitted fires after a transaction commits as one indivisible
// unit (§3). Payload mirrors the pre-event so a fact-listener sees exactly
// what moved. Not cancellable — the trade has already happened.
type TradeCommitted struct {
	TxnID string
	Legs  []TradeLeg
}

// Name implements Event.
func (TradeCommitted) Name() string { return EventTradeCommitted }
