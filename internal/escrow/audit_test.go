package escrow

import (
	"context"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
)

func TestAudit_AppendOnCommitAndLoad(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	clk := clock.NewManual(time.Unix(1_700_000_000, 0).UTC())
	store := NewAuditStore(dir, clk)

	cus := newFakeCustodian()
	cus.give("A", "sword")
	cus.setGold("B", 100)

	tx := New("txn-1", cus, &fakeBus{}, WithAudit(store, "direct-trade"))
	_ = tx.StageItem(ctx, "A", "sword")
	_ = tx.StageCoin(ctx, "B", 50)
	if err := tx.Commit(ctx, map[string]string{"A": "B", "B": "A"}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	recs, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.TxnID != "txn-1" || r.Source != "direct-trade" {
		t.Errorf("record id/source = %q/%q", r.TxnID, r.Source)
	}
	if r.Version != CurrentAuditVersion {
		t.Errorf("record version = %d, want %d", r.Version, CurrentAuditVersion)
	}
	if !r.Time.Equal(clk.Now()) {
		t.Errorf("record time = %v, want %v", r.Time, clk.Now())
	}
	if len(r.Legs) != 2 {
		t.Fatalf("legs = %d, want 2", len(r.Legs))
	}
	// One item leg A→B (sword) and one coin leg B→A (50).
	var sawItem, sawCoin bool
	for _, l := range r.Legs {
		switch l.Kind {
		case "item":
			sawItem = l.Party == "A" && l.Dest == "B" && l.Item == "sword"
		case "coin":
			sawCoin = l.Party == "B" && l.Dest == "A" && l.Amount == 50
		}
	}
	if !sawItem || !sawCoin {
		t.Errorf("legs not recorded faithfully: %+v", r.Legs)
	}
}

func TestAudit_AppendOnlyAcrossCommits(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	clk := clock.NewManual(time.Unix(1_700_000_000, 0).UTC())
	store := NewAuditStore(dir, clk)

	commit := func(id string) {
		cus := newFakeCustodian()
		cus.give("A", "item-"+id)
		tx := New(id, cus, &fakeBus{}, WithAudit(store, "direct-trade"))
		_ = tx.StageItem(ctx, "A", "item-"+id)
		if err := tx.Commit(ctx, map[string]string{"A": "B"}); err != nil {
			t.Fatalf("commit %s: %v", id, err)
		}
	}

	commit("txn-1")
	first, _ := store.Load()
	commit("txn-2")
	second, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("records = %d, want 2", len(second))
	}
	// The first record is unchanged by the second append (append-only).
	if second[0].TxnID != first[0].TxnID || second[0].TxnID != "txn-1" {
		t.Errorf("first record mutated: %q vs %q", second[0].TxnID, first[0].TxnID)
	}
	if second[1].TxnID != "txn-2" {
		t.Errorf("second record = %q, want txn-2", second[1].TxnID)
	}
}

func TestAudit_LoadMissingIsEmpty(t *testing.T) {
	store := NewAuditStore(t.TempDir(), clock.NewManual(time.Unix(0, 0)))
	recs, err := store.Load()
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("records = %d, want 0", len(recs))
	}
}

func TestAudit_NoCommitOnVetoNotRecorded(t *testing.T) {
	ctx := context.Background()
	store := NewAuditStore(t.TempDir(), clock.NewManual(time.Unix(0, 0)))
	cus := newFakeCustodian()
	cus.give("A", "sword")

	tx := New("txn-1", cus, &fakeBus{veto: true}, WithAudit(store, "direct-trade"))
	_ = tx.StageItem(ctx, "A", "sword")
	_ = tx.Commit(ctx, map[string]string{"A": "B"})

	recs, _ := store.Load()
	if len(recs) != 0 {
		t.Errorf("vetoed commit must not append an audit record; got %d", len(recs))
	}
}
