package trade

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// TestView_ClosedWhenNotTrading returns an empty, closed view for a player with
// no open session.
func TestView_ClosedWhenNotTrading(t *testing.T) {
	m := newTestManager(&fakeBus{}, nil)
	if v := m.View("nobody"); v.Open {
		t.Errorf("View for a non-trading player = %+v, want Open=false", v)
	}
}

// TestView_ProjectsBothSides opens a trade, stages an item + coin on one side and
// an item on the other, confirms one side, and checks the viewer-relative
// projection (names via the describer, coin via the label, confirmed flags).
func TestView_ProjectsBothSides(t *testing.T) {
	ctx := context.Background()
	cur := economy.NewCurrencyService(nil)
	names := map[entities.EntityID]string{"i-dagger": "a steel dagger", "i-medkit": "a medkit"}
	describe := func(id entities.EntityID) string { return names[id] }
	m := NewManager(&fakeBus{}, nil, cur, nil, describe, economy.CurrencyLabel{Noun: "nuyen", Suffix: "¥"})

	a := newParty("A", "Alice")
	b := newParty("B", "Bob")
	a.AddToInventory("i-dagger")
	a.gold = 100
	b.AddToInventory("i-medkit")
	open(t, m, a, b)

	if err := m.OfferItem(ctx, a, "i-dagger"); err != nil {
		t.Fatalf("A offer dagger: %v", err)
	}
	if err := m.OfferCoin(ctx, a, 50); err != nil {
		t.Fatalf("A offer coin: %v", err)
	}
	if err := m.OfferItem(ctx, b, "i-medkit"); err != nil {
		t.Fatalf("B offer medkit: %v", err)
	}
	// A confirms; any of B's changes would clear it, but B is done offering.
	if err := m.Confirm(ctx, a); err != nil {
		t.Fatalf("A confirm: %v", err)
	}

	// From Alice's perspective: Mine = dagger + 50¥ + confirmed; Theirs = medkit.
	v := m.View("A")
	if !v.Open {
		t.Fatal("View(A) should be open")
	}
	if len(v.Mine.Items) != 1 || v.Mine.Items[0].Name != "a steel dagger" {
		t.Errorf("Mine.Items = %+v, want [a steel dagger]", v.Mine.Items)
	}
	if v.Mine.Coin != "50¥" {
		t.Errorf("Mine.Coin = %q, want 50¥ (currency-labelled)", v.Mine.Coin)
	}
	if !v.Mine.Confirmed {
		t.Error("Mine.Confirmed = false, want true (A confirmed)")
	}
	if v.Theirs.Party != "Bob" || len(v.Theirs.Items) != 1 || v.Theirs.Items[0].Name != "a medkit" {
		t.Errorf("Theirs = %+v, want Bob with [a medkit]", v.Theirs)
	}
	if v.Theirs.Coin != "" {
		t.Errorf("Theirs.Coin = %q, want empty (no coin offered)", v.Theirs.Coin)
	}

	// From Bob's perspective the sides swap: Mine = medkit, Theirs = dagger + 50¥.
	vb := m.View("B")
	if vb.Mine.Items[0].Name != "a medkit" || vb.Theirs.Items[0].Name != "a steel dagger" || vb.Theirs.Coin != "50¥" {
		t.Errorf("View(B) sides not viewer-relative: %+v", vb)
	}
	if vb.Mine.Confirmed {
		t.Error("View(B).Mine.Confirmed = true, want false (B hasn't confirmed)")
	}
}
