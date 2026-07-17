package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/faction"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// scoreSubject is the identity/resource read surface the score sheet
// needs beyond the base Actor (vitals come from combat.Combatant, level
// from ProgressionHolder). The production connActor satisfies it; tests
// use a fake. The handler type-asserts and renders whatever is present,
// so a minimal actor degrades to just its name rather than erroring.
type scoreSubject interface {
	RaceID() string
	ClassID() string
	BackgroundID() string
	Gender() string
	Alignment() int
	AlignmentTag() string
	Gold() int
	Sustenance() int
	Mana() int
	ManaMax() int
	Movement() int
	MovementMax() int
	// Essence / EssenceMax are the Shadowrun Essence budget in TENTHS (SR-M4;
	// 60 == 6.0). A world with no essence pool reads 0/0, which the sheet takes
	// as "no Essence" and omits the line — so the field is inert everywhere but
	// Shadowrun.
	Essence() int
	EssenceMax() int
	StatValue(progression.StatType) int
	Saves() progression.Saves
	// AttributeSet is the character's resolved base attribute set (SR-M1), so
	// the sheet renders the world's declared attributes in order rather than a
	// hardcoded six. nil → the classic-six fallback.
	AttributeSet() *progression.AttributeSet
}

// ScoreHandler implements `score` (aliased `sc`) — the player's character
// sheet: identity, level/track, vitals, the six attributes, AC/hit,
// alignment, gold, sustenance, trains, AND the full worn-equipment list.
// Self-only; `consider <target>` sizes up others. Rendered as a framed,
// color-tagged bento panel (render.Panel, ui-rendering-help §8); color
// degrades cleanly and the frame is ASCII so no glyph fallback is needed.
// smartlinkChecker is the optional capability the score sheet probes to show the
// smartlink↔smartgun pairing status (item-modification §6). connActor satisfies
// it; a test fake that doesn't simply omits the line.
type smartlinkChecker interface {
	HasEquippedCapability(key string) bool
	WieldedWeaponHasCapability(key string) bool
}

