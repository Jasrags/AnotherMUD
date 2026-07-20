package pack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestLoad_ShadowrunBootSlice is the SR-M3c-1 gate: selecting the `shadowrun`
// pack boots {tapestry-core, shadowrun} via dependency closure, seeds a runner
// on the eight Shadowrun primaries + Edge, and stands them on a street corner —
// with the Stun monitor deriving its ceiling from Willpower (the SR-M3c-1 build
// step 0 fix: a formula-driven pool max that Effective alone can't evaluate).
func TestLoad_ShadowrunBootSlice(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	// Select only shadowrun; the dependency closure adds tapestry-core.
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	// The starter district loaded — a bootable room + its area.
	if _, err := regs.World.Room("shadowrun:street-corner"); err != nil {
		t.Errorf("shadowrun starter room not loaded: %v", err)
	}
	seattle, err := regs.World.Area("shadowrun:seattle")
	if err != nil {
		t.Errorf("shadowrun area not loaded: %v", err)
	} else {
		// The area property bag carries the gazetteer's classification metadata.
		if got, ok := seattle.PropertyString("region"); !ok || got != "seattle" {
			t.Errorf("seattle area region = %q (ok=%v), want seattle", got, ok)
		}
		if got, ok := seattle.PropertyString("security"); !ok || got != "A" {
			t.Errorf("seattle area security = %q (ok=%v), want A", got, ok)
		}
		if got, ok := seattle.PropertyString("level_range"); !ok || got != "1-10" {
			t.Errorf("seattle area level_range = %q (ok=%v), want 1-10", got, ok)
		}
	}

	// The demo Armorer recipe registered (crafting-and-cooking + web-client-plan
	// P3 Slice B craft form): namespaced id, resolvable inputs/output. Confirms
	// the pack's recipes/ glob is wired and the recipe survived the loader's
	// output-template validation (an unknown output would be silently skipped).
	rec, err := regs.Recipes.Get("shadowrun:handload-apds")
	if err != nil {
		t.Errorf("demo recipe shadowrun:handload-apds not registered: %v", err)
	} else {
		if rec.Discipline != "armorer" {
			t.Errorf("handload-apds discipline = %q, want armorer", rec.Discipline)
		}
		if rec.Output.Template != "shadowrun:apds-round" {
			t.Errorf("handload-apds output = %q, want shadowrun:apds-round", rec.Output.Template)
		}
		// Every input template must resolve against the registered item set (an
		// unresolvable input would make the recipe uncraftable at runtime); the
		// two demo components are the tungsten dart + the caseless round.
		wantInputs := map[string]bool{"shadowrun:tungsten-dart": false, "shadowrun:caseless-round": false}
		for _, in := range rec.Inputs {
			if _, err := regs.Items.Get(item.TemplateID(in.Template)); err != nil {
				t.Errorf("handload-apds input %q not a registered item template", in.Template)
			}
			if _, expected := wantInputs[in.Template]; expected {
				wantInputs[in.Template] = true
			}
		}
		for tpl, seen := range wantInputs {
			if !seen {
				t.Errorf("handload-apds missing expected input %q (inputs=%+v)", tpl, rec.Inputs)
			}
		}
	}

	// The shadowrun `human` overrode the core baseline (priority 1) — proven by
	// a cap key only the SR metatype declares (agility isn't a classic attribute).
	human, ok := regs.Races.Get("human")
	if !ok {
		t.Fatal("human metatype not loaded")
	}
	if _, hasAgility := human.StatCaps["agility"]; !hasAgility {
		t.Errorf("human StatCaps = %v, want an 'agility' cap (the SR override should win over core human)", human.StatCaps)
	}

	// Role×origin creation: a class's role "floor" kit (starting_items) is
	// namespace-qualified at decode (like a background's Items). A bare id would
	// silently never resolve at grant time, shipping the floor inert — so pin the
	// qualified shape here, at the real load path.
	sam, ok := regs.Classes.Get("street-samurai")
	if !ok {
		t.Fatal("street-samurai class not loaded")
	}
	if got := sam.StartingItems; len(got) != 1 || got[0] != "shadowrun:stun-baton" {
		t.Errorf("street-samurai StartingItems = %v, want [shadowrun:stun-baton] (namespace-qualified)", got)
	}
	face, ok := regs.Classes.Get("face")
	if !ok {
		t.Fatal("face class not loaded")
	}
	if got := face.StartingItems; len(got) != 1 || got[0] != "shadowrun:streetline-special" {
		t.Errorf("face StartingItems = %v, want [shadowrun:streetline-special] (namespace-qualified)", got)
	}

	// Role×origin creation origins: the three backgrounds load, each granting the
	// universal commlink among its always-granted items, and the two papered
	// origins carry a real SIN. Ids namespace-qualify at decode — a typo would
	// fail-soft to nothing at grant time, so pin a signature item per origin.
	for _, tc := range []struct{ id, wantItem string }{
		{"street-kid", "shadowrun:meta-link"},
		{"corporate-dropout", "shadowrun:corporate-sin"},
		{"ex-security", "shadowrun:national-sin"},
	} {
		bg, ok := regs.Backgrounds.Get(tc.id)
		if !ok {
			t.Fatalf("%s background not loaded", tc.id)
		}
		found := false
		// Every background grants a stimpatch in its always-on base gear
		// (basic self-care every runner carries). Item ids fail-soft on a
		// typo, so pin it here so a future edit can't silently drop it.
		foundStim := false
		for _, it := range bg.Items {
			if it == tc.wantItem {
				found = true
			}
			if it == "shadowrun:stimpatch" {
				foundStim = true
			}
		}
		if !found {
			t.Errorf("%s Items = %v, want to contain %s", tc.id, bg.Items, tc.wantItem)
		}
		if !foundStim {
			t.Errorf("%s Items = %v, want to contain shadowrun:stimpatch (base first aid)", tc.id, bg.Items)
		}
	}

	// The medical items back both the background grants above and the vendor
	// sells lists (ripperdoc / fixer / ares-arms-clerk / greasy-ben / street-doc
	// / the bars); a bad id fails soft in both, so confirm they registered.
	for _, id := range []string{"shadowrun:stimpatch", "shadowrun:medkit"} {
		if _, err := regs.Items.Get(item.TemplateID(id)); err != nil {
			t.Errorf("medical item %s not loaded: %v", id, err)
		}
	}

	// The world selects the Shadowrun attribute set (manifest attribute_set:).
	if got := regs.WorldAttributeSets["shadowrun"]; got != "shadowrun-primaries" {
		t.Errorf("WorldAttributeSets[shadowrun] = %q, want shadowrun-primaries", got)
	}
	srSet, ok := regs.AttributeSets.Get("shadowrun-primaries")
	if !ok {
		t.Fatal("shadowrun-primaries attribute set not loaded")
	}
	if got := len(srSet.Keys()); got != 9 {
		t.Errorf("shadowrun-primaries has %d attributes, want 9 (8 primaries + edge)", got)
	}

	// The Stun monitor loaded as a nonlethal, formula-driven pool.
	stun, ok := regs.Pools.Get("stun")
	if !ok {
		t.Fatal("stun pool not declared")
	}
	if stun.MaxFormula != "8 + ceil(willpower / 2)" {
		t.Errorf("stun MaxFormula = %q, want the willpower formula", stun.MaxFormula)
	}
	if !stun.Rules.Nonlethal || !stun.Rules.DepletionEvent {
		t.Errorf("stun rules = %+v, want nonlethal + depletion_event", stun.Rules)
	}
	if !stun.SeedOnPlayer || !stun.SeedOnMob {
		t.Errorf("stun seeds player=%v mob=%v, want both true", stun.SeedOnPlayer, stun.SeedOnMob)
	}

	// The combat channel map remapped defense onto SR primaries (not the core
	// baseline `ac`). Reaction 3 + Intuition 3 = 6.
	mapping, err := regs.ChannelMap.Build()
	if err != nil {
		t.Fatalf("build channel map: %v", err)
	}
	srBase := progression.SeedBaseFromSet(srSet)
	sb := progression.NewWithBase(srBase)
	lookup := func(name string) int { return sb.Effective(progression.StatType(name)) }
	// All four channels derive off the SR primaries (defaults 3 each), not the
	// core baseline (which reads hit_mod/ac/str this world doesn't seed).
	if got := mapping.Value(channel.Attack, lookup); got != 3 {
		t.Errorf("attack channel = %d, want 3 (agility 3; weapon skill adds via proficiency)", got)
	}
	if got := mapping.Value(channel.Defense, lookup); got != 6 {
		t.Errorf("defense channel = %d, want 6 (reaction 3 + intuition 3)", got)
	}
	if got := mapping.Value(channel.DamageBonus, lookup); got != 3 {
		t.Errorf("damage_bonus channel = %d, want 3 (strength 3)", got)
	}
	// mitigation = body + armor; `armor` is unwired until SR-M3c-2, so it reads
	// 0 → body alone (3). This asserts the c-1 degradation is intentional.
	if got := mapping.Value(channel.Mitigation, lookup); got != 3 {
		t.Errorf("mitigation channel = %d, want 3 (body 3 + armor 0, armor unwired in c-1)", got)
	}

	// END-TO-END: seed the Stun monitor onto the SR-seeded stat block through the
	// real seeder. Willpower defaults to 3, so 8 + ceil(3/2) = 8 + 2 = 10 — NOT 0,
	// which is exactly the SR-M3c-1 build-step-0 failure this proves is fixed.
	set := pool.NewSet()
	entities.SeedPoolInto(set, sb, "stun", progression.StatType(stun.MaxChannel), stun.MaxFormula, stun.Rules)
	p, ok := set.Get("stun")
	if !ok {
		t.Fatal("stun pool was not seeded")
	}
	if got := p.Max(); got != 10 {
		t.Fatalf("seeded stun max = %d, want 10 (8 + ceil(willpower 3 / 2)); 0 would mean the formula seam is broken", got)
	}
}

