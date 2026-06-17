package item

import (
	"slices"
	"testing"
)

func TestWeaponTierVocabulary(t *testing.T) {
	for _, tier := range []string{"simple", "martial", "exotic"} {
		if !ValidTier(tier) {
			t.Errorf("ValidTier(%q) = false, want true", tier)
		}
	}
	if ValidTier("legendary") {
		t.Error("ValidTier(legendary) = true, want false (not a tier)")
	}
	// The empty string is "untiered" — not a tier name; callers treat
	// absence as LowestTier separately.
	if ValidTier("") {
		t.Error(`ValidTier("") = true, want false`)
	}
	if got, want := LowestTier(), "simple"; got != want {
		t.Errorf("LowestTier() = %q, want %q", got, want)
	}
	// Ordered low→high so a future graduated penalty can read distance.
	if got := WeaponTierNames(); !slices.Equal(got, []string{"simple", "martial", "exotic"}) {
		t.Errorf("WeaponTierNames() = %v, want ordered simple/martial/exotic", got)
	}
}

func TestRangedClassVocabulary(t *testing.T) {
	for _, class := range []string{"thrown", "projectile"} {
		if !ValidRangedClass(class) {
			t.Errorf("ValidRangedClass(%q) = false, want true", class)
		}
	}
	if ValidRangedClass("hurled") {
		t.Error("ValidRangedClass(hurled) = true, want false (not a class)")
	}
	// The empty string is "melee" — not a ranged class; callers treat
	// absence as melee separately.
	if ValidRangedClass("") {
		t.Error(`ValidRangedClass("") = true, want false (melee, not a class)`)
	}
	if got := RangedClassNames(); !slices.Equal(got, []string{"thrown", "projectile"}) {
		t.Errorf("RangedClassNames() = %v, want thrown/projectile", got)
	}
}

func TestRangedDamageBonus(t *testing.T) {
	rating2 := 2
	rating0 := 0
	cases := []struct {
		name        string
		class       string
		strRating   *int
		base        int
		want        int
	}{
		{"melee keeps full bonus", "", nil, 3, 3},
		{"melee keeps negative", "", nil, -1, -1},
		{"thrown adds full bonus", RangedThrown, nil, 3, 3},
		{"thrown keeps negative", RangedThrown, nil, -2, -2},
		{"plain projectile drops positive bonus", RangedProjectile, nil, 3, 0},
		{"plain projectile keeps negative (too weak to draw)", RangedProjectile, nil, -2, -2},
		{"plain projectile zero stays zero", RangedProjectile, nil, 0, 0},
		{"rated projectile caps positive at rating", RangedProjectile, &rating2, 5, 2},
		{"rated projectile under cap keeps bonus", RangedProjectile, &rating2, 1, 1},
		{"rated projectile keeps negative", RangedProjectile, &rating2, -1, -1},
		{"rated-zero projectile caps positive to zero", RangedProjectile, &rating0, 4, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RangedDamageBonus(tc.class, tc.strRating, tc.base); got != tc.want {
				t.Errorf("RangedDamageBonus(%q, %v, %d) = %d, want %d",
					tc.class, tc.strRating, tc.base, got, tc.want)
			}
		})
	}
}

func TestProficient(t *testing.T) {
	tiers := []string{"martial"}    // class grants all martial
	cats := []string{"two-rivers-longbow"} // ...plus this one exotic kind

	cases := []struct {
		name       string
		wTier, wCat string
		want       bool
	}{
		{"untiered weapon is always proficient", "", "anything", true},
		{"lowest tier is always proficient", "simple", "club", true},
		{"granted tier is proficient", "martial", "battleaxe", true},
		{"granted category is proficient even out of tier", "exotic", "two-rivers-longbow", true},
		{"ungranted tier and category is not proficient", "exotic", "ashandarei", false},
	}
	for _, c := range cases {
		if got := Proficient(tiers, cats, c.wTier, c.wCat); got != c.want {
			t.Errorf("%s: Proficient(%q,%q) = %v, want %v", c.name, c.wTier, c.wCat, got, c.want)
		}
	}

	// No class grants: only the lowest tier / untiered is usable.
	if Proficient(nil, nil, "martial", "sword") {
		t.Error("no grants ⇒ martial weapon should NOT be proficient")
	}
	if !Proficient(nil, nil, "simple", "dagger") {
		t.Error("no grants ⇒ a simple (lowest-tier) weapon is still proficient (everyone has the lowest tier)")
	}
}

func TestDamageTypeVocabulary(t *testing.T) {
	for _, dt := range []string{"bludgeoning", "piercing", "slashing"} {
		if !ValidDamageType(dt) {
			t.Errorf("ValidDamageType(%q) = false, want true", dt)
		}
	}
	if ValidDamageType("fire") {
		t.Error("ValidDamageType(fire) = true, want false")
	}
	if ValidDamageType("") {
		t.Error(`ValidDamageType("") = true, want false`)
	}
}
