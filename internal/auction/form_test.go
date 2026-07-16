package auction

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// TestForm_ProjectsListingsAndOwnership lists one item and checks the read-only
// Form projection: the listing carries the numeric ref / name / raw price /
// seller, the viewer's ownership flag flips per viewer, and an active listing
// yields no collectibles.
func TestForm_ProjectsListingsAndOwnership(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	inst, _ := items.GetByID(itemID)
	if err := m.List(context.Background(), seller, inst.(*entities.ItemInstance), 200); err != nil {
		t.Fatalf("list: %v", err)
	}
	now := clock.RealClock{}.Now()

	// A buyer viewing: the listing is not theirs (Mine=false).
	fb := m.Form(now, "bob")
	if fb.Total != 1 || len(fb.Listings) != 1 {
		t.Fatalf("buyer form: %d listings (total %d), want 1/1", len(fb.Listings), fb.Total)
	}
	o := fb.Listings[0]
	if o.Name != "an iron dagger" || o.Price != 200 || o.Seller != "Alice" || o.Mine {
		t.Errorf("listing = %+v, want an iron dagger / 200 / Alice / not-mine", o)
	}
	if o.Ref == "" || strings.HasPrefix(o.Ref, "au-") {
		t.Errorf("ref = %q, want the numeric suffix with au- stripped", o.Ref)
	}
	if o.SecondsLeft <= 0 {
		t.Errorf("secondsLeft = %d, want > 0 for an hour-long listing", o.SecondsLeft)
	}
	if fb.Collectible.Items != 0 || fb.Collectible.Coin != 0 {
		t.Errorf("collectible = %+v, want empty for a buyer with no pending pickups", fb.Collectible)
	}

	// The seller viewing the same listing: Mine=true.
	fa := m.Form(now, "alice")
	if len(fa.Listings) != 1 || !fa.Listings[0].Mine {
		t.Errorf("seller form: listing not marked Mine: %+v", fa.Listings)
	}
}