// TestLoad_ShadowrunEssencePool is the SR-M4 gate: the `essence` pool loads with
// a constant tenths ceiling (max_formula "60" == 6.0), degrades the `magic`
// pool, seeds on the player only, and the three cyberware items carry their
// authored decimal essence_cost converted to integer tenths.
func TestLoad_ShadowrunEssencePool(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	essence, ok := regs.Pools.Get("essence")
	if !ok {
		t.Fatal("essence pool not declared")
	}
	if essence.MaxFormula != "60" {
		t.Errorf("essence MaxFormula = %q, want the constant \"60\" (6.0 in tenths)", essence.MaxFormula)
	}
	if essence.Rules.Degrades != "magic" {
		t.Errorf("essence degrades %q, want magic", essence.Rules.Degrades)
	}
	if essence.Rules.DepletionEvent || essence.Rules.Nonlethal {
		t.Errorf("essence rules = %+v, want no depletion_event/nonlethal (a build limit, not a KO)", essence.Rules)
	}
	if !essence.SeedOnPlayer || essence.SeedOnMob {
		t.Errorf("essence seeds player=%v mob=%v, want player-only", essence.SeedOnPlayer, essence.SeedOnMob)
	}

	// The constant formula seeds a full 6.0 (60 tenths) — no attribute vars, so
	// the ceiling never moves. A runner starts at full humanity.
	srSet, ok := regs.AttributeSets.Get("shadowrun-primaries")
	if !ok {
		t.Fatal("shadowrun-primaries attribute set not loaded")
	}
	sb := progression.NewWithBase(progression.SeedBaseFromSet(srSet))
	set := pool.NewSet()
	entities.SeedPoolInto(set, sb, "essence", progression.StatType(essence.MaxChannel), essence.MaxFormula, essence.Rules)
	p, ok := set.Get("essence")
	if !ok {
		t.Fatal("essence pool was not seeded")
	}
	if cur, max := p.Snapshot(); cur != 60 || max != 60 {
		t.Fatalf("seeded essence = %d/%d, want 60/60 (6.0 full)", cur, max)
	}

	// The cyberware items carry their SR decimal essence_cost as integer tenths.
	for _, tc := range []struct {
		id   string
		want int
	}{
		{"wired-reflexes", 20},     // 2.0
		{"muscle-replacement", 10}, // 1.0
		{"cybereyes", 2},           // 0.2
	} {
		it, err := regs.Items.Get(item.TemplateID("shadowrun:" + tc.id))
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if it.EssenceCost != tc.want {
			t.Errorf("%s essence_cost = %d tenths, want %d", tc.id, it.EssenceCost, tc.want)
		}
	}
}

