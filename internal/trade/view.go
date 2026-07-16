package trade

// This file holds the read-only projection of an open trade (web-client-plan P3
// Slice B++) — the Char.Trade form. It mirrors what the `trade` verb's offer
// text shows (renderOffers/offerText), but as structured data a rich client
// renders into a two-column live panel and submits against with the existing
// offer/rescind/confirm/decline verbs (the authority invariant). No mutation.

// TradeItem is one staged item in a TradeView: its runtime entity id (an opaque
// row key) and display name.
type TradeItem struct {
	ID   string
	Name string
}

// TradeOffer is one side's staged half of an open trade (read-only): the party's
// display name, the items staged into the offer, the formatted coin ("" when
// none — the caller's CurrencyLabel already applied), and whether that side has
// confirmed the CURRENT pair (any offer change clears it — the bait-and-switch
// guard, direct-trade §4).
type TradeOffer struct {
	Party     string
	Items     []TradeItem
	Coin      string
	Confirmed bool
}

// TradeView is the read-only projection of a player's open trade (web-client-plan
// P3 Slice B++), for the Char.Trade panel. Open is false when the player has no
// trade in progress (the panel hides). Mine/Theirs are from the viewer's
// perspective, so the same session projects differently for each party.
type TradeView struct {
	Open   bool
	Mine   TradeOffer
	Theirs TradeOffer
}

// View snapshots the viewer's open trade into a read-only TradeView. Returns
// {Open: false} when the player is not trading (the client hides the panel).
// Taken under m.mu; the name/money helpers it calls are pure (no actor lock), so
// unlike the mutating verbs this never reaches actor.mu and is safe to call from
// the GMCP flush pass with no lock held.
func (m *Manager) View(playerID string) TradeView {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, mine, theirs := m.lookup(playerID)
	if s == nil {
		return TradeView{Open: false}
	}
	return TradeView{
		Open:   true,
		Mine:   m.offerView(mine),
		Theirs: m.offerView(theirs),
	}
}

// offerView projects one side into a read-only TradeOffer, resolving item ids to
// display names (the same describer offerText uses) and formatting coin through
// the pack CurrencyLabel. Caller holds m.mu.
func (m *Manager) offerView(o *offerSide) TradeOffer {
	items := make([]TradeItem, 0, len(o.items))
	for _, id := range o.items {
		items = append(items, TradeItem{ID: string(id), Name: m.name(id)})
	}
	coin := ""
	if o.coin > 0 {
		coin = m.money.Format(o.coin)
	}
	return TradeOffer{
		Party:     o.party.Name(),
		Items:     items,
		Coin:      coin,
		Confirmed: o.confirmed,
	}
}
