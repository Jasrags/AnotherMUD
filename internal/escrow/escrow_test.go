package escrow

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// fakeCustodian models inventories and gold so the tests exercise the real
// invariant: staging an item physically removes it from the owner (which is
// what makes cross-transaction double-staging impossible), and coin leaves
// the balance at stage and only reappears at commit/return.
var errDeliver = errors.New("fake: deliver failed")

type fakeCustodian struct {
	mu          sync.Mutex
	inv         map[string]map[string]bool // party -> set of item ids held
	gold        map[string]int             // party -> balance
	bound       map[string]bool            // item ids that are non-tradable
	failDeliver string                     // item id whose DeliverItem fails
}

func newFakeCustodian() *fakeCustodian {
	return &fakeCustodian{
		inv:   map[string]map[string]bool{},
		gold:  map[string]int{},
		bound: map[string]bool{},
	}
}

func (f *fakeCustodian) give(party, item string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inv[party] == nil {
		f.inv[party] = map[string]bool{}
	}
	f.inv[party][item] = true
}

func (f *fakeCustodian) setGold(party string, amount int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gold[party] = amount
}

func (f *fakeCustodian) has(party, item string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inv[party][item]
}

func (f *fakeCustodian) balance(party string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gold[party]
}

func (f *fakeCustodian) StageItem(_ context.Context, party, item string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bound[item] {
		return ErrItemBound
	}
	if !f.inv[party][item] {
		return ErrItemGone
	}
	delete(f.inv[party], item)
	return nil
}

func (f *fakeCustodian) ReturnItem(_ context.Context, party, item string) error {
	f.give(party, item)
	return nil
}

func (f *fakeCustodian) DeliverItem(_ context.Context, dest, item string) error {
	if f.failDeliver != "" && item == f.failDeliver {
		return errDeliver
	}
	f.give(dest, item)
	return nil
}

func (f *fakeCustodian) TakeItem(_ context.Context, holder, item string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.inv[holder][item] {
		return ErrItemGone
	}
	delete(f.inv[holder], item)
	return nil
}

func (f *fakeCustodian) ReserveCoin(_ context.Context, party string, amount int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gold[party] < amount {
		return ErrInsufficientCoin
	}
	f.gold[party] -= amount
	return nil
}

func (f *fakeCustodian) ReturnCoin(_ context.Context, party string, amount int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gold[party] += amount
}

func (f *fakeCustodian) DeliverCoin(_ context.Context, dest string, amount int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gold[dest] += amount
}

// fakeBus records published events and optionally vetoes the pre-event.
type fakeBus struct {
	mu        sync.Mutex
	veto      bool
	committed int
	lastLegs  []eventbus.TradeLeg
}

func (b *fakeBus) Publish(_ context.Context, e eventbus.Event) {
	if c, ok := e.(eventbus.TradeCommitted); ok {
		b.mu.Lock()
		b.committed++
		b.lastLegs = c.Legs
		b.mu.Unlock()
	}
}

func (b *fakeBus) PublishCancellable(_ context.Context, e eventbus.CancellableEvent) bool {
	if b.veto {
		e.Cancel()
	}
	return e.Cancelled()
}

func (b *fakeBus) commits() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.committed
}

func TestCommit_TwoPartySwap(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "sword")
	cus.setGold("B", 100)
	bus := &fakeBus{}

	tx := New("t1", cus, bus)
	if err := tx.StageItem(ctx, "A", "sword"); err != nil {
		t.Fatalf("stage item: %v", err)
	}
	if err := tx.StageCoin(ctx, "B", 50); err != nil {
		t.Fatalf("stage coin: %v", err)
	}

	if err := tx.Commit(ctx, map[string]string{"A": "B", "B": "A"}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if !cus.has("B", "sword") {
		t.Error("B should hold the sword after the swap")
	}
	if cus.has("A", "sword") {
		t.Error("A should no longer hold the sword")
	}
	if got := cus.balance("A"); got != 50 {
		t.Errorf("A gold = %d, want 50", got)
	}
	if got := cus.balance("B"); got != 50 {
		t.Errorf("B gold = %d, want 50", got)
	}
	if bus.commits() != 1 {
		t.Errorf("trade.committed fired %d times, want 1", bus.commits())
	}
}