// TestLoad_ShadowrunMetatypes is the SR-M3c-2 metatype-roster gate: all five
// metatypes load, each overriding the core baseline (priority 1), and the four
// metahumans carry their identity as distinct attribute CAPS + size + a starting
// StatBonuses skew (sr-m3c-deferred-fixes: the starting-attribute bonus + the
// flat hp_max Physical-monitor bump both shipped) — a metatype's edge is its
// ceiling, frame, AND a higher seed.
func TestLoad_ShadowrunMetatypes(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	for _, id := range []string{"human", "elf", "dwarf", "ork", "troll"} {
		if !regs.Races.Has(id) {
			t.Errorf("metatype %q not loaded", id)
		}
	}

	// Identity spot-checks: the caps that make each metatype itself.
	troll, _ := regs.Races.Get("troll")
	if troll.StatCaps["body"] != 10 || troll.StatCaps["strength"] != 10 {
		t.Errorf("troll body/strength caps = %d/%d, want 10/10", troll.StatCaps["body"], troll.StatCaps["strength"])
	}
	if troll.Size != "large" {
		t.Errorf("troll size = %q, want large (size-and-wielding)", troll.Size)
	}
	// Starting skew: the troll's body/strength seed bonus + its flat hp_max
	// Physical-monitor bump (the largest metatype toughness bonus).
	if troll.StatBonuses["body"] != 4 || troll.StatBonuses["strength"] != 4 {
		t.Errorf("troll body/strength bonus = %d/%d, want 4/4", troll.StatBonuses["body"], troll.StatBonuses["strength"])
	}
	if troll.StatBonuses["hp_max"] != 6 {
		t.Errorf("troll hp_max bonus = %d, want 6", troll.StatBonuses["hp_max"])
	}
	dwarf, _ := regs.Races.Get("dwarf")
	if dwarf.Size != "small" {
		t.Errorf("dwarf size = %q, want small", dwarf.Size)
	}
	if elf, _ := regs.Races.Get("elf"); elf.StatCaps["charisma"] != 8 {
		t.Errorf("elf charisma cap = %d, want 8", elf.StatCaps["charisma"])
	}
	if ork, _ := regs.Races.Get("ork"); ork.StatCaps["logic"] != 5 {
		t.Errorf("ork logic cap = %d, want 5 (capped, the sprawl's prejudice in numbers)", ork.StatCaps["logic"])
	}
}