func ScoreHandler(ctx context.Context, c *Context) error {
	d := scoreData{Name: c.Actor.Name()}
	if sc, ok := c.Actor.(smartlinkChecker); ok {
		d.Smartlink = sc.HasEquippedCapability("smartlink") && sc.WieldedWeaponHasCapability("smartgun")
	}

	if ss, ok := c.Actor.(scoreSubject); ok {
		d.Gender = titleCase(ss.Gender())
		d.Race = titleCase(ss.RaceID())
		d.Class = titleCase(ss.ClassID())
		d.Background = titleCase(ss.BackgroundID())
		d.HasResources = true
		d.Mana, d.MaxMana = ss.Mana(), ss.ManaMax()
		d.MV, d.MaxMV = ss.Movement(), ss.MovementMax()
		d.Essence, d.MaxEssence = ss.Essence(), ss.EssenceMax()
		d.HasStats = true
		// The world's declared attributes in order (SR-M1); each abbrev falls
		// back to the uppercased id. A boot with no attribute content (set nil)
		// drops to the classic-six fields below.
		if set := ss.AttributeSet(); set != nil && len(set.Attributes) > 0 {
			for _, at := range set.Attributes {
				ab := at.Abbrev
				if ab == "" {
					// Fallback for content that omitted an abbrev; cap the
					// width so a long id can't crowd the second grid column.
					ab = strings.ToUpper(string(at.ID))
					if len(ab) > 4 {
						ab = ab[:4]
					}
				}
				d.Attrs = append(d.Attrs, scoreAttr{
					Abbrev:   ab,
					Value:    ss.StatValue(at.ID),
					Category: at.Category,
				})
			}
		} else {
			d.STR = ss.StatValue(progression.StatSTR)
			d.INT = ss.StatValue(progression.StatINT)
			d.WIS = ss.StatValue(progression.StatWIS)
			d.DEX = ss.StatValue(progression.StatDEX)
			d.CON = ss.StatValue(progression.StatCON)
			d.LUCK = ss.StatValue(progression.StatLUCK)
		}
		// Saving throws (saves §2/§4): derived from class + ability mods.
		d.HasSaves = true
		sv := ss.Saves()
		d.Fort, d.Reflex, d.Will = sv.Fortitude, sv.Reflex, sv.Will
		d.HasAlign = true
		// AlignmentTag returns the raw tag id ("alignment_neutral"); show
		// the bare word ("neutral") on the sheet.
		d.Align = ss.Alignment()
		d.AlignTag = strings.TrimPrefix(ss.AlignmentTag(), "alignment_")
		d.HasGold = true
		d.Gold = ss.Gold()
		d.Money = c.Money
		d.HasSust = true
		d.Sust = ss.Sustenance()
		// Tier thresholds aren't externally configurable (only the drain
		// rate is — Phase 2), so the default config's tiers match the live
		// service. If thresholds ever become configurable, read them off a
		// threaded SustenanceService instead.
		d.SustTier = titleCase(string(economy.DefaultSustenanceConfig().TierOf(d.Sust)))
	}

	if cb, ok := c.Actor.(combat.Combatant); ok {
		d.HasVitals = true
		d.HP, d.MaxHP = cb.Vitals().Snapshot()
		st := cb.Stats()
		d.AC, d.Hit = st.AC, st.HitMod
	}

	// Faction standing (faction.md §6): one row per faction the character has
	// touched (a present entry in the standing bag), so a fresh sheet stays
	// uncluttered. The full per-faction list is the `standing` verb.
	if c.Faction != nil {
		if fe, ok := c.Actor.(faction.Entity); ok {
			for _, def := range c.Faction.Registry().All() {
				if v, present := fe.Standing(def.ID); present {
					name := def.Name
					if name == "" {
						name = def.ID
					}
					rank := def.RankOf(v)
					if rank == "" {
						rank = "—" // standing below the lowest rung (matches the `standing` verb)
					}
					d.Standings = append(d.Standings, fmt.Sprintf("%s (%s)", name, rank))
				}
			}
		}
	}

	// Trains available to spend (training §; same surface the `train` verb
	// reads). Probed via an anonymous interface so the sheet does not pull
	// in the training adapter type — a minimal actor simply omits the row.
	if th, ok := c.Actor.(interface{ TrainsAvailable() int }); ok {
		d.HasTrains = true
		d.Trains = th.TrainsAvailable()
	}

	// Saidin taint (WoT S2 Phase 4+). Probed via an anonymous interface (like
	// trains) so the sheet stays decoupled from the channeling adapter; shown
	// only once accrued, so it surfaces precisely when the curse takes hold.
	if mh, ok := c.Actor.(interface{ Madness() int }); ok {
		if m := mh.Madness(); m > 0 {
			d.HasMadness = true
			d.Madness = m
		}
	}

	// Channeling origin (WoT, v28). Probed via an anonymous interface (like
	// madness) so the sheet stays decoupled from the channeling adapter; shown
	// only when set, so it never clutters a non-WoT character's sheet.
	if gh, ok := c.Actor.(interface{ ChannelingGift() string }); ok {
		d.Gift = channelingGiftLabel(gh.ChannelingGift())
	}

	// Renown (reputation.md §3 / §7). Probed via an anonymous interface so the
	// sheet stays decoupled from the reputation adapter; shown whenever reputation
	// is wired (RenownTier returns a non-empty name), as a core attribute beside
	// alignment — Unknown included. EffectiveRenown folds in the Fame feat bonus
	// (§7), and Infamous flags the Infamy feat so the line reads as feared.
	if rh, ok := c.Actor.(interface {
		EffectiveRenown() int
		RenownTier() string
		Infamous() bool
	}); ok {
		if tier := rh.RenownTier(); tier != "" {
			d.HasRenown = true
			d.Renown = rh.EffectiveRenown()
			d.RenownTier = tier
			d.Infamous = rh.Infamous()
		}
	}

	// Known languages (languages.md §4). Probed via an anonymous interface (like
	// the gift) so the sheet stays decoupled; shown only when the character
	// knows at least one tongue, so it never clutters a language-less sheet.
	if lh, ok := c.Actor.(interface{ KnownLanguages() []string }); ok {
		d.Languages = strings.Join(lh.KnownLanguages(), ", ")
	}

	// Karma-ledger advancement (SR-M5). Probed via an anonymous interface so the
	// sheet stays decoupled from the session package; connActor satisfies it, a
	// test fake that doesn't simply omits the line. A karma-ledger character is
	// level-less, so this REPLACES the level/track block below (gated on
	// !d.HasKarma) — showing a phantom "Level 1" for a runner who never levels
	// would be a lie.
	if kh, ok := c.Actor.(interface {
		UsesKarmaLedger() bool
		KarmaBalance() (int64, int64)
	}); ok && kh.UsesKarmaLedger() {
		d.HasKarma = true
		d.KarmaCurrent, d.KarmaTotal = kh.KarmaBalance()
	}

	if ph, ok := c.Actor.(ProgressionHolder); ok && c.Progression != nil && !d.HasKarma {
		// Primary track = the actor's class bound_track (its own world's
		// advancement track), not the first-registered track — otherwise a
		// world-locked character (an SR street-samurai, a WoT armsman) shows the
		// engine-default "adventurer" that leaks in via the core dependency.
		// Falls back to the boot's configured default track for a classless
		// character (c.DefaultXPTrack, the ANOTHERMUD_DEFAULT_XP_TRACK knob),
		// then the engine constant — mirroring xp.go / GrantKillXP so score
		// never disagrees with where XP is actually granted.
		primary := c.DefaultXPTrack
		if primary == "" {
			primary = DefaultXPTrack
		}
		if pt, ok := c.Actor.(PrimaryTrackHolder); ok {
			primary = pt.PrimaryTrack(primary)
		}
		for _, td := range c.Progression.Tracks().All() {
			if !strings.EqualFold(td.Name, primary) {
				continue
			}
			info, ok := ph.TrackInfo(c.Progression, td.Name)
			if !ok {
				break
			}
			d.HasLevel = true
			d.Track = td.DisplayName
			if d.Track == "" {
				d.Track = td.Name
			}
			d.Level, d.XP, d.XpToNext = info.Level, info.XP, info.XpToNext
			d.AtMax = info.MaxLevel > 0 && info.Level >= info.MaxLevel
			break
		}
	}

	// Equipment: every slot in registration order (folded in from the old
	// standalone view — `eq` still renders the focused list). Occupied
	// slots carry the item name colored by rarity; empties read subtle.
	d.Equip = gatherScoreEquip(c)
	d.HasEquip = len(d.Equip) > 0

	return c.Actor.Write(ctx, renderScore(d))
}