func TestCommit_VetoRollsEveryoneWhole(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "sword")
	cus.setGold("B", 100)
	bus := &fakeBus{veto: true}

	tx := New("t1", cus, bus)
	_ = tx.StageItem(ctx, "A", "sword")
	_ = tx.StageCoin(ctx, "B", 50)

	err := tx.Commit(ctx, map[string]string{"A": "B", "B": "A"})
	if !errors.Is(err, ErrVetoed) {
		t.Fatalf("commit err = %v, want ErrVetoed", err)
	}

	if !cus.has("A", "sword") {
		t.Error("A should hold the sword back after a veto")
	}
	if cus.has("B", "sword") {
		t.Error("B must not receive the sword on a veto")
	}
	if got := cus.balance("B"); got != 100 {
		t.Errorf("B gold = %d, want 100 (made whole)", got)
	}
	if got := cus.balance("A"); got != 0 {
		t.Errorf("A gold = %d, want 0", got)
	}
	if bus.commits() != 0 {
		t.Error("trade.committed must not fire on a veto")
	}
}

func TestWithdrawItem_ReturnsIntact(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "sword")
	tx := New("t1", cus, &fakeBus{})

	if err := tx.StageItem(ctx, "A", "sword"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if cus.has("A", "sword") {
		t.Fatal("staging should remove the item from A")
	}
	if err := tx.WithdrawItem(ctx, "A", "sword"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if !cus.has("A", "sword") {
		t.Error("withdraw should return the sword to A")
	}
}

func TestWithdrawCoin_PartialAndFull(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.setGold("A", 100)
	tx := New("t1", cus, &fakeBus{})

	_ = tx.StageCoin(ctx, "A", 80)
	if got := cus.balance("A"); got != 20 {
		t.Fatalf("after staging 80, A gold = %d, want 20", got)
	}
	if err := tx.WithdrawCoin(ctx, "A", 30); err != nil {
		t.Fatalf("withdraw 30: %v", err)
	}
	if got := cus.balance("A"); got != 50 {
		t.Errorf("A gold = %d, want 50", got)
	}
	// Over-withdraw clamps to the remaining staged 50.
	if err := tx.WithdrawCoin(ctx, "A", 999); err != nil {
		t.Fatalf("withdraw rest: %v", err)
	}
	if got := cus.balance("A"); got != 100 {
		t.Errorf("A gold = %d, want 100 (fully restored)", got)
	}
	if err := tx.WithdrawCoin(ctx, "A", 10); !errors.Is(err, ErrNotStaged) {
		t.Errorf("withdraw with no coin staged = %v, want ErrNotStaged", err)
	}
}

func TestStageItem_DoubleStageSameTxn(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "sword")
	tx := New("t1", cus, &fakeBus{})

	if err := tx.StageItem(ctx, "A", "sword"); err != nil {
		t.Fatalf("first stage: %v", err)
	}
	if err := tx.StageItem(ctx, "A", "sword"); !errors.Is(err, ErrAlreadyStaged) {
		t.Errorf("second stage = %v, want ErrAlreadyStaged", err)
	}
}

func TestStageItem_DoubleStageCrossTxn(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "sword")

	tx1 := New("t1", cus, &fakeBus{})
	if err := tx1.StageItem(ctx, "A", "sword"); err != nil {
		t.Fatalf("tx1 stage: %v", err)
	}
	// The item left A's inventory, so a second transaction cannot stage it.
	tx2 := New("t2", cus, &fakeBus{})
	if err := tx2.StageItem(ctx, "A", "sword"); !errors.Is(err, ErrItemGone) {
		t.Errorf("cross-txn stage = %v, want ErrItemGone", err)
	}
}

func TestStageItem_BoundRefused(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "heirloom")
	cus.bound["heirloom"] = true
	tx := New("t1", cus, &fakeBus{})

	if err := tx.StageItem(ctx, "A", "heirloom"); !errors.Is(err, ErrItemBound) {
		t.Errorf("stage bound = %v, want ErrItemBound", err)
	}
	if !cus.has("A", "heirloom") {
		t.Error("a refused bound item must stay with its owner")
	}
}

func TestStageCoin_Insufficient(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.setGold("A", 10)
	tx := New("t1", cus, &fakeBus{})

	if err := tx.StageCoin(ctx, "A", 50); !errors.Is(err, ErrInsufficientCoin) {
		t.Errorf("stage coin = %v, want ErrInsufficientCoin", err)
	}
	if got := cus.balance("A"); got != 10 {
		t.Errorf("balance after refused stage = %d, want 10", got)
	}
}