// TestLoad_ShadowrunWeaponsAndArmor is the SR-M3c-2 arsenal gate: the weapons
// and armour decode with their combat identity — crucially the stun baton routes
// to the Stun monitor (target_pool) while lethal weapons take the default hp
// (Physical) path, and armour carries an armor_bonus the soak reads.
func TestLoad_ShadowrunWeaponsAndArmor(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	// The stun baton routes to the Stun monitor; nothing else does.
	baton, err := regs.Items.Get("shadowrun:stun-baton")
	if err != nil {
		t.Fatalf("stun-baton: %v", err)
	}
	if baton.TargetPool != "stun" {
		t.Errorf("stun-baton target_pool = %q, want stun (routes to the Stun monitor → KO)", baton.TargetPool)
	}
	for _, id := range []string{"katana", "ares-predator-v", "smg"} {
		w, err := regs.Items.Get(item.TemplateID("shadowrun:" + id))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if w.TargetPool != "" {
			t.Errorf("%s target_pool = %q, want empty (lethal → hp/Physical default path)", id, w.TargetPool)
		}
	}

	// Firearms are ranged and feed on `bullet` ammo.
	pistol, _ := regs.Items.Get("shadowrun:ares-predator-v")
	if pistol.RangedClass != "projectile" || pistol.AmmoKind != "bullet" {
		t.Errorf("ares-predator-v ranged = (%q,%q), want (projectile, bullet)", pistol.RangedClass, pistol.AmmoKind)
	}
	// The Predator V is holder-fed (SR5 "15 (c)"): it takes a heavy-pistol clip
	// and carries no rounds itself.
	if pistol.AcceptsHolder != "heavy-pistol" || pistol.Magazine != 0 {
		t.Errorf("ares-predator-v = (accepts_holder %q, magazine %d), want (heavy-pistol, 0)", pistol.AcceptsHolder, pistol.Magazine)
	}
	// The clip is the ammunition holder: capacity 15, fits the heavy-pistol
	// family, feeds `bullet` rounds.
	clip, err := regs.Items.Get("shadowrun:predator-clip")
	if err != nil {
		t.Fatalf("predator-clip: %v", err)
	}
	if clip.Magazine != 15 || clip.HolderFits != "heavy-pistol" || clip.AmmoKind != "bullet" {
		t.Errorf("predator-clip = (magazine %d, holder_fits %q, ammo_kind %q), want (15, heavy-pistol, bullet)",
			clip.Magazine, clip.HolderFits, clip.AmmoKind)
	}

	// Armour carries the soak rating the channel map reads through `armor`.
	jacket, err := regs.Items.Get("shadowrun:armored-jacket")
	if err != nil {
		t.Fatalf("armored-jacket: %v", err)
	}
	if jacket.ArmorBonus != 3 {
		t.Errorf("armored-jacket armor_bonus = %d, want 3", jacket.ArmorBonus)
	}
	hasBody := false
	for _, s := range jacket.EligibleSlots {
		if s == "body" {
			hasBody = true
		}
	}
	if !hasBody {
		t.Errorf("armored-jacket eligible_slots = %v, want to include body", jacket.EligibleSlots)
	}

	// The heavier vest completes the pair (bonus + body slot).
	vest, err := regs.Items.Get("shadowrun:armor-vest")
	if err != nil {
		t.Fatalf("armor-vest: %v", err)
	}
	if vest.ArmorBonus != 4 {
		t.Errorf("armor-vest armor_bonus = %d, want 4", vest.ArmorBonus)
	}
	hasBody = false
	for _, s := range vest.EligibleSlots {
		if s == "body" {
			hasBody = true
		}
	}
	if !hasBody {
		t.Errorf("armor-vest eligible_slots = %v, want to include body", vest.EligibleSlots)
	}
}

