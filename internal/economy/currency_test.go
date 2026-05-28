package economy

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeEntity is a minimal Entity for service tests. lockedEntity below
// adds its own mutex for the concurrency test; the plain fakeEntity is
// used by the single-threaded cases.
type fakeEntity struct {
	id   string
	gold int
}

func (f *fakeEntity) ID() string    { return f.id }
func (f *fakeEntity) Gold() int     { return f.gold }
func (f *fakeEntity) SetGold(v int) { f.gold = v }

// lockedEntity guards its balance so the race detector exercises the
// service's RMW serialization rather than flagging the entity's own
// field access (the real connActor guards gold under a.mu likewise).
type lockedEntity struct {
	mu   sync.Mutex
	id   string
	gold int
}

func (e *lockedEntity) ID() string { return e.id }
func (e *lockedEntity) Gold() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gold
}
func (e *lockedEntity) SetGold(v int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.gold = v
}

// captureSink records the last event of each kind for assertions.
type captureSink struct {
	creditCalls int
	debitCalls  int
	lastID      string
	lastAmount  int
	lastReason  string
	lastTotal   int
}

func (s *captureSink) OnGoldCredited(_ context.Context, id string, amount int, reason string, total int) {
	s.creditCalls++
	s.lastID, s.lastAmount, s.lastReason, s.lastTotal = id, amount, reason, total
}

func (s *captureSink) OnGoldDebited(_ context.Context, id string, amount int, reason string, total int) {
	s.debitCalls++
	s.lastID, s.lastAmount, s.lastReason, s.lastTotal = id, amount, reason, total
}

func TestAddGold(t *testing.T) {
	tests := []struct {
		name       string
		start      int
		delta      int
		wantTotal  int
		wantCredit int
		wantDebit  int
		wantAmount int
	}{
		{name: "credit from zero", start: 0, delta: 50, wantTotal: 50, wantCredit: 1, wantAmount: 50},
		{name: "credit accumulates", start: 50, delta: 25, wantTotal: 75, wantCredit: 1, wantAmount: 25},
		{name: "debit within balance", start: 75, delta: -25, wantTotal: 50, wantDebit: 1, wantAmount: 25},
		{name: "debit floors at zero", start: 30, delta: -100, wantTotal: 0, wantDebit: 1, wantAmount: 100},
		{name: "zero delta is credit", start: 10, delta: 0, wantTotal: 10, wantCredit: 1, wantAmount: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &captureSink{}
			svc := NewCurrencyService(sink)
			e := &fakeEntity{id: "p1", gold: tt.start}

			got := svc.AddGold(context.Background(), e, tt.delta, "test")

			if got != tt.wantTotal {
				t.Errorf("AddGold returned %d, want %d", got, tt.wantTotal)
			}
			if e.gold != tt.wantTotal {
				t.Errorf("entity gold = %d, want %d", e.gold, tt.wantTotal)
			}
			if sink.creditCalls != tt.wantCredit {
				t.Errorf("credit calls = %d, want %d", sink.creditCalls, tt.wantCredit)
			}
			if sink.debitCalls != tt.wantDebit {
				t.Errorf("debit calls = %d, want %d", sink.debitCalls, tt.wantDebit)
			}
			if sink.lastAmount != tt.wantAmount {
				t.Errorf("event amount = %d, want %d (absolute magnitude)", sink.lastAmount, tt.wantAmount)
			}
			if sink.lastTotal != tt.wantTotal {
				t.Errorf("event newTotal = %d, want %d", sink.lastTotal, tt.wantTotal)
			}
		})
	}
}

func TestAddGoldNilEntity(t *testing.T) {
	sink := &captureSink{}
	svc := NewCurrencyService(sink)
	if got := svc.AddGold(context.Background(), nil, 50, "test"); got != 0 {
		t.Errorf("AddGold(nil) = %d, want 0", got)
	}
	if sink.creditCalls != 0 || sink.debitCalls != 0 {
		t.Errorf("nil entity should emit no events, got %d credit / %d debit", sink.creditCalls, sink.debitCalls)
	}
}

