package economy

import "testing"

// TestCurrencyLabel_Formats pins the display behavior for the two shipped
// vocabularies: the gold default (noun suffix) and the Shadowrun reskin (¥
// symbol). The zero value must behave exactly like the gold default so an unset
// label (tests / a world that declares no currency) still prints sensibly.
func TestCurrencyLabel_Formats(t *testing.T) {
	nuyen := CurrencyLabel{Noun: "nuyen", Suffix: "¥"}

	cases := []struct {
		name       string
		label      CurrencyLabel
		n          int
		wantFormat string
		wantName   string
		wantTitle  string
		wantSymbol string
	}{
		{"gold default", DefaultCurrency, 725, "725 gold", "gold", "Gold", ""},
		{"zero value == gold", CurrencyLabel{}, 50, "50 gold", "gold", "Gold", ""},
		{"nuyen ¥", nuyen, 725, "725¥", "nuyen", "Nuyen", "¥"},
		{"nuyen zero", nuyen, 0, "0¥", "nuyen", "Nuyen", "¥"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.label.Format(tc.n); got != tc.wantFormat {
				t.Errorf("Format(%d) = %q, want %q", tc.n, got, tc.wantFormat)
			}
			if got := tc.label.Name(); got != tc.wantName {
				t.Errorf("Name() = %q, want %q", got, tc.wantName)
			}
			if got := tc.label.Title(); got != tc.wantTitle {
				t.Errorf("Title() = %q, want %q", got, tc.wantTitle)
			}
			if got := tc.label.Symbol(); got != tc.wantSymbol {
				t.Errorf("Symbol() = %q, want %q", got, tc.wantSymbol)
			}
		})
	}
}

// TestCurrencyLabel_AmountPreservesFormatting proves Amount appends the unit to
// an already-formatted number string (so a caller's thousands separators
// survive) — the score purse path.
func TestCurrencyLabel_AmountPreservesFormatting(t *testing.T) {
	if got := DefaultCurrency.Amount("1,250"); got != "1,250 gold" {
		t.Errorf("gold Amount = %q, want %q", got, "1,250 gold")
	}
	nuyen := CurrencyLabel{Noun: "nuyen", Suffix: "¥"}
	if got := nuyen.Amount("1,250"); got != "1,250¥" {
		t.Errorf("nuyen Amount = %q, want %q", got, "1,250¥")
	}
}