// TestLoad_ShadowrunCurrency proves the currency-label seam: the shadowrun world
// declares nuyen/¥ in its manifest, which the loader records in WorldCurrencies
// for the composition root to resolve boot-wide. A world with no `currency:`
// block (fantasy) is absent from the map and falls back to the gold default.
func TestLoad_ShadowrunCurrency(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	cur, ok := regs.WorldCurrencies["shadowrun"]
	if !ok {
		t.Fatal("shadowrun world declared no currency — the manifest currency: block didn't load")
	}
	if cur.Noun != "nuyen" || cur.Suffix != "¥" {
		t.Errorf("shadowrun currency = (%q, %q), want (nuyen, ¥)", cur.Noun, cur.Suffix)
	}
	// The reskin flows through to display: "725¥", label "Nuyen".
	if got := cur.Format(725); got != "725¥" {
		t.Errorf("Format(725) = %q, want 725¥", got)
	}
	if got := cur.Title(); got != "Nuyen" {
		t.Errorf("Title() = %q, want Nuyen", got)
	}
}

// TestLoad_ShadowrunAdvancement gates the karma-ledger strategy selection
// (SR-M5): the shadowrun manifest's `advancement: karma-ledger` must land in
// WorldAdvancement so the session actor routes rewards into a karma balance
// instead of onto a progression track. A level-track world (starter-world) is
// absent from the map — the default.
func TestLoad_ShadowrunAdvancement(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}
	if got := regs.WorldAdvancement["shadowrun"]; got != AdvancementKarmaLedger {
		t.Errorf("shadowrun advancement = %q, want %q", got, AdvancementKarmaLedger)
	}
	// A level-track world is not in the map — the default strategy needs no entry.
	if _, ok := regs.WorldAdvancement["tapestry-core"]; ok {
		t.Errorf("the engine baseline should carry no advancement entry (it is not a world)")
	}
	// The manifest's karma_costs block lands in WorldKarmaCosts at the SR canon values.
	if kc := regs.WorldKarmaCosts["shadowrun"]; kc.SkillMult != 2 || kc.AttributeMult != 5 {
		t.Errorf("shadowrun karma costs = %+v, want {SkillMult:2 AttributeMult:5}", kc)
	}
	// The two SR-owned karma qualities loaded, each carrying a karma_cost.
	for id, wantCost := range map[string]int{"ambidextrous": 4, "high-pain-tolerance": 7} {
		f, ok := regs.Feats.Get(id)
		if !ok {
			t.Errorf("SR quality %q did not load", id)
			continue
		}
		if f.KarmaCost != wantCost {
			t.Errorf("%q karma_cost = %d, want %d", id, f.KarmaCost, wantCost)
		}
	}
}

