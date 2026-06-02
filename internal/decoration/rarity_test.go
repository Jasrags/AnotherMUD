package decoration

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

func rareTier() Tier {
	return Tier{
		Key:     "rare",
		Order:   30,
		Display: "RARE",
		Left:    "[",
		Right:   "]",
		Color:   render.ThemeEntry{FG: "blue"},
		Visible: true,
	}
}

// A registered tier resolves case-insensitively (key normalized on both
// register and get).
func TestRarity_RegisterAndGetCaseInsensitive(t *testing.T) {
	r := NewRarityRegistry()
	if !r.Register(Tier{Key: "Rare", Order: 30, Display: "RARE", Left: "[", Right: "]", Visible: true}) {
		t.Fatal("Register returned false for a valid tier")
	}
	for _, k := range []string{"rare", "RARE", " Rare "} {
		if _, ok := r.Get(k); !ok {
			t.Errorf("Get(%q) miss; want hit (keys are case-insensitive)", k)
		}
	}
}

// An empty key is rejected and stores nothing.
func TestRarity_EmptyKeyRejected(t *testing.T) {
	r := NewRarityRegistry()
	if r.Register(Tier{Key: "   ", Display: "X", Left: "[", Right: "]", Visible: true}) {
		t.Error("Register accepted an empty key")
	}
	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0", r.Len())
	}
}

// Re-registering a key replaces the prior definition (later wins).
func TestRarity_RegisterLaterWins(t *testing.T) {
	r := NewRarityRegistry()
	r.Register(Tier{Key: "rare", Order: 30, Display: "RARE", Left: "[", Right: "]", Visible: true})
	r.Register(Tier{Key: "rare", Order: 35, Display: "Rare!", Left: "<", Right: ">", Visible: true})

	got, ok := r.Get("rare")
	if !ok {
		t.Fatal("Get(rare) miss after re-register")
	}
	if got.Order != 35 || got.Display != "Rare!" || got.VisibleText() != "<Rare!>" {
		t.Errorf("after re-register: %+v, want Order=35 Display=Rare! visible=<Rare!>", got)
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1 (re-register replaces, not appends)", r.Len())
	}
}

// All returns tiers sorted by Order ascending, ties broken by key.
func TestRarity_AllSortedByOrder(t *testing.T) {
	r := NewRarityRegistry()
	r.Register(Tier{Key: "legendary", Order: 50, Display: "LEG", Left: "[", Right: "]", Visible: true})
	r.Register(Tier{Key: "common", Order: 10})
	r.Register(Tier{Key: "rare", Order: 30, Display: "RARE", Left: "[", Right: "]", Visible: true})

	all := r.All()
	want := []string{"common", "rare", "legendary"}
	if len(all) != len(want) {
		t.Fatalf("All len = %d, want %d", len(all), len(want))
	}
	for i, w := range want {
		if all[i].Key != w {
			t.Errorf("All[%d].Key = %q, want %q", i, all[i].Key, w)
		}
	}
}

// VisibleText renders the decorated text for a complete visible tier, and
// empty for each "renders as nothing" condition (§2).
func TestRarity_VisibleText(t *testing.T) {
	tests := []struct {
		name string
		tier Tier
		want string
	}{
		{"complete visible tier", rareTier(), "[RARE]"},
		{"invisible", func() Tier { x := rareTier(); x.Visible = false; return x }(), ""},
		{"no display text", func() Tier { x := rareTier(); x.Display = ""; return x }(), ""},
		{"no decorator pair", func() Tier { x := rareTier(); x.Left, x.Right = "", ""; return x }(), ""},
		{"half a decorator pair", func() Tier { x := rareTier(); x.Right = ""; return x }(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tier.VisibleText(); got != tt.want {
				t.Errorf("VisibleText() = %q, want %q", got, tt.want)
			}
		})
	}
}

// MaxVisibleWidth is the widest visible tag; invisible/blank tiers do not
// contribute, and an empty registry is zero.
func TestRarity_MaxVisibleWidth(t *testing.T) {
	r := NewRarityRegistry()
	if got := r.MaxVisibleWidth(); got != 0 {
		t.Errorf("empty MaxVisibleWidth = %d, want 0", got)
	}
	r.Register(Tier{Key: "common", Order: 10})                                                        // blank → width 0
	r.Register(Tier{Key: "rare", Order: 30, Display: "RARE", Left: "[", Right: "]", Visible: true})   // "[RARE]" = 6
	r.Register(Tier{Key: "epic", Order: 40, Display: "EPIC!!", Left: "<", Right: ">", Visible: true}) // "<EPIC!!>" = 8

	if got := r.MaxVisibleWidth(); got != 8 {
		t.Errorf("MaxVisibleWidth = %d, want 8", got)
	}
}

// An unknown key resolves to (zero, false) — callers treat it as unset.
func TestRarity_GetUnknown(t *testing.T) {
	r := NewRarityRegistry()
	r.Register(rareTier())
	if _, ok := r.Get("mythic"); ok {
		t.Error("Get(mythic) hit; want miss for an unregistered key")
	}
	if _, ok := r.Get(""); ok {
		t.Error("Get(\"\") hit; want miss")
	}
}

// ValidateKey accepts clean keys and rejects empty, markup-bearing, and
// whitespace keys (the load-boundary guard).
func TestValidateKey(t *testing.T) {
	good := []string{"rare", "Rare", " rare ", "epic-plus", "tier2"}
	for _, k := range good {
		if err := ValidateKey(k); err != nil {
			t.Errorf("ValidateKey(%q) = %v, want nil", k, err)
		}
	}
	bad := []string{"", "   ", "ra>re", "ra<re", "a{b}", "ice cold", "tab\tkey"}
	for _, k := range bad {
		if err := ValidateKey(k); err == nil {
			t.Errorf("ValidateKey(%q) = nil, want error", k)
		}
	}
}

// Register rejects an invalid key (markup char) on both registries.
func TestRegister_RejectsInvalidKey(t *testing.T) {
	r := NewRarityRegistry()
	if r.Register(Tier{Key: "ra>re", Display: "X", Left: "[", Right: "]", Visible: true}) {
		t.Error("rarity Register accepted a markup-bearing key")
	}
	if r.Len() != 0 {
		t.Errorf("rarity Len = %d, want 0", r.Len())
	}
	e := NewEssenceRegistry()
	if e.Register(Essence{Key: "ice cold", Glyph: "❄"}) {
		t.Error("essence Register accepted a whitespace key")
	}
	if e.Len() != 0 {
		t.Errorf("essence Len = %d, want 0", e.Len())
	}
}
