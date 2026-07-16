package economy

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
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

// ratedCredTpl is credTpl plus a credential_rating (the §7 scan bonus).
func ratedCredTpl(id, name string, rating int, permits ...string) *item.Template {
	tpl := credTpl(id, name, permits...)
	tpl.Properties[PropCredentialRating] = rating
	return tpl
}

// giveCredential spawns a credential template into the store and hands it to the
// shopper's inventory, mirroring a carried fake SIN. Returns the live instance so
// a test can assert its burned state.
func giveCredential(t *testing.T, f *shopFixture, sh *fakeShopper, tpl *item.Template) *entities.ItemInstance {
	t.Helper()
	inst, err := f.store.Spawn(tpl)
	if err != nil {
		t.Fatalf("spawn credential: %v", err)
	}
	sh.AddToInventory(inst.ID())
	return inst
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

			res := f.svc.Buy(context.Background(), sh, npc, cfg, tt.stock.Name, nil, nil, nil)
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

// scanPass / scanFail are deterministic LicenseScanners for the §7 tests.
func scanPass(int, int) bool { return true }
func scanFail(int, int) bool { return false }

func TestBuy_Scan(t *testing.T) {
	const npc = "clerk1"
	pistol := legalityTpl("sr:pistol", "a heavy pistol", LegalityRestricted, "firearms")

	t.Run("scan pass sells and leaves the credential intact", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		f.tpls.Add(pistol)
		sh := newShopper("p1", 100_000)
		inst := giveCredential(t, f, sh, ratedCredTpl("sr:sin", "a fake SIN", 4, "firearms"))
		cfg := ShopConfig{Sells: []string{"sr:pistol"}, RequiresLicense: true, ScannerRating: 10}

		res := f.svc.Buy(context.Background(), sh, npc, cfg, "pistol", nil, nil, scanPass)
		if res.Outcome != ShopOK {
			t.Fatalf("outcome = %v, want ShopOK", res.Outcome)
		}
		if credentialBurned(inst) {
			t.Error("credential burned on a passing scan")
		}
	})

	t.Run("scan fail burns the credential and refuses the sale", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		f.tpls.Add(pistol)
		sh := newShopper("p1", 100_000)
		inst := giveCredential(t, f, sh, ratedCredTpl("sr:sin", "a fake SIN", 2, "firearms"))
		cfg := ShopConfig{Sells: []string{"sr:pistol"}, RequiresLicense: true, ScannerRating: 10}

		res := f.svc.Buy(context.Background(), sh, npc, cfg, "pistol", nil, nil, scanFail)
		if res.Outcome != ShopSINBurned {
			t.Fatalf("outcome = %v, want ShopSINBurned", res.Outcome)
		}
		if res.BurnedCredential != "a fake SIN" {
			t.Errorf("BurnedCredential = %q, want %q", res.BurnedCredential, "a fake SIN")
		}
		if !credentialBurned(inst) {
			t.Error("credential not burned after a failed scan")
		}
		if sh.gold != 100_000 {
			t.Errorf("gold = %d, want 100000 (no charge on a burned refusal)", sh.gold)
		}
		// A burned credential no longer clears the gate — a retry reads as SINless.
		if res2 := f.svc.Buy(context.Background(), sh, npc, cfg, "pistol", nil, nil, scanFail); res2.Outcome != ShopSINRequired {
			t.Errorf("retry with a burned SIN = %v, want ShopSINRequired", res2.Outcome)
		}
	})

	t.Run("scanner_rating 0 never rolls (slice-1 behavior)", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		f.tpls.Add(pistol)
		sh := newShopper("p1", 100_000)
		inst := giveCredential(t, f, sh, ratedCredTpl("sr:sin", "a fake SIN", 2, "firearms"))
		cfg := ShopConfig{Sells: []string{"sr:pistol"}, RequiresLicense: true, ScannerRating: 0}

		// Even a failing scanner is never consulted when scanner_rating is 0.
		if res := f.svc.Buy(context.Background(), sh, npc, cfg, "pistol", nil, nil, scanFail); res.Outcome != ShopOK {
			t.Fatalf("outcome = %v, want ShopOK", res.Outcome)
		}
		if credentialBurned(inst) {
			t.Error("credential burned though scanner_rating was 0")
		}
	})

	t.Run("legal good never triggers a scan", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		vest := legalityTpl("sr:vest", "an armor vest", LegalityLegal, "")
		f.tpls.Add(vest)
		sh := newShopper("p1", 100_000)
		inst := giveCredential(t, f, sh, ratedCredTpl("sr:sin", "a fake SIN", 2))
		cfg := ShopConfig{Sells: []string{"sr:vest"}, RequiresLicense: true, ScannerRating: 10}

		if res := f.svc.Buy(context.Background(), sh, npc, cfg, "vest", nil, nil, scanFail); res.Outcome != ShopOK {
			t.Fatalf("legal-good outcome = %v, want ShopOK (no scan)", res.Outcome)
		}
		if credentialBurned(inst) {
			t.Error("credential burned buying a legal good")
		}
	})

	t.Run("highest-rated matching credential is the one scanned", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		f.tpls.Add(pistol)
		sh := newShopper("p1", 100_000)
		low := giveCredential(t, f, sh, ratedCredTpl("sr:sin-lo", "a cheap SIN", 2, "firearms"))
		high := giveCredential(t, f, sh, ratedCredTpl("sr:sin-hi", "a premium SIN", 5, "firearms"))
		cfg := ShopConfig{Sells: []string{"sr:pistol"}, RequiresLicense: true, ScannerRating: 10}

		var scannedRating int
		record := func(credRating, _ int) bool { scannedRating = credRating; return false }
		res := f.svc.Buy(context.Background(), sh, npc, cfg, "pistol", nil, nil, record)

		if scannedRating != 5 {
			t.Errorf("scanned rating = %d, want 5 (the best fake)", scannedRating)
		}
		if res.BurnedCredential != "a premium SIN" || !credentialBurned(high) {
			t.Errorf("burned = %q / high-burned=%v, want the premium SIN burned", res.BurnedCredential, credentialBurned(high))
		}
		if credentialBurned(low) {
			t.Error("the cheap SIN burned though the premium was presented")
		}
	})
}