// TestLoad_UnknownAdvancementRejected proves a typo in the manifest
// `advancement:` field is a hard load error, not a silent fall-through to
// level-track (SR-M5) — the karma-vs-XP routing must never depend on a
// misspelling passing unnoticed.
func TestLoad_UnknownAdvancementRejected(t *testing.T) {
	dir := t.TempDir()
	writeManifest := func(name, body string) string {
		d := filepath.Join(dir, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(d, "pack.yaml"), []byte(body), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		return d
	}
	writeManifest("badworld", "name: badworld\nkind: world\nsplash: splash.txt\nadvancement: karma_ledger\n")
	if err := os.WriteFile(filepath.Join(dir, "badworld", "splash.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write splash: %v", err)
	}
	regs := NewRegistries()
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	err := Load(context.Background(), dir, []string{"badworld"}, regs, nil, nil, nil)
	if err == nil {
		t.Fatal("Load accepted an unknown advancement strategy — a typo silently fell through to level-track")
	}
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("error = %v, want ErrInvalidContent", err)
	}
}

// TestLoad_ShadowrunBallisticProjectiles gates the silent-weapon set (WEAPONS.md
// Ballistic Projectiles): the bow feeds loose arrows (no reload gate), the three
// crossbows are reload-gated and chamber a bolt via `load`. All are lethal
// Physical (no target_pool) and share the core bow/crossbow flavor voices.
func TestLoad_ShadowrunBallisticProjectiles(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	// The bow: a projectile feeding loose `arrow` ammo, no reload gate, no holder.
	bow, err := regs.Items.Get("shadowrun:bow")
	if err != nil {
		t.Fatalf("bow: %v", err)
	}
	if bow.RangedClass != "projectile" || bow.AmmoKind != "arrow" {
		t.Errorf("bow ranged = (%q,%q), want (projectile, arrow)", bow.RangedClass, bow.AmmoKind)
	}
	if bow.ReloadTicks != 0 {
		t.Errorf("bow reload_ticks = %d, want 0 (loose-round feed, no reload gate)", bow.ReloadTicks)
	}
	if bow.TargetPool != "" || bow.AcceptsHolder != "" || bow.Magazine != 0 {
		t.Errorf("bow = (target_pool %q, accepts_holder %q, magazine %d), want (\"\",\"\",0)",
			bow.TargetPool, bow.AcceptsHolder, bow.Magazine)
	}

	// The three crossbows: reload-gated, bolt-fed, escalating reload cost.
	for _, tc := range []struct {
		id    string
		ticks int
	}{
		{"light-crossbow", 20},
		{"medium-crossbow", 35},
		{"heavy-crossbow", 50},
	} {
		xbow, err := regs.Items.Get(item.TemplateID("shadowrun:" + tc.id))
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if xbow.RangedClass != "projectile" || xbow.AmmoKind != "bolt" {
			t.Errorf("%s ranged = (%q,%q), want (projectile, bolt)", tc.id, xbow.RangedClass, xbow.AmmoKind)
		}
		if xbow.ReloadTicks != tc.ticks {
			t.Errorf("%s reload_ticks = %d, want %d", tc.id, xbow.ReloadTicks, tc.ticks)
		}
		if xbow.TargetPool != "" {
			t.Errorf("%s target_pool = %q, want empty (lethal → hp/Physical default path)", tc.id, xbow.TargetPool)
		}
	}

	// The ammo: loose arrows + bolts, each matched verbatim against a weapon's
	// ammo_kind. Neither is a holder.
	for _, tc := range []struct{ id, kind string }{
		{"arrow", "arrow"},
		{"bolt", "bolt"},
	} {
		ammo, err := regs.Items.Get(item.TemplateID("shadowrun:" + tc.id))
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if ammo.AmmoKind != tc.kind || ammo.HolderFits != "" {
			t.Errorf("%s = (ammo_kind %q, holder_fits %q), want (%q, \"\")", tc.id, ammo.AmmoKind, ammo.HolderFits, tc.kind)
		}
	}
}