func TestSetGold(t *testing.T) {
	sink := &captureSink{}
	svc := NewCurrencyService(sink)
	e := &fakeEntity{id: "p1", gold: 100}

	// Set lower than current still emits credited (spec §2.2).
	if err := svc.SetGold(context.Background(), e, 40, "admin"); err != nil {
		t.Fatalf("SetGold: %v", err)
	}
	if e.gold != 40 {
		t.Errorf("gold = %d, want 40", e.gold)
	}
	if sink.creditCalls != 1 || sink.debitCalls != 0 {
		t.Errorf("Set emits credited regardless of direction: got %d credit / %d debit", sink.creditCalls, sink.debitCalls)
	}
	if sink.lastAmount != 40 || sink.lastTotal != 40 {
		t.Errorf("event amount/total = %d/%d, want 40/40", sink.lastAmount, sink.lastTotal)
	}
}

func TestSetGoldRejectsNegative(t *testing.T) {
	sink := &captureSink{}
	svc := NewCurrencyService(sink)
	e := &fakeEntity{id: "p1", gold: 100}

	err := svc.SetGold(context.Background(), e, -5, "bad")
	if !errors.Is(err, ErrNegativeAmount) {
		t.Errorf("SetGold(-5) error = %v, want ErrNegativeAmount", err)
	}
	if e.gold != 100 {
		t.Errorf("rejected SetGold must not mutate: gold = %d, want 100", e.gold)
	}
	if sink.creditCalls != 0 {
		t.Errorf("rejected SetGold must emit no event, got %d", sink.creditCalls)
	}
}

func TestRead(t *testing.T) {
	svc := NewCurrencyService(nil)
	if got := svc.Read(nil); got != 0 {
		t.Errorf("Read(nil) = %d, want 0", got)
	}
	if got := svc.Read(&fakeEntity{gold: 42}); got != 42 {
		t.Errorf("Read = %d, want 42", got)
	}
}

// TestAddGoldConcurrent pins the RMW-atomicity fix: N goroutines each
// credit 1 gold; no update may be lost. Without the service mutex the
// read-compute-write triple interleaves and the final total comes in
// short. Run with -race.
func TestAddGoldConcurrent(t *testing.T) {
	svc := NewCurrencyService(nil)
	e := &lockedEntity{id: "p1"}

	const workers = 50
	const each = 20
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < each; j++ {
				svc.AddGold(context.Background(), e, 1, "concurrent")
			}
		}()
	}
	wg.Wait()

	if got := e.Gold(); got != workers*each {
		t.Errorf("gold = %d, want %d (no credit may be lost under concurrency)", got, workers*each)
	}
}

func TestDebit(t *testing.T) {
	sink := &captureSink{}
	svc := NewCurrencyService(sink)
	e := &fakeEntity{id: "p1", gold: 50}

	// Affordable charge applies and emits a debit event.
	if got, ok := svc.Debit(context.Background(), e, 30, "buy"); !ok || got != 20 {
		t.Fatalf("Debit(30) = (%d,%v), want (20,true)", got, ok)
	}
	if sink.debitCalls != 1 || sink.lastAmount != 30 || sink.lastTotal != 20 {
		t.Errorf("debit event = calls %d amount %d total %d, want 1/30/20", sink.debitCalls, sink.lastAmount, sink.lastTotal)
	}

	// Unaffordable charge is refused: balance unchanged, no event.
	if got, ok := svc.Debit(context.Background(), e, 999, "buy"); ok || got != 20 {
		t.Errorf("Debit(999) = (%d,%v), want (20,false)", got, ok)
	}
	if sink.debitCalls != 1 {
		t.Errorf("refused debit must emit no event, got %d", sink.debitCalls)
	}
}

// TestDebitConcurrent pins the atomicity that closes the shop
// gate→charge double-spend: with exactly enough gold for ONE charge,
// N concurrent debits must let exactly one succeed. Run with -race.
func TestDebitConcurrent(t *testing.T) {
	svc := NewCurrencyService(nil)
	e := &lockedEntity{id: "p1", gold: 100}

	const workers = 40
	results := make(chan bool, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, ok := svc.Debit(context.Background(), e, 100, "race")
			results <- ok
		}()
	}
	wg.Wait()
	close(results)

	wins := 0
	for ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Errorf("successful debits = %d, want exactly 1 (no over-spend)", wins)
	}
	if e.Gold() != 0 {
		t.Errorf("final gold = %d, want 0", e.Gold())
	}
}

func TestNilSinkIsSafe(t *testing.T) {
	svc := NewCurrencyService(nil)
	e := &fakeEntity{id: "p1"}
	// Must not panic with the default nop sink.
	svc.AddGold(context.Background(), e, 10, "test")
	_ = svc.SetGold(context.Background(), e, 5, "test")
	if e.gold != 5 {
		t.Errorf("gold = %d, want 5", e.gold)
	}
}
