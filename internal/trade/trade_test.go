package trade

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// fakeParty is a test connActor: gold + an inventory set. Satisfies
// trade.Party.
type fakeParty struct {
	id   string
	name string
	gold int
	inv  map[entities.EntityID]bool
	out  []string
}

func newParty(id, name string) *fakeParty {
	return &fakeParty{id: id, name: name, inv: map[entities.EntityID]bool{}}
}

func (p *fakeParty) ID() string                              { return p.id }
func (p *fakeParty) Name() string                            { return p.name }
func (p *fakeParty) Gold() int                               { return p.gold }
func (p *fakeParty) SetGold(v int)                           { p.gold = v }
func (p *fakeParty) Write(_ context.Context, m string) error { p.out = append(p.out, m); return nil }
func (p *fakeParty) AddToInventory(id entities.EntityID)     { p.inv[id] = true }

func (p *fakeParty) RemoveFromInventory(id entities.EntityID) bool {
	if !p.inv[id] {
		return false
	}
	delete(p.inv, id)
	return true
}

func (p *fakeParty) Inventory() []entities.EntityID {
	out := make([]entities.EntityID, 0, len(p.inv))
	for id := range p.inv {
		out = append(out, id)
	}
	return out
}

// fakeBus records committed events and optionally vetoes.
type fakeBus struct {
	veto      bool
	committed int
}

func (b *fakeBus) Publish(_ context.Context, e eventbus.Event) {
	if _, ok := e.(eventbus.TradeCommitted); ok {
		b.committed++
	}
}

func (b *fakeBus) PublishCancellable(_ context.Context, e eventbus.CancellableEvent) bool {
	if b.veto {
		e.Cancel()
	}
	return e.Cancelled()
}

// newTestManager wires a Manager over a real CurrencyService and the given
// bus, with everything tradable unless in the bound set.
func newTestManager(bus *fakeBus, bound map[entities.EntityID]bool) *Manager {
	cur := economy.NewCurrencyService(nil)
	tradable := func(id entities.EntityID) bool { return !bound[id] }
	return NewManager(bus, nil, cur, tradable, nil, economy.DefaultCurrency)
}

// A coin offer is announced through the pack's currency label (currency-label
// seam), so a Shadowrun-style label shows "50¥", never "50 gold".
func TestOfferCoin_UsesCurrencyLabel(t *testing.T) {
	ctx := context.Background()
	cur := economy.NewCurrencyService(nil)
	m := NewManager(&fakeBus{}, nil, cur, nil, nil, economy.CurrencyLabel{Noun: "nuyen", Suffix: "¥"})
	a := newParty("A", "Alice")
	b := newParty("B", "Bob")
	b.gold = 100

	open(t, m, a, b)
	if err := m.OfferCoin(ctx, b, 50); err != nil {
		t.Fatalf("offer coin: %v", err)
	}
	// Alice (the counterparty) sees Bob's coin offer announced.
	var saw string
	for _, line := range a.out {
		if strings.Contains(line, "adds") {
			saw = line
		}
	}
	if !strings.Contains(saw, "50¥") || strings.Contains(saw, "gold") {
		t.Errorf("coin offer message = %q, want it to show 50¥ (not gold)", saw)
	}
}

// open is a test helper that runs the symmetric handshake to an open session.
func open(t *testing.T, m *Manager, a, b *fakeParty) {
	t.Helper()
	ctx := context.Background()
	if err := m.Initiate(ctx, a, b); err != nil {
		t.Fatalf("initiate a→b: %v", err)
	}
	if m.InSession(a.id) {
		t.Fatal("session should not open on the first request")
	}
	if err := m.Initiate(ctx, b, a); err != nil {
		t.Fatalf("initiate b→a (accept): %v", err)
	}
	if !m.InSession(a.id) || !m.InSession(b.id) {
		t.Fatal("session should be open after the handshake")
	}
}

func TestSwap_HappyPath(t *testing.T) {
	ctx := context.Background()
	bus := &fakeBus{}
	m := newTestManager(bus, nil)
	a := newParty("A", "Alice")
	a.inv["sword"] = true
	b := newParty("B", "Bob")
	b.gold = 100

	open(t, m, a, b)
	if err := m.OfferItem(ctx, a, "sword"); err != nil {
		t.Fatalf("offer item: %v", err)
	}
	if err := m.OfferCoin(ctx, b, 50); err != nil {
		t.Fatalf("offer coin: %v", err)
	}
	if err := m.Confirm(ctx, a); err != nil {
		t.Fatalf("confirm a: %v", err)
	}
	if err := m.Confirm(ctx, b); err != nil {
		t.Fatalf("confirm b: %v", err)
	}

	if a.inv["sword"] || !b.inv["sword"] {
		t.Error("sword should have moved A→B")
	}
	if a.gold != 50 || b.gold != 50 {
		t.Errorf("gold = A:%d B:%d, want 50/50", a.gold, b.gold)
	}
	if m.InSession(a.id) || m.InSession(b.id) {
		t.Error("session should be closed after a completed swap")
	}
	if bus.committed != 1 {
		t.Errorf("committed events = %d, want 1", bus.committed)
	}
}