// TestLoad_ShadowrunClassAndBackground is the SR-M3c-3 creation-content gate:
// the Street Samurai class, its bound world track, and the Street Kid background
// load with the fields the creation flow + level-up + granter read. The default
// creation flow (giftGated=false) offers these directly — Shadowrun needs no
// custom flow, so there is no case in CreationFlowFor.
func TestLoad_ShadowrunClassAndBackground(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	// The world advancement track (SR classes bind here, not a core track).
	if _, ok := regs.Tracks.Get("street"); !ok {
		t.Error("street track not loaded")
	}

	sam, ok := regs.Classes.Get("street-samurai")
	if !ok {
		t.Fatal("street-samurai class not loaded")
	}
	if sam.BoundTrack != "street" {
		t.Errorf("street-samurai bound_track = %q, want street", sam.BoundTrack)
	}
	if !containsStr(sam.ProficiencyTiers, "simple") || !containsStr(sam.ProficiencyTiers, "martial") {
		t.Errorf("street-samurai proficiency tiers = %v, want simple+martial", sam.ProficiencyTiers)
	}
	if !containsStr(sam.ArmorProficiencyTiers, "light") || !containsStr(sam.ArmorProficiencyTiers, "medium") {
		t.Errorf("street-samurai armor tiers = %v, want light+medium", sam.ArmorProficiencyTiers)
	}
	// The level-1 combat kit is reused core abilities.
	granted := map[string]bool{}
	for _, p := range sam.Path {
		granted[p.AbilityID] = true
	}
	if !granted["basic-strike"] {
		t.Errorf("street-samurai path = %v, want basic-strike granted", granted)
	}

	bg, ok := regs.Backgrounds.Get("street-kid")
	if !ok {
		t.Fatal("street-kid background not loaded")
	}
	if bg.Gold != 500 {
		t.Errorf("street-kid gold = %d, want 500 (starting nuyen)", bg.Gold)
	}
	if len(bg.FeatOptions) != 2 {
		t.Errorf("street-kid feat_options = %v, want 2 (a pick-one chooser)", bg.FeatOptions)
	}
	if len(bg.EquipmentPackages) != 3 {
		t.Errorf("street-kid equipment_packages count = %d, want 3", len(bg.EquipmentPackages))
	}
	if len(bg.Skills) == 0 {
		t.Error("street-kid grants no skills, want the barrens stealth kit")
	}
}