func TestCheckpointScan(t *testing.T) {
	t.Run("no credential is turned back", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		sh := newShopper("p1", 0)
		out, _ := f.svc.CheckpointScan(sh, "corporate", 14, scanPass)
		if out != CheckpointNoSIN {
			t.Fatalf("out = %v, want CheckpointNoSIN", out)
		}
	})

	t.Run("credential without the permit is turned back (no scan)", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		sh := newShopper("p1", 0)
		inst := giveCredential(t, f, sh, ratedCredTpl("sr:sin", "a fake SIN", 3, "firearms"))
		out, _ := f.svc.CheckpointScan(sh, "corporate", 14, scanFail)
		if out != CheckpointNoPermit {
			t.Fatalf("out = %v, want CheckpointNoPermit", out)
		}
		if credentialBurned(inst) {
			t.Error("credential burned though its permit never matched (no scan should run)")
		}
	})

	t.Run("matching permit + scan pass clears the checkpoint", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		sh := newShopper("p1", 0)
		inst := giveCredential(t, f, sh, ratedCredTpl("sr:sin", "a fake SIN", 4, "corporate"))
		out, _ := f.svc.CheckpointScan(sh, "corporate", 14, scanPass)
		if out != CheckpointOK {
			t.Fatalf("out = %v, want CheckpointOK", out)
		}
		if credentialBurned(inst) {
			t.Error("credential burned on a passing scan")
		}
	})

	t.Run("scan fail burns the fake and refuses the crossing", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		sh := newShopper("p1", 0)
		inst := giveCredential(t, f, sh, ratedCredTpl("sr:sin", "a premium SIN", 4, "corporate"))
		out, name := f.svc.CheckpointScan(sh, "corporate", 14, scanFail)
		if out != CheckpointBurned || name != "a premium SIN" {
			t.Fatalf("out=%v name=%q, want CheckpointBurned / a premium SIN", out, name)
		}
		if !credentialBurned(inst) {
			t.Error("credential not burned after a failed checkpoint scan")
		}
		// A burned fake no longer clears the checkpoint — a retry reads as SINless.
		if out2, _ := f.svc.CheckpointScan(sh, "corporate", 14, scanFail); out2 != CheckpointNoSIN {
			t.Errorf("retry with a burned SIN = %v, want CheckpointNoSIN", out2)
		}
	})

	t.Run("identity-only checkpoint (no permit) still scans", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		sh := newShopper("p1", 0)
		inst := giveCredential(t, f, sh, ratedCredTpl("sr:sin", "a fake SIN", 2))
		out, _ := f.svc.CheckpointScan(sh, "", 14, scanFail)
		if out != CheckpointBurned {
			t.Fatalf("out = %v, want CheckpointBurned (identity checkpoints scan)", out)
		}
		if !credentialBurned(inst) {
			t.Error("identity-only checkpoint did not burn the fake on a failed scan")
		}
	})

	t.Run("scannerRating 0 never rolls", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		sh := newShopper("p1", 0)
		inst := giveCredential(t, f, sh, ratedCredTpl("sr:sin", "a fake SIN", 2, "corporate"))
		out, _ := f.svc.CheckpointScan(sh, "corporate", 0, scanFail)
		if out != CheckpointOK {
			t.Fatalf("out = %v, want CheckpointOK (no scan at rating 0)", out)
		}
		if credentialBurned(inst) {
			t.Error("credential burned though scannerRating was 0")
		}
	})
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
