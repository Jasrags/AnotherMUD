package economy

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// credTpl builds a credential item template (a fake SIN) clearing the given
// permits (sin-and-legality.md §3).
func credTpl(id, name string, permits ...string) *item.Template {
	perms := make([]any, len(permits))
	for i, p := range permits {
		perms[i] = p
	}
	return &item.Template{
		ID:         item.TemplateID(id),
		Name:       name,
		Type:       "item",
		Tags:       []string{TagCredential},
		Properties: map[string]any{"value": 100, PropPermits: perms},
	}
}

// legalityTpl builds a stock item template carrying a legality band (+ permit).
func legalityTpl(id, name, band, permit string) *item.Template {
	props := map[string]any{"value": 20, PropLegality: band}
	if permit != "" {
		props[PropPermit] = permit
	}
	return &item.Template{ID: item.TemplateID(id), Name: name, Type: "item", Properties: props}
}

// giveCredential spawns a credential template into the store and hands it to the
// shopper's inventory, mirroring a carried fake SIN.
func giveCredential(t *testing.T, f *shopFixture, sh *fakeShopper, tpl *item.Template) {
	t.Helper()
	inst, err := f.store.Spawn(tpl)
	if err != nil {
		t.Fatalf("spawn credential: %v", err)
	}
	sh.AddToInventory(inst.ID())
}

func TestBuy_LicenseGate(t *testing.T) {
	const npc = "clerk1"
	tests := []struct {
		name        string
		requires    bool           // shop.RequiresLicense
		stock       *item.Template // the item being bought
		credentials []*item.Template
		want        ShopOutcome
		wantPermit  string
	}{
		{
			name:     "SINless buyer refused at legit store",
			requires: true,
			stock:    legalityTpl("sr:vest", "an armor vest", LegalityLegal, ""),
			want:     ShopSINRequired,
		},
		{
			name:        "legal good clears with any credential",
			requires:    true,
			stock:       legalityTpl("sr:vest", "an armor vest", LegalityLegal, ""),
			credentials: []*item.Template{credTpl("sr:sin", "a fake SIN")},
			want:        ShopOK,
		},
		{
			name:        "restricted good needs matching permit",
			requires:    true,
			stock:       legalityTpl("sr:pistol", "a heavy pistol", LegalityRestricted, "firearms"),
			credentials: []*item.Template{credTpl("sr:sin", "a fake SIN", "cyberware")},
			want:        ShopLicenseRequired,
			wantPermit:  "firearms",
		},
		{
			name:        "restricted good cleared by matching permit",
			requires:    true,
			stock:       legalityTpl("sr:pistol", "a heavy pistol", LegalityRestricted, "firearms"),
			credentials: []*item.Template{credTpl("sr:sin", "a fake SIN", "firearms", "cyberware")},
			want:        ShopOK,
		},
		{
			name:        "restricted good with no named permit cleared by any credential",
			requires:    true,
			stock:       legalityTpl("sr:thing", "a restricted thing", LegalityRestricted, ""),
			credentials: []*item.Template{credTpl("sr:sin", "a fake SIN")},
			want:        ShopOK,
		},
		{
			name:        "forbidden good refused at legit store even with full papers",
			requires:    true,
			stock:       legalityTpl("sr:ak", "an AK-97", LegalityForbidden, ""),
			credentials: []*item.Template{credTpl("sr:sin", "a fake SIN", "firearms")},
			want:        ShopForbiddenGoods,
		},
		{
			name:     "shadow vendor sells forbidden goods to a SINless buyer",
			requires: false,
			stock:    legalityTpl("sr:ak", "an AK-97", LegalityForbidden, ""),
			want:     ShopOK,
		},
		{
			name:     "shadow vendor sells restricted goods with no permit",
			requires: false,
			stock:    legalityTpl("sr:pistol", "a heavy pistol", LegalityRestricted, "firearms"),
			want:     ShopOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newShopFixture(t, DefaultEconomyConfig())
			f.tpls.Add(tt.stock)
			sh := newShopper("p1", 100_000)
			for _, c := range tt.credentials {
				giveCredential(t, f, sh, c)
			}
			cfg := ShopConfig{Sells: []string{string(tt.stock.ID)}, RequiresLicense: tt.requires}

			res := f.svc.Buy(context.Background(), sh, npc, cfg, tt.stock.Name, nil, nil)
			if res.Outcome != tt.want {
				t.Fatalf("outcome = %v, want %v", res.Outcome, tt.want)
			}
			if tt.wantPermit != "" && res.RequiredPermit != tt.wantPermit {
				t.Errorf("RequiredPermit = %q, want %q", res.RequiredPermit, tt.wantPermit)
			}
			// A refusal must not charge the buyer.
			if tt.want != ShopOK && sh.gold != 100_000 {
				t.Errorf("gold = %d, want 100000 (no charge on refusal)", sh.gold)
			}
		})
	}
}

func TestCarriedCredentials(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	sh := newShopper("p1", 100)

	// No credential → empty (the licenses verb reports SINless).
	if got := f.svc.CarriedCredentials(sh); len(got) != 0 {
		t.Fatalf("CarriedCredentials with none = %v, want empty", got)
	}

	giveCredential(t, f, sh, credTpl("sr:sin", "a fake SIN", "cyberware", "firearms"))
	got := f.svc.CarriedCredentials(sh)
	if len(got) != 1 {
		t.Fatalf("CarriedCredentials = %d entries, want 1", len(got))
	}
	if got[0].Name != "a fake SIN" {
		t.Errorf("name = %q, want %q", got[0].Name, "a fake SIN")
	}
	// Permits are sorted for a stable display.
	if len(got[0].Permits) != 2 || got[0].Permits[0] != "cyberware" || got[0].Permits[1] != "firearms" {
		t.Errorf("permits = %v, want [cyberware firearms]", got[0].Permits)
	}
}