func containsStr(ss []string, want string) bool {
	return slices.Contains(ss, want)
}

// TestLoad_ShadowrunDistrictAndMobs is the SR-M3c-3 district gate: the walkable
// district loads with its population placed, and the two mobs carry the combat
// identity a live gunfight needs — the hostile ganger (starts it) and the
// neutral corp-sec guard (finishes it), each armed, armoured, and dropping
// nuyen.
func TestLoad_ShadowrunDistrictAndMobs(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	for _, id := range []string{"street-corner", "back-alley", "market-street", "corp-plaza"} {
		if _, err := regs.World.Room("shadowrun:" + world.RoomID(id)); err != nil {
			t.Errorf("room shadowrun:%s not loaded: %v", id, err)
		}
	}

	// The neon sprawl is navigable after dark: the seattle area's `light_floor:
	// dim` bakes onto every room, so a torchless runner reads Dim (not Gloom) at
	// night. Without it the outdoor rooms ride the natural sky and go dark. This
	// is the "move around the MVP area" fix.
	corner, err := regs.World.Room("shadowrun:street-corner")
	if err != nil {
		t.Fatalf("street-corner: %v", err)
	}
	if got, ok := corner.PropertyString("light_floor"); !ok || got != "dim" {
		t.Errorf("street-corner light_floor = (%q,%v), want (dim,true) baked from the seattle area", got, ok)
	}
	res := light.NewResolver(light.DefaultConfig(), fixedNight{})
	if got := res.Effective(corner, light.Black, light.Black); got != light.Dim {
		t.Errorf("street-corner at night = %v, want Dim (neon floor lifts the dark → navigable)", got)
	}

	ganger, err := regs.Mobs.Get("shadowrun:ganger")
	if err != nil {
		t.Fatalf("ganger: %v", err)
	}
	if ganger.DispositionRules == nil || ganger.DispositionRules.Default != mob.ReactionHostile {
		t.Errorf("ganger disposition = %v, want hostile", ganger.DispositionRules)
	}
	if ganger.XPValue != 30 {
		t.Errorf("ganger xp_value = %d, want 30", ganger.XPValue)
	}
	if ganger.LootTable != "shadowrun:ganger-loot" {
		t.Errorf("ganger loot_table = %q, want shadowrun:ganger-loot", ganger.LootTable)
	}
	if !containsStr(ganger.Equipment, "shadowrun:katana") {
		t.Errorf("ganger equipment = %v, want a katana", ganger.Equipment)
	}

	guard, err := regs.Mobs.Get("shadowrun:sec-guard")
	if err != nil {
		t.Fatalf("sec-guard: %v", err)
	}
	if guard.DispositionRules == nil || guard.DispositionRules.Default != mob.ReactionNeutral {
		t.Errorf("sec-guard disposition = %v, want neutral", guard.DispositionRules)
	}
	if !containsStr(guard.Equipment, "shadowrun:smg") || !containsStr(guard.Equipment, "shadowrun:armor-vest") {
		t.Errorf("sec-guard equipment = %v, want smg + armor-vest", guard.Equipment)
	}

	// Nuyen is a currency item (auto-converts to balance on pickup/loot).
	nuyen, err := regs.Items.Get("shadowrun:nuyen")
	if err != nil {
		t.Fatalf("nuyen: %v", err)
	}
	isCurrency := false
	for _, tag := range nuyen.Tags {
		if tag == "currency" {
			isCurrency = true
		}
	}
	if !isCurrency {
		t.Errorf("nuyen tags = %v, want to include currency", nuyen.Tags)
	}
}