func TestCommit_AlreadyDone(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "sword")
	tx := New("t1", cus, &fakeBus{})
	_ = tx.StageItem(ctx, "A", "sword")
	if err := tx.Commit(ctx, map[string]string{"A": "B"}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := tx.Commit(ctx, map[string]string{"A": "B"}); !errors.Is(err, ErrAlreadyDone) {
		t.Errorf("second commit = %v, want ErrAlreadyDone", err)
	}
	if err := tx.StageItem(ctx, "A", "x"); !errors.Is(err, ErrAlreadyDone) {
		t.Errorf("stage after commit = %v, want ErrAlreadyDone", err)
	}
}

func TestCommit_MissingDestination(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "sword")
	tx := New("t1", cus, &fakeBus{})
	_ = tx.StageItem(ctx, "A", "sword")

	err := tx.Commit(ctx, map[string]string{}) // no dest for A
	if !errors.Is(err, ErrNoDestination) {
		t.Fatalf("commit = %v, want ErrNoDestination", err)
	}
	// Nothing moved and the txn is still usable (not finalized).
	if cus.has("A", "sword") {
		t.Error("a destination-less commit must not move value")
	}
	if err := tx.WithdrawItem(ctx, "A", "sword"); err != nil {
		t.Errorf("txn should still be live after a rejected commit: %v", err)
	}
}

func TestRollback_Explicit(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "sword")
	cus.setGold("A", 100)
	tx := New("t1", cus, &fakeBus{})
	_ = tx.StageItem(ctx, "A", "sword")
	_ = tx.StageCoin(ctx, "A", 40)

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if !cus.has("A", "sword") || cus.balance("A") != 100 {
		t.Error("rollback must return all staged value to A")
	}
	// Rollback is idempotent / safe after finalize.
	if err := tx.Rollback(ctx); err != nil {
		t.Errorf("second rollback = %v, want nil", err)
	}
}

// TestCommit_MidDeliveryItemFailureMakesWhole verifies the make-whole
// guarantee when an item delivery fails after an earlier item already moved
// and coin was reserved: everything returns to its owner, no duplication.
func TestCommit_MidDeliveryItemFailureMakesWhole(t *testing.T) {
	ctx := context.Background()
	cus := newFakeCustodian()
	cus.give("A", "sword1")
	cus.give("A", "sword2")
	cus.setGold("B", 100)
	cus.failDeliver = "sword2" // the second item delivery blows up
	bus := &fakeBus{}

	tx := New("t1", cus, bus)
	_ = tx.StageItem(ctx, "A", "sword1")
	_ = tx.StageItem(ctx, "A", "sword2")
	_ = tx.StageCoin(ctx, "B", 50)

	err := tx.Commit(ctx, map[string]string{"A": "B", "B": "A"})
	if !errors.Is(err, errDeliver) {
		t.Fatalf("commit err = %v, want errDeliver", err)
	}

	// Both swords back with A; neither stranded at B.
	if !cus.has("A", "sword1") || !cus.has("A", "sword2") {
		t.Error("both swords must return to A after a mid-commit failure")
	}
	if cus.has("B", "sword1") || cus.has("B", "sword2") {
		t.Error("no sword may be stranded at B")
	}
	// Coin fully restored — never delivered, never double-credited.
	if got := cus.balance("B"); got != 100 {
		t.Errorf("B gold = %d, want 100 (coin made whole)", got)
	}
	if got := cus.balance("A"); got != 0 {
		t.Errorf("A gold = %d, want 0", got)
	}
	if bus.commits() != 0 {
		t.Error("trade.committed must not fire on a failed commit")
	}
}

// TestConcurrent_WithdrawVsCommit fires a withdraw and a commit at once and
// asserts value conservation regardless of which wins (run with -race).
func TestConcurrent_WithdrawVsCommit(t *testing.T) {
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		cus := newFakeCustodian()
		cus.give("A", "sword")
		cus.setGold("B", 100)
		tx := New("t1", cus, &fakeBus{})
		_ = tx.StageItem(ctx, "A", "sword")
		_ = tx.StageCoin(ctx, "B", 50)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = tx.WithdrawItem(ctx, "A", "sword") }()
		go func() { defer wg.Done(); _ = tx.Commit(ctx, map[string]string{"A": "B", "B": "A"}) }()
		wg.Wait()

		// Invariant: exactly one sword exists, total gold is conserved at 100,
		// and the sword is never in two places.
		swords := 0
		if cus.has("A", "sword") {
			swords++
		}
		if cus.has("B", "sword") {
			swords++
		}
		if swords != 1 {
			t.Fatalf("iter %d: sword count = %d, want exactly 1", i, swords)
		}
		if total := cus.balance("A") + cus.balance("B"); total != 100 {
			t.Fatalf("iter %d: total gold = %d, want 100 (conserved)", i, total)
		}
	}
}