// gatherScoreEquip walks the slot registry in order and returns one row
// per sub-slot: the compact slot name ("wield", "light") and the item name
// wrapped in its rarity tag (or a subtle "(empty)"). Both the score sheet
// and `eq` render these rows, so the two views list the same slots in the
// same order with the same labels. Returns nil when slots/items are
// unavailable.
func gatherScoreEquip(c *Context) []equipRow {
	if c.Items == nil || c.Slots == nil {
		return nil
	}
	// The world's attribute set gives modifier labels their display name
	// ("reaction" → "Reaction"); a boot/test actor without one falls back to a
	// humanized stat key. Read once for the whole gather.
	var attrs *progression.AttributeSet
	if ss, ok := c.Actor.(scoreSubject); ok {
		attrs = ss.AttributeSet()
	}
	equipped := c.Actor.Equipment()
	var rows []equipRow
	for _, def := range c.Slots.All() {
		for i := 0; i < def.Max; i++ {
			key, err := slot.BuildKey(def.Name, i, def.Max)
			if err != nil {
				continue
			}
			row := equipRow{Slot: def.Name, Name: "<subtle>(empty)</subtle>"}
			if id, ok := equipped[key]; ok {
				if e, ok := c.Items.GetByID(id); ok {
					if inst, ok := e.(*entities.ItemInstance); ok {
						tag := itemRarityTag(inst)
						row.Name = "<" + tag + ">" + inst.Name() + "</" + tag + ">"
						row.Effect = itemEffectSummary(attrs, inst)
						row.Ammo, row.AmmoType, row.AmmoLoaded = weaponAmmoState(inst)
					}
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// itemEffectSummary renders a compact, player-facing description of the
// mechanical benefits an equipped item grants — the "what does this provide"
// readout (ui-rendering-help §11: mechanics belong on the self-surface, never
// on the flavor-only `look` lens). It reads the same data the equip pipeline
// applies: the item's stat modifiers (+2 Reaction) and its structured armor
// bonus (Armor 3). Modifier labels resolve through the world's attribute set
// so they match the score sheet; a stat outside the set (or a nil set) falls
// back to a humanized key. Returns "" when the item grants nothing mechanical,
// so a plain item (a torch, a trophy) shows no tail.
func itemEffectSummary(attrs *progression.AttributeSet, inst *entities.ItemInstance) string {
	var parts []string
	for _, m := range inst.Modifiers() {
		parts = append(parts, fmt.Sprintf("%+d %s", m.Value, statLabel(attrs, m.Stat)))
	}
	if ab := inst.ArmorBonus(); ab != 0 {
		parts = append(parts, fmt.Sprintf("Armor %d", ab))
	}
	return strings.Join(parts, ", ")
}

// weaponAmmoState reports a wielded firearm's load state for the worn readout:
// a count tag ("15 rds", "empty", "12/15 rds"), the loaded rounds' type label
// ("APDS", "standard"; "" when empty), and whether it currently has rounds (for
// coloring). It covers the two firearm feed models — holder-fed (a pistol
// taking a clip: the inserted clip's remaining rounds + grade, or "empty" with
// no/spent clip) and internally-fed magazine (loaded/capacity + grade). A
// non-firearm (melee, thrown, a bow, or a reload-gated crossbow) returns
// ("", "", false), so the readout stays silent for everything a `reload`
// doesn't apply to. Answers "is my gun loaded, and with what?" from the worn
// view, before combat surfaces it as a dry click.
func weaponAmmoState(inst *entities.ItemInstance) (count, ammoType string, loaded bool) {
	switch {
	case inst.AcceptsHolder() != "":
		_, rounds, grade, has := inst.InsertedHolder()
		if has && rounds > 0 {
			return fmt.Sprintf("%d rds", rounds), ammoTypeLabel(grade), true
		}
		// No clip and a spent-clip-still-seated (has && rounds==0) both read as
		// "empty" — the player's next move is `reload` either way, so the
		// display need not distinguish them. Reload's smart-swap decision lives
		// in the session InsertHolder path, not here, so this collapse is
		// display-only. No rounds → no type.
		return "empty", "", false
	case inst.Magazine() > 0:
		// Internally-fed magazines ignore ammo grade (session.go ReloadWieldedMagazine
		// never sets holder_ammo_grade), so this reads "standard" for every such
		// weapon today — matching the SR5 simplification, not a missing feature.
		rounds := inst.MagazineLoaded()
		if rounds > 0 {
			return fmt.Sprintf("%d/%d rds", rounds, inst.Magazine()), ammoTypeLabel(inst.HolderAmmoGrade()), true
		}
		return fmt.Sprintf("%d/%d rds", rounds, inst.Magazine()), "", false
	}
	return "", "", false
}

// ammoTypeLabel renders a loaded round's type from its grade: the uppercased
// grade key names a special round ("apds" → "APDS"), and ungraded rounds read
// as "standard". This is the ammo-type surface for the worn + inventory
// readouts; grades carry no display name today, so the key is the label (works
// for acronym grades — a future word-grade would want a grade `name:` field).
func ammoTypeLabel(grade string) string {
	if grade == "" {
		return "standard"
	}
	return strings.ToUpper(grade)
}

// ammoTypeTag wraps an ammo-type label in its color tag: a special round pops
// in <highlight>, plain "standard" stays <subtle>. Empty label → no tag. Shared
// by the worn view (renderEquipRows) and the inventory listing so the two read
// consistently. "standard" is the sentinel ammoTypeLabel emits for ungraded
// rounds; no grade key is named "standard", so the compare is safe.
func ammoTypeTag(label string) string {
	if label == "" {
		return ""
	}
	tag := "highlight"
	if label == "standard" {
		tag = "subtle"
	}
	return "<" + tag + ">[" + label + "]</" + tag + ">"
}

// statLabel maps a modifier's raw stat key to its display label: the attribute
// set's declared Name when the key is one of the world's attributes, else a
// humanized fallback ("hit_mod" → "Hit mod"; titleCase caps only the first
// rune) so an engine-vital modifier still reads cleanly.
func statLabel(attrs *progression.AttributeSet, stat string) string {
	if attrs != nil {
		if at, ok := attrs.Get(progression.StatType(stat)); ok && at.Name != "" {
			return at.Name
		}
	}
	return titleCase(strings.ReplaceAll(stat, "_", " "))
}

// equipRow is one worn-equipment line: the compact slot name ("wield") and
// the item's display name, already wrapped in its rarity markup by the
// gatherer. Shared by the score sheet's Equipment column and `eq`.
type equipRow struct {
	Slot string
	Name string
	// Effect is the compact mechanical readout for an occupied slot
	// ("+2 Reaction", "Armor 3"), empty for an empty slot or an item that
	// grants nothing. Rendered by the `eq`/worn view (renderEquipRows); the
	// packed score-sheet cell omits it to keep its multi-column grid aligned.
	Effect string
	// Ammo is the count tag for a wielded firearm ("15 rds", "empty",
	// "12/15 rds"), empty for any non-firearm. AmmoLoaded drives its color
	// (green when there are rounds, red when empty). AmmoType is the loaded
	// rounds' type label ("APDS", "standard"; empty when the weapon is empty or
	// not a firearm). Like Effect, rendered by the worn view only.
	Ammo       string
	AmmoType   string
	AmmoLoaded bool
}

// scoreAttr is one attribute cell on the sheet (SR-M1): its short label, its
// effective value, and its category (physical/mental/special) for grouping.
type scoreAttr struct {
	Abbrev   string
	Value    int
	Category string
}

// scoreData is the rendered character sheet's data, gathered from the
// actor's interfaces. The Has* flags mark which sections the actor could
// supply so renderScore omits the rest (a minimal/test actor shows only
// its name).
type scoreData struct {
	Name        string
	Gender      string
	Race, Class string
	Background  string
	// Smartlink marks the smartlink↔smartgun pairing as active (a worn smartlink
	// + a wielded smartgun weapon, item-modification §6) so the sheet shows it.
	Smartlink bool
	// Languages is the comma-joined display names of the character's known
	// tongues (languages.md §4); empty hides the row.
	Languages string
	// Gift is the WoT channeling origin (spark/learn/none), shown as a friendly
	// phrase under the identity line when set; empty for non-WoT characters.
	Gift string

	HasVitals     bool
	HP, MaxHP     int
	HasResources  bool
	Mana, MaxMana int
	MV, MaxMV     int
	// Essence / MaxEssence are the Shadowrun Essence budget in tenths (SR-M4).
	// MaxEssence == 0 ⇒ the world has no Essence, so the sheet omits the line.
	Essence, MaxEssence int

	// HasStats is true when the actor satisfied scoreSubject (some attribute
	// data will render). The actual render path is chosen by len(Attrs): the
	// data-driven grid when non-empty, else the classic-six int fields.
	HasStats bool
	// Attrs is the world's declared attributes in render order (SR-M1). When
	// non-empty the sheet renders these (grouped by category); the STR..LUCK
	// fields below are the classic-six fallback for a boot with no attribute
	// content (and keep the older unit tests that populate them valid).
	Attrs                         []scoreAttr
	STR, INT, WIS, DEX, CON, LUCK int
	AC, Hit                       int

	HasSaves           bool
	Fort, Reflex, Will int

	// HasMadness shows the saidin-taint row only when a channeler has begun to
	// accrue it (Madness > 0) — it never clutters a non-channeler or a woman
	// (WoT S2 Phase 4+). The label coarsens the number into an ominous band.
	HasMadness bool
	Madness    int

	HasAlign bool
	AlignTag string
	Align    int

	// HasRenown shows the renown line (reputation.md §3) when reputation is
	// wired. RenownTier is the display tier name (e.g. "Known in the Region");
	// Renown is the effective score (base + Fame, §7); Infamous flags the Infamy
	// feat so the line reads as feared rather than admired (PD-5).
	HasRenown  bool
	RenownTier string
	Renown     int
	Infamous   bool

	// Standings is one pre-formatted "Faction (Rank)" string per faction the
	// character has *touched* (faction.md §6) — an untouched character shows no
	// standing rows, keeping a fresh sheet clean. The full list (including
	// untouched factions at their starting standing) is the `standing` verb.
	Standings []string

	HasGold bool
	Gold    int
	// Money is the world's currency-display vocabulary (nuyen/¥ vs gold),
	// copied from the Context so the purse row labels + formats correctly.
	Money economy.CurrencyLabel

	HasSust  bool
	Sust     int
	SustTier string

	HasTrains bool
	Trains    int

	HasLevel bool
	Track    string
	Level    int
	XP       int64
	XpToNext int64
	AtMax    bool

	// HasKarma shows the karma line (SR-M5) for a karma-ledger character, in
	// place of the level/track line (a karma-ledger world is level-less).
	// KarmaCurrent is the spendable balance; KarmaTotal is lifetime earned.
	HasKarma     bool
	KarmaCurrent int64
	KarmaTotal   int64

	HasEquip bool
	Equip    []equipRow
}

// scorePanelWidth is the fixed visible width of the score sheet. 80 is
// the classic terminal width and matches render.DefaultPanelWidth.
const scorePanelWidth = 80

// renderScore formats the sheet as a framed, color-tagged bento panel
// (render.Panel — ASCII frame, tag-aware width math). Pure: every section
// is gated on its Has* flag, so it is unit-testable without an actor. A
// minimal actor (name only) renders as the bare name, no frame.
func renderScore(d scoreData) string {
	if !(d.HasStats || d.HasVitals || d.HasLevel || d.HasKarma || d.HasEquip) {
		return d.Name
	}

	// Top-left: identity. Top-right: combat + vitals.
	var charCol, combatCol []string
	charCol = append(charCol, scHi(d.Name))
	// "Gender Race Class", omitting any empty part (gender is empty for
	// pre-v22 characters; race/class for the raceless/classless).
	if identity := strings.TrimSpace(strings.Join(nonEmpty(d.Gender, d.Race, d.Class), " ")); identity != "" {
		charCol = append(charCol, scHi(identity))
	}
	if d.Background != "" {
		charCol = append(charCol, scKV("Background", scHi(d.Background), 11))
	}
	if d.Smartlink {
		charCol = append(charCol, scKV("Smartlink", scHi("active"), 11))
	}
	if d.Gift != "" {
		charCol = append(charCol, scKV("The Power", scHi(d.Gift), 11))
	}
	if d.Languages != "" {
		charCol = append(charCol, scKV("Languages", scHi(d.Languages), 11))
	}
	if d.HasLevel {
		// ASCII separator (not "·") — panel width math is byte-based, so a
		// multi-byte glyph would over-count this row and drift the border.
		charCol = append(charCol, scHi(fmt.Sprintf("Level %d", d.Level))+" "+scSub("- "+d.Track))
		charCol = append(charCol, scXPLine(d))
	}
	if d.HasKarma {
		// SR-M5: a karma-ledger runner shows spendable karma + lifetime earned
		// where a level-track character shows level/track. "Karma: 40 (170 earned)".
		charCol = append(charCol, scKV("Karma",
			scHi(fmt.Sprintf("%d", d.KarmaCurrent))+" "+scSub(fmt.Sprintf("(%d earned)", d.KarmaTotal)), 11))
	}
	if d.HasAlign {
		charCol = append(charCol, scKV("Alignment", scHi(fmt.Sprintf("%s (%d)", d.AlignTag, d.Align)), 11))
	}
	if d.HasRenown {
		// Infamy (PD-5) reframes the same magnitude as feared rather than known.
		label := d.RenownTier
		if d.Infamous {
			label += " (infamous)"
		}
		charCol = append(charCol, scKV("Renown", scHi(fmt.Sprintf("%s (%d)", label, d.Renown)), 11))
	}
	if d.HasVitals {
		combatCol = append(combatCol,
			scKV("Armor Class", scHi(strconv.Itoa(d.AC)), 12),
			scKV("Hit Bonus", scHi(fmt.Sprintf("%+d", d.Hit)), 12),
			scKV("HP", scTier(d.HP, d.MaxHP, "hp", fmt.Sprintf("%d / %d", d.HP, d.MaxHP)), 12),
		)
	}
	if d.HasResources {
		// MA/MV are real pools (WoT S2): current/max like HP. Non-channelers
		// show 0/0 (no resource_max), which is the honest reading.
		combatCol = append(combatCol, scSub("MA")+" <mana>"+fmt.Sprintf("%d/%d", d.Mana, d.MaxMana)+
			"</mana>    "+scSub("MV")+" <mv>"+fmt.Sprintf("%d/%d", d.MV, d.MaxMV)+"</mv>")
	}
	if d.MaxEssence > 0 {
		// Essence (Shadowrun SR-M4): stored in tenths, shown as the SR decimal.
		// Only a world that declares an `essence` pool reaches here, so the line
		// is Shadowrun-only. Current falls as cyberware is installed.
		combatCol = append(combatCol, scKV("Essence",
			scHi(fmt.Sprintf("%s / %s", tenths(d.Essence), tenths(d.MaxEssence))), 12))
	}
	if d.HasSaves {
		// Fortitude / Reflex / Will (saves §2). Compact so the row fits the
		// Combat column width; values are signed (a negative ability mod can
		// push a weak save below zero).
		combatCol = append(combatCol, scKV("Saves",
			scHi(fmt.Sprintf("Fort %+d  Ref %+d  Will %+d", d.Fort, d.Reflex, d.Will)), 12))
	}
	if d.HasMadness {
		// Saidin taint (WoT S2 Phase 4+) — shown in alarming red, the number
		// coarsened into an ominous band so it reads as dread, not a stat to
		// optimize.
		combatCol = append(combatCol, scKV("Madness",
			"<danger>"+madnessBand(d.Madness)+"</danger>", 12))
	}

	// Lower row: three columns — Attributes, Purse & Training, and the worn
	// Equipment (compact slot names). Attributes is fixed-narrow; the other
	// two fill the remainder so neither the sustenance line nor item names
	// truncate (a third equal column would clip both).
	var attrLines, purseLines, equipLines []string
	if len(d.Attrs) > 0 {
		attrLines = scAttrGrid(d.Attrs)
	} else if d.HasStats {
		attrLines = append(attrLines,
			scAttr("STR", d.STR, "DEX", d.DEX),
			scAttr("INT", d.INT, "CON", d.CON),
			scAttr("WIS", d.WIS, "LUCK", d.LUCK),
		)
	}
	if d.HasGold {
		// Label + amount reskin per the world's currency (nuyen/¥ vs gold). The
		// <gold> color tag is the semantic "currency" tint (theme.yaml), reused
		// regardless of the noun. commafy keeps the thousands separator.
		purseLines = append(purseLines, scKV(d.Money.Title(),
			"<gold>"+commafy(int64(d.Gold))+d.Money.Symbol()+"</gold>", 12))
	}
	if d.HasSust {
		purseLines = append(purseLines, scKV("Sustenance",
			scTier(d.Sust, economy.MaxSustenance, "good", fmt.Sprintf("%s (%d/%d)", d.SustTier, d.Sust, economy.MaxSustenance)), 12))
	}
	if d.HasTrains {
		tag := "subtle"
		if d.Trains > 0 {
			tag = "good"
		}
		purseLines = append(purseLines, scKV("Trains", "<"+tag+">"+fmt.Sprintf("%d unspent", d.Trains)+"</"+tag+">", 12))
	}
	if d.HasEquip {
		for _, r := range d.Equip {
			equipLines = append(equipLines, scEquipCell(r))
		}
	}

	sections := []render.Section{
		{Rows: scColumns([]scCol{
			{title: "Character", lines: charCol},
			{title: "Combat", lines: combatCol},
		})},
	}
	var lower []scCol
	if len(attrLines) > 0 {
		lower = append(lower, scCol{title: "Attributes", width: scAttrColWidth, lines: attrLines})
	}
	if len(purseLines) > 0 {
		lower = append(lower, scCol{title: "Purse & Training", lines: purseLines})
	}
	if len(equipLines) > 0 {
		lower = append(lower, scCol{title: "Equipment", lines: equipLines})
	}
	if len(lower) > 0 {
		sections = append(sections, render.Section{SeparatorAbove: render.RuleMinor, Rows: scColumns(lower)})
	}

	// Faction standing (faction.md §6): a full-width section so long faction
	// names ("The Children of the Light") fit without truncating in the narrow
	// Character column. One line per touched faction; absent for a fresh sheet.
	if len(d.Standings) > 0 {
		standingLines := make([]string, 0, len(d.Standings))
		for _, s := range d.Standings {
			standingLines = append(standingLines, scHi(s))
		}
		sections = append(sections, render.Section{
			SeparatorAbove: render.RuleMinor,
			Rows:           scColumns([]scCol{{title: "Standing", lines: standingLines}}),
		})
	}

	out, err := render.Panel{Width: scorePanelWidth, Sections: sections}.Render()
	if err != nil {
		// Title overflow is the only error path and can't occur with the
		// short fixed titles here; degrade to the bare name rather than
		// surfacing a render error to the player.
		return d.Name
	}
	return out
}

// scAttrColWidth is the fixed visible width of the Attributes column in
// the lower section. "WIS 10   LUCK 12" (the widest attribute line) is 16
// columns; the Purse and Equipment columns fill the rest of the panel.
const scAttrColWidth = 16

// scCol is one column of a score section: a <title> header, the
// pre-formatted content lines, and a fixed visible width (0 = fill an
// equal share of the row's leftover width).
type scCol struct {
	title string
	width int
	lines []string
}

// scColumns builds a section's rows from N columns: a title row over the
// columns, then one cell-row per content line (shorter columns padded with
// blanks so the frame stays rectangular). The last column always fills so
// the row spans the full panel width regardless of fixed widths before it.
func scColumns(cols []scCol) []render.Row {
	if len(cols) == 0 {
		return nil
	}
	last := len(cols) - 1
	cell := func(j int, content string) render.Cell {
		fill := cols[j].width == 0 || j == last
		w := cols[j].width
		if fill {
			w = 0
		}
		return render.Cell{Content: content, Width: w, Fill: fill}
	}
	n := 0
	for _, c := range cols {
		if len(c.lines) > n {
			n = len(c.lines)
		}
	}
	titleCells := make([]render.Cell, len(cols))
	for j, c := range cols {
		titleCells[j] = cell(j, "<title>"+c.title+"</title>")
	}
	rows := []render.Row{render.CellRow(titleCells, true)}
	for i := 0; i < n; i++ {
		cells := make([]render.Cell, len(cols))
		for j, c := range cols {
			content := ""
			if i < len(c.lines) {
				content = c.lines[i]
			}
			cells[j] = cell(j, content)
		}
		rows = append(rows, render.CellRow(cells, true))
	}
	return rows
}

// scXPLine formats the experience line shown in the Character column:
// current XP plus either the remaining XP to the next level or a max-level
// marker. Plain text reads "XP 12,500  (2,500 to next level)".
func scXPLine(d scoreData) string {
	tail := "(" + commafy(d.XpToNext) + " to next level)"
	if d.AtMax {
		tail = "(max level)"
	}
	return scSub("XP") + " " + scHi(commafy(d.XP)) + "  " + scSub(tail)
}

// scKV formats "label<pad>value" with the label subtle and the value
// markup placed at a fixed visible column (labelW), so values line up
// down a column.
func scKV(label, valueMarkup string, labelW int) string {
	pad := max(labelW-len(label), 1)
	return scSub(label) + strings.Repeat(" ", pad) + valueMarkup
}

// scAttr formats two attributes on one line ("STR 16   DEX 14"), the
// first padded to a fixed visible width so the second column aligns.
func scAttr(s1 string, v1 int, s2 string, v2 int) string {
	left := scSub(s1) + " " + scHi(strconv.Itoa(v1))
	vis := len(s1) + 1 + len(strconv.Itoa(v1))
	pad := max(9-vis, 1)
	return left + strings.Repeat(" ", pad) + scSub(s2) + " " + scHi(strconv.Itoa(v2))
}

// scAttrOne formats a lone attribute (the left half of scAttr), for an
// odd-count category group.
func scAttrOne(s string, v int) string {
	return scSub(s) + " " + scHi(strconv.Itoa(v))
}

// scAttrCategoryOrder is the render order for attribute categories; unknown or
// empty categories sort after these, in first-seen order. Extend this if a new
// AttrCategory* constant is added in progression/attributeset.go — otherwise
// the new category renders in the trailing first-seen group rather than a
// deliberate slot.
var scAttrCategoryOrder = []string{
	progression.AttrCategoryPhysical,
	progression.AttrCategoryMental,
	progression.AttrCategorySpecial,
}

// scAttrGrid renders the world's declared attributes grouped by category, two
// per line within each group (a category never shares a line with the next),
// so a Shadowrun sheet reads as Physical / Mental / Special blocks and the
// classic six group the same way (SR-M1 step 4). Categories render in
// scAttrCategoryOrder, then any other category in first-seen order;
// attributes keep their declared order within a category.
func scAttrGrid(attrs []scoreAttr) []string {
	buckets := map[string][]scoreAttr{}
	var firstSeen []string
	for _, a := range attrs {
		if _, seen := buckets[a.Category]; !seen {
			firstSeen = append(firstSeen, a.Category)
		}
		buckets[a.Category] = append(buckets[a.Category], a)
	}

	// Known categories first (fixed order), then the rest in first-seen order.
	knownCats := map[string]bool{}
	var cats []string
	for _, c := range scAttrCategoryOrder {
		if _, ok := buckets[c]; ok {
			cats = append(cats, c)
			knownCats[c] = true
		}
	}
	for _, c := range firstSeen {
		if !knownCats[c] {
			cats = append(cats, c)
		}
	}

	var lines []string
	for _, c := range cats {
		group := buckets[c]
		for i := 0; i < len(group); i += 2 {
			if i+1 < len(group) {
				lines = append(lines, scAttr(group[i].Abbrev, group[i].Value, group[i+1].Abbrev, group[i+1].Value))
			} else {
				lines = append(lines, scAttrOne(group[i].Abbrev, group[i].Value))
			}
		}
	}
	return lines
}

// scEquipCell formats one worn-equipment cell for the score sheet: the
// subtle short slot name padded to a fixed column, then the (already
// color-wrapped) item name.
func scEquipCell(r equipRow) string {
	if r.Slot == "" && r.Name == "" {
		return ""
	}
	pad := max(9-len(r.Slot), 1)
	return scSub(r.Slot) + strings.Repeat(" ", pad) + r.Name
}

// scTier wraps text in a good/warning/danger tag chosen by value/max
// ratio; fullTag names the healthy-tier color (e.g. "hp" or "good").
func scTier(value, max int, fullTag, text string) string {
	tag := fullTag
	if max > 0 {
		switch r := float64(value) / float64(max); {
		case r <= 0.25:
			tag = "danger"
		case r <= 0.5:
			tag = "warning"
		}
	}
	return "<" + tag + ">" + text + "</" + tag + ">"
}

func scHi(s string) string  { return "<highlight>" + s + "</highlight>" }
func scSub(s string) string { return "<subtle>" + s + "</subtle>" }

// tenths renders an integer count of tenths as a one-decimal string (32 → "3.2",
// 60 → "6.0"), the display form for Shadowrun Essence (SR-M4). Negatives are
// clamped to 0 — Essence never floors below zero.
func tenths(v int) string {
	if v < 0 {
		v = 0
	}
	return fmt.Sprintf("%d.%d", v/10, v%10)
}

// madnessBand coarsens a saidin-taint score into an ominous qualitative band for
// the score sheet (WoT S2 Phase 4+). The number is deliberately hidden behind
// dread-flavored words — the curse should read as creeping unease, not a meter
// to min-max. The bands roughly track the manifestation thresholds (the
// mechanical bite escalates as the words darken). Callers gate on Madness > 0,
// so the "untouched" band never shows.
func madnessBand(m int) string {
	switch {
	case m >= 75:
		return "the madness has you"
	case m >= 50:
		return "voices clamor"
	case m >= 25:
		return "a shadow on your mind"
	default:
		return "a faint whisper"
	}
}

// channelingGiftLabel maps a stored channeling gift ("spark"/"learn"/"none")
// to the phrase shown on the score sheet. An empty or unrecognized gift maps
// to "" so the row is omitted (non-WoT characters, pre-v28 saves).
func channelingGiftLabel(gift string) string {
	switch strings.ToLower(strings.TrimSpace(gift)) {
	case "spark":
		return "born with the spark"
	case "learn":
		return "able to learn"
	case "none":
		return "cannot channel"
	default:
		return ""
	}
}

// commafy formats an integer with thousands separators ("1250" → "1,250").
func commafy(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// titleCase upper-cases the first rune of s (for race/class ids and the
// sustenance tier word). Leaves the rest untouched — these are single
// lowercase tokens like "human" / "fighter" / "full".
func titleCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// nonEmpty returns the non-blank arguments in order — used to join an
// identity line ("Gender Race Class") without leaving stray spaces when a
// part is absent.
func nonEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}