func TestStage_RemovedFromInventory(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(&fakeBus{}, nil)
	a := newParty("A", "Alice")
	a.inv["sword"] = true
	b := newParty("B", "Bob")
	open(t, m, a, b)

	if err := m.OfferItem(ctx, a, "sword"); err != nil {
		t.Fatalf("offer: %v", err)
	}
	// Remove-at-stage: the item leaves A's inventory entirely (so no other
	// verb can reach it).
	if a.inv["sword"] {
		t.Error("staged item must leave the owner's inventory (remove-at-stage)")
	}
	// Withdraw returns it to A.
	if err := m.WithdrawItem(ctx, a, "sword"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if !a.inv["sword"] {
		t.Error("withdrawn item must return to the owner")
	}
}

func TestStage_CoinDebitedAtStage(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(&fakeBus{}, nil)
	a := newParty("A", "Alice")
	a.gold = 100
	b := newParty("B", "Bob")
	open(t, m, a, b)

	if err := m.OfferCoin(ctx, a, 40); err != nil {
		t.Fatalf("offer coin: %v", err)
	}
	if a.gold != 60 {
		t.Errorf("coin should be debited at stage; A gold = %d, want 60", a.gold)
	}
	if err := m.WithdrawCoin(ctx, a, 40); err != nil {
		t.Fatalf("withdraw coin: %v", err)
	}
	if a.gold != 100 {
		t.Errorf("withdrawn coin should return; A gold = %d, want 100", a.gold)
	}
}

func TestConfirm_ResetOnChange(t *testing.T) {
	ctx := context.Background()
	bus := &fakeBus{}
	m := newTestManager(bus, nil)
	a := newParty("A", "Alice")
	a.inv["sword"] = true
	b := newParty("B", "Bob")
	b.gold = 100
	open(t, m, a, b)

	_ = m.OfferItem(ctx, a, "sword")
	_ = m.OfferCoin(ctx, b, 50)
	if err := m.Confirm(ctx, a); err != nil {
		t.Fatalf("confirm a: %v", err)
	}
	// B changes the offer — this must reset A's confirmation.
	if err := m.OfferCoin(ctx, b, 10); err != nil {
		t.Fatalf("offer more coin: %v", err)
	}
	// B confirming now must NOT commit (A's confirm was reset).
	if err := m.Confirm(ctx, b); err != nil {
		t.Fatalf("confirm b: %v", err)
	}
	if bus.committed != 0 || b.inv["sword"] {
		t.Fatal("a change after confirm must prevent the swap from firing")
	}
	// Re-confirm both → now it commits.
	_ = m.Confirm(ctx, a)
	if bus.committed != 1 || !b.inv["sword"] {
		t.Errorf("swap should fire once both re-confirm; committed=%d", bus.committed)
	}
	if a.gold != 60 {
		t.Errorf("A gold = %d, want 60 (50+10)", a.gold)
	}
}

func TestCommit_VetoMakesWholeAndReopens(t *testing.T) {
	ctx := context.Background()
	bus := &fakeBus{veto: true}
	m := newTestManager(bus, nil)
	a := newParty("A", "Alice")
	a.inv["sword"] = true
	b := newParty("B", "Bob")
	b.gold = 100
	open(t, m, a, b)

	_ = m.OfferItem(ctx, a, "sword")
	_ = m.OfferCoin(ctx, b, 50)
	_ = m.Confirm(ctx, a)
	_ = m.Confirm(ctx, b) // triggers the (vetoed) commit

	if !a.inv["sword"] || b.inv["sword"] {
		t.Error("vetoed swap must leave the sword with A")
	}
	if a.gold != 0 || b.gold != 100 {
		t.Errorf("vetoed swap must make whole; A:%d B:%d want 0/100", a.gold, b.gold)
	}
	if !m.InSession(a.id) {
		t.Error("session must stay open after a vetoed commit (retry)")
	}
	if bus.committed != 0 {
		t.Error("no committed event on veto")
	}
}

func TestVeto_RetryAfterReset(t *testing.T) {
	ctx := context.Background()
	bus := &fakeBus{veto: true}
	m := newTestManager(bus, nil)
	a := newParty("A", "Alice")
	a.inv["sword"] = true
	b := newParty("B", "Bob")
	b.gold = 100
	open(t, m, a, b)

	_ = m.OfferItem(ctx, a, "sword")
	_ = m.OfferCoin(ctx, b, 50)
	_ = m.Confirm(ctx, a)
	_ = m.Confirm(ctx, b) // vetoed → made whole + session reset for retry

	// The session must still accept a fresh offer (the txn was rebuilt) and a
	// second swap must succeed once the veto is lifted.
	bus.veto = false
	if err := m.OfferItem(ctx, a, "sword"); err != nil {
		t.Fatalf("re-offer after veto must work (txn rebuilt): %v", err)
	}
	_ = m.OfferCoin(ctx, b, 50)
	_ = m.Confirm(ctx, a)
	if err := m.Confirm(ctx, b); err != nil {
		t.Fatalf("retry confirm: %v", err)
	}
	if !b.inv["sword"] || a.gold != 50 || b.gold != 50 {
		t.Errorf("retry swap did not complete: bInv=%v aGold=%d bGold=%d", b.inv["sword"], a.gold, b.gold)
	}
	if bus.committed != 1 {
		t.Errorf("exactly one successful commit expected; got %d", bus.committed)
	}
}

func TestRescindByQuery(t *testing.T) {
	ctx := context.Background()
	// describe maps the id to a display name so the keyword match has something
	// to resolve against.
	cur := economy.NewCurrencyService(nil)
	m := NewManager(&fakeBus{}, nil, cur, nil, func(id entities.EntityID) string {
		if id == "sword-1" {
			return "a steel sword"
		}
		return string(id)
	}, economy.DefaultCurrency)
	a := newParty("A", "Alice")
	a.inv["sword-1"] = true
	b := newParty("B", "Bob")
	open(t, m, a, b)

	if err := m.OfferItem(ctx, a, "sword-1"); err != nil {
		t.Fatalf("offer: %v", err)
	}
	if a.inv["sword-1"] {
		t.Fatal("staged item should have left inventory")
	}
	// Rescind by keyword (the item is no longer in inventory).
	if err := m.WithdrawItemByQuery(ctx, a, "sword"); err != nil {
		t.Fatalf("rescind by query: %v", err)
	}
	if !a.inv["sword-1"] {
		t.Error("rescinded item must return to inventory")
	}
	// A non-matching query reports not-in-offer.
	_ = m.OfferItem(ctx, a, "sword-1")
	if err := m.WithdrawItemByQuery(ctx, a, "axe"); err != ErrNotInOffer {
		t.Errorf("non-matching rescind = %v, want ErrNotInOffer", err)
	}
}

func TestCancel_ReturnsAll(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(&fakeBus{}, nil)
	a := newParty("A", "Alice")
	a.inv["sword"] = true
	a.gold = 30
	b := newParty("B", "Bob")
	open(t, m, a, b)

	_ = m.OfferItem(ctx, a, "sword")
	_ = m.OfferCoin(ctx, a, 30)
	if err := m.Cancel(ctx, a); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !a.inv["sword"] || a.gold != 30 {
		t.Errorf("cancel must restore A fully: inv=%v gold=%d", a.inv["sword"], a.gold)
	}
	if m.InSession(a.id) {
		t.Error("session must be gone after cancel")
	}
}

func TestCancelFor_TeardownReturnsAll(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(&fakeBus{}, nil)
	a := newParty("A", "Alice")
	a.gold = 80
	b := newParty("B", "Bob")
	open(t, m, a, b)
	_ = m.OfferCoin(ctx, a, 80)

	// Simulate a disconnect/room-change teardown for A.
	m.CancelFor(ctx, a.id, "Alice left")
	if a.gold != 80 {
		t.Errorf("teardown must return staged coin; A gold = %d, want 80", a.gold)
	}
	if m.InSession(a.id) || m.InSession(b.id) {
		t.Error("teardown must close the session for both parties")
	}
}

func TestInitiate_Guards(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(&fakeBus{}, nil)
	a := newParty("A", "Alice")
	b := newParty("B", "Bob")
	c := newParty("C", "Carol")

	if err := m.Initiate(ctx, a, a); err != ErrSelf {
		t.Errorf("self-trade = %v, want ErrSelf", err)
	}
	open(t, m, a, b)
	if err := m.Initiate(ctx, a, c); err != ErrAlreadyTrading {
		t.Errorf("already-trading = %v, want ErrAlreadyTrading", err)
	}
	if err := m.Initiate(ctx, c, a); err != ErrPartnerBusy {
		t.Errorf("partner-busy = %v, want ErrPartnerBusy", err)
	}
}

func TestOffer_BoundRefused(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(&fakeBus{}, map[entities.EntityID]bool{"heirloom": true})
	a := newParty("A", "Alice")
	a.inv["heirloom"] = true
	b := newParty("B", "Bob")
	open(t, m, a, b)

	if err := m.OfferItem(ctx, a, "heirloom"); err == nil {
		t.Fatal("a bound item must be refused from an offer")
	}
	if !a.inv["heirloom"] {
		t.Error("a refused item must stay in the owner's inventory")
	}
}

func TestOffer_NotTrading(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(&fakeBus{}, nil)
	a := newParty("A", "Alice")
	a.inv["sword"] = true
	if err := m.OfferItem(ctx, a, "sword"); err != ErrNotTrading {
		t.Errorf("offer with no session = %v, want ErrNotTrading", err)
	}
}
