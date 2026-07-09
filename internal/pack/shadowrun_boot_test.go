package pack

import (
	"context"
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
	if _, err := regs.World.Area("shadowrun:seattle"); err != nil {
		t.Errorf("shadowrun area not loaded: %v", err)
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
