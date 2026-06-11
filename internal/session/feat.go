package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/srckey"
)

// featStats is the ability set a feat prerequisite can gate on (EPIC S4
// Phase 4). Captured into the CharacterView snapshot so feat.Eligible reads a
// consistent picture without holding a.mu.
var featStats = []progression.StatType{
	progression.StatSTR, progression.StatINT, progression.StatWIS,
	progression.StatDEX, progression.StatCON, progression.StatLUCK,
}

// featCharView is a captured feat.CharacterView snapshot (EPIC S4 Phase 4). It
// is built from the actor's public accessors (each self-locked) so the prereq
// evaluator never re-enters a.mu. Command dispatch is serial per session, so
// the multi-step gather sees a consistent character.
type featCharView struct {
	scores   map[string]int
	known    map[string]bool
	level    int
	classes  []string
	prof     *progression.ProficiencyManager
	entityID string
}

func (v featCharView) AbilityScore(stat string) int {
	return v.scores[strings.ToLower(strings.TrimSpace(stat))]
}

func (v featCharView) SkillProficiency(abilityID string) int {
	if v.prof == nil {
		return 0
	}
	p, _ := v.prof.Proficiency(v.entityID, abilityID)
	return p
}

func (v featCharView) HasFeat(featID string) bool {
	return v.known[strings.ToLower(strings.TrimSpace(featID))]
}

func (v featCharView) CharacterLevel() int { return v.level }
func (v featCharView) ClassIDs() []string  { return v.classes }

// featCharView builds the eligibility snapshot. Called outside a.mu.
func (a *connActor) buildFeatCharView() featCharView {
	entityID := a.PlayerID()
	if entityID == "" {
		entityID = a.ID()
	}
	scores := make(map[string]int, len(featStats))
	for _, s := range featStats {
		scores[string(s)] = a.StatValue(s)
	}
	known := make(map[string]bool)
	a.mu.Lock()
	prof := a.prof
	if a.save != nil {
		for _, kf := range a.save.KnownFeats {
			// Normalize at insertion so HasFeat (which lowercases its arg)
			// matches even a hand-edited save with a mixed-case feat id.
			known[strings.ToLower(strings.TrimSpace(kf.FeatID))] = true
		}
	}
	a.mu.Unlock()
	return featCharView{
		scores:   scores,
		known:    known,
		level:    a.characterLevel(),
		classes:  a.ClassIDs(),
		prof:     prof,
		entityID: entityID,
	}
}

// characterLevel sums the actor's bound-track levels (one class today; the
// multiclass total is the sum). A classless / level-1 character is level 1.
func (a *connActor) characterLevel() int {
	total := 0
	for _, cid := range a.ClassIDs() {
		a.mu.Lock()
		bound := ""
		if a.classes != nil {
			if cls, ok := a.classes.Get(cid); ok {
				bound = cls.BoundTrack
			}
		}
		a.mu.Unlock()
		if bound != "" {
			total += a.Level(bound)
		}
	}
	if total < 1 {
		total = 1
	}
	return total
}

// FeatCredits is declared on connActor in session.go (Phase 2). KnownFeats and
// the take/list verbs below complete the EPIC S4 Phase 4 feat-actor surface.

// TakeFeat spends one banked feat slot to take feat featID (EPIC S4 Phase 4 —
// docs/proposals/wot-feats.md §2.3). param binds a per-parameter feat (a
// weapon/skill); it is ignored for single/stackable feats. Returns (true,
// confirmation) on success, or (false, reason) when the feat is unknown, the
// rule forbids it (already taken / missing param), prerequisites or the class
// gate fail, or no slot is banked. The credit decrement + known-feats record
// happen together under a.mu (the spend side SpendTrain-style, not a negative
// CreditFeats).
func (a *connActor) TakeFeat(featID, param string) (bool, string) {
	a.mu.Lock()
	reg := a.feats
	a.mu.Unlock()
	if reg == nil {
		return false, "Feats are not available in this world."
	}
	f, ok := reg.Get(featID)
	if !ok {
		return false, fmt.Sprintf("There is no feat called %q.", strings.TrimSpace(featID))
	}
	param = strings.ToLower(strings.TrimSpace(param))

	// Rule check: already-taken / missing param, by multi-take rule. The
	// `already` flag is consulted only for single + per-param feats; a
	// stackable feat is always allowed (recordFeatLocked increments its count).
	a.mu.Lock()
	already := a.featTakenLocked(f, param)
	a.mu.Unlock()
	switch f.MultiTake {
	case feat.MultiTakeParam:
		if param == "" {
			return false, fmt.Sprintf("%s must be taken for a specific target, e.g. `feat %s <target>`.", f.DisplayName, f.ID)
		}
		if already {
			return false, fmt.Sprintf("You already have %s (%s).", f.DisplayName, param)
		}
	case feat.MultiTakeStackable:
		// Always allowed; recordFeatLocked increments the count.
	default: // single
		if already {
			return false, fmt.Sprintf("You already have %s.", f.DisplayName)
		}
	}

	// Prerequisites + class gate.
	if el := feat.Eligible(f, a.buildFeatCharView()); !el.OK {
		return false, featIneligibleMsg(f, el)
	}

	// Spend + record atomically. Re-check the credit AND the already-taken
	// guard under the lock: a concurrent grant/spend is impossible under serial
	// dispatch today, but this keeps the invariants honest if a future bus event
	// (e.g. a quest feat reward) ever records a feat off the dispatch path.
	a.mu.Lock()
	if a.featCredits <= 0 {
		a.mu.Unlock()
		return false, "You have no feat slots to spend. (You earn one at creation and one every 3 levels.)"
	}
	if f.MultiTake != feat.MultiTakeStackable && a.featTakenLocked(f, param) {
		a.mu.Unlock()
		return false, fmt.Sprintf("You already have %s.", f.DisplayName)
	}
	a.featCredits--
	a.recordFeatLocked(f, param)
	if a.save != nil {
		a.save.FeatCredits = a.featCredits
	}
	a.markDirtyLocked()
	a.mu.Unlock()

	// Re-install the stat-shaped feat bonuses (e.g. a freshly-taken Toughness
	// raising max HP) now that known_feats changed (Phase 3b).
	a.applyFeatGrants()

	if f.MultiTake == feat.MultiTakeParam {
		return true, fmt.Sprintf("You gain the %s feat (%s).", f.DisplayName, param)
	}
	return true, fmt.Sprintf("You gain the %s feat.", f.DisplayName)
}

// GrantFeat records featID as a held feat WITHOUT spending a slot or checking
// prerequisites (EPIC S4 Phase 5): an authored grant from a background/class
// (backgrounds §2). param binds a per-parameter feat. A feat absent from the
// registry is skipped fail-soft. The grant is GRANT-ONCE — idempotent for ALL
// multi-take modes (including stackable): if the feat is already held it is a
// no-op. So the character.created subscriber re-firing, or a relog re-grant,
// never duplicates or inflates a stack. (A deliberate "add another stack"
// would be a separate, explicit API — an authored background grant means "you
// have this feat", granted once.) The conferred stat bonuses are reinstalled
// after recording.
func (a *connActor) GrantFeat(featID, param string) {
	a.mu.Lock()
	reg := a.feats
	a.mu.Unlock()
	if reg == nil {
		return
	}
	f, ok := reg.Get(featID)
	if !ok {
		return
	}
	param = strings.ToLower(strings.TrimSpace(param))
	a.mu.Lock()
	if a.featTakenLocked(f, param) {
		a.mu.Unlock()
		return
	}
	a.recordFeatLocked(f, param)
	a.markDirtyLocked()
	a.mu.Unlock()
	a.applyFeatGrants()
}

// featWeaponBonuses is the per-weapon-category feat bonus cache (EPIC S4
// Phase 3c), recomputed on every feat change and read LOCK-FREE in
// connActor.Stats() (the combat hot path, which deliberately avoids a.mu).
// Mirrors the a.weapon atomic-pointer pattern. Nil maps read as zero.
type featWeaponBonuses struct {
	hit  map[string]int // weapon category → to-hit bonus (Weapon Focus)
	crit map[string]int // weapon category → threat-low widen (Improved Critical)
}

// applyFeatGrants recomputes ALL feat grants from the actor's known_feats and
// installs them (EPIC S4 Phase 3b/3c). Idempotent — safe to call at load and
// after every take/grant:
//   - hp_max stat modifier under srckey.Feat (AddModifiers replaces per source;
//     the OnMaxChange→vitals binding then moves the ceiling). 3b.
//   - the per-weapon-category hit/crit cache, stored in an atomic pointer Stats
//     reads lock-free. 3c.
//   - ability grants (Power Attack) taught via prof.Learn (idempotent; the
//     single-grant guard never resets a practiced ability). 3c.
func (a *connActor) applyFeatGrants() {
	a.mu.Lock()
	reg := a.feats
	sb := a.statBlock
	prof := a.prof
	entityID := a.playerID
	var held []feat.Taken
	if a.save != nil {
		for _, kf := range a.save.KnownFeats {
			held = append(held, feat.Taken{FeatID: kf.FeatID, Param: kf.Param, Count: kf.Count})
		}
	}
	a.mu.Unlock()

	b := feat.ComputeBonuses(held, reg) // reg nil → zero bonuses

	// 3b: hp_max stat modifier.
	if sb != nil {
		if b.MaxHP != 0 {
			sb.AddModifier(srckey.Feat("hp_max"), progression.StatHPMax, b.MaxHP)
		} else {
			// Remove any stale feat hp_max modifier (empty list removes it).
			sb.AddModifiers(srckey.Feat("hp_max"), nil)
		}
	}

	// 3c: per-weapon-category hit/crit cache (read lock-free in Stats).
	a.featWeaponBonus.Store(&featWeaponBonuses{hit: b.HitByCategory, crit: b.CritByCategory})

	// 3c: ability grants (Power Attack). Teach at baseline ONLY if not already
	// known — prof.Learn overwrites the proficiency value, so re-Learning on
	// every applyFeatGrants (login + each feat change) would reset a practiced
	// granted ability back to 1. The existence check makes the grant idempotent.
	if prof != nil && entityID != "" {
		for _, abID := range b.Abilities {
			if _, known := prof.Proficiency(entityID, abID); known {
				continue
			}
			prof.Learn(entityID, abID, 1)
		}
	}
}

// FeatSkillBonus returns the additive feat check bonus for skill ability
// skillID (Skill Emphasis — EPIC S4 Phase 3c). Recomputed on demand (skill
// checks are cold). 0 when no feat emphasizes that skill.
func (a *connActor) FeatSkillBonus(skillID string) int {
	a.mu.Lock()
	reg := a.feats
	var held []feat.Taken
	if a.save != nil {
		for _, kf := range a.save.KnownFeats {
			held = append(held, feat.Taken{FeatID: kf.FeatID, Param: kf.Param, Count: kf.Count})
		}
	}
	a.mu.Unlock()
	if reg == nil {
		return 0
	}
	return feat.ComputeBonuses(held, reg).SkillByID[strings.ToLower(strings.TrimSpace(skillID))]
}

// featTakenLocked reports whether the actor already holds f (caller holds
// a.mu). For a per-parameter feat, "taken" means the same param; for a single
// or stackable feat, any instance counts.
func (a *connActor) featTakenLocked(f *feat.Feat, param string) bool {
	if a.save == nil {
		return false
	}
	for _, kf := range a.save.KnownFeats {
		if kf.FeatID != f.ID {
			continue
		}
		if f.MultiTake == feat.MultiTakeParam {
			if kf.Param == param {
				return true
			}
			continue
		}
		return true
	}
	return false
}

// recordFeatLocked writes the taken feat into known_feats (caller holds a.mu).
// A stackable feat increments an existing entry's count (or seeds it at 1); a
// single/per-parameter feat appends a new entry.
func (a *connActor) recordFeatLocked(f *feat.Feat, param string) {
	if a.save == nil {
		return
	}
	if f.MultiTake == feat.MultiTakeStackable {
		for i := range a.save.KnownFeats {
			if a.save.KnownFeats[i].FeatID == f.ID {
				c := a.save.KnownFeats[i].Count
				if c < 1 {
					c = 1
				}
				a.save.KnownFeats[i].Count = c + 1
				return
			}
		}
		a.save.KnownFeats = append(a.save.KnownFeats, player.KnownFeat{FeatID: f.ID, Count: 1})
		return
	}
	a.save.KnownFeats = append(a.save.KnownFeats, player.KnownFeat{FeatID: f.ID, Param: param})
}

// FeatListing renders the `feats` verb output (EPIC S4 Phase 4): the feats the
// character holds, the banked slot count, and (when slots remain) the feats
// they are currently eligible to take.
func (a *connActor) FeatListing() string {
	a.mu.Lock()
	reg := a.feats
	credits := a.featCredits
	var known []player.KnownFeat
	if a.save != nil {
		known = append(known, a.save.KnownFeats...)
	}
	a.mu.Unlock()

	var b strings.Builder
	b.WriteString("<title>Feats</title>\n")
	if len(known) == 0 {
		b.WriteString("  <subtle>(none taken)</subtle>\n")
	} else {
		sort.Slice(known, func(i, j int) bool { return known[i].FeatID < known[j].FeatID })
		for _, kf := range known {
			name := kf.FeatID
			if reg != nil {
				if f, ok := reg.Get(kf.FeatID); ok {
					name = f.DisplayName
				}
			}
			line := "  " + name
			if kf.Param != "" {
				line += " (" + kf.Param + ")"
			}
			if kf.Count > 1 {
				line += fmt.Sprintf(" x%d", kf.Count)
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString(fmt.Sprintf("Feat slots available: <stat>%d</stat>\n", credits))
	if reg != nil && credits > 0 {
		view := a.buildFeatCharView()
		var avail []string
		for _, f := range reg.All() {
			// Single feats already held drop off; per-param/stackable can repeat.
			if f.MultiTake == feat.MultiTakeSingle && view.HasFeat(f.ID) {
				continue
			}
			if feat.Eligible(f, view).OK {
				avail = append(avail, f.DisplayName)
			}
		}
		if len(avail) > 0 {
			b.WriteString("Available: " + strings.Join(avail, ", "))
		} else {
			b.WriteString("<subtle>No feats are available to take right now.</subtle>")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// featIneligibleMsg turns a failed Eligibility into a player-facing reason.
func featIneligibleMsg(f *feat.Feat, el feat.Eligibility) string {
	var parts []string
	if el.ClassExcluded {
		parts = append(parts, "your class cannot take it")
	}
	for _, p := range el.UnmetPrereqs {
		parts = append(parts, prereqPhrase(p))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("You cannot take %s yet.", f.DisplayName)
	}
	return fmt.Sprintf("You cannot take %s: %s.", f.DisplayName, strings.Join(parts, "; "))
}

// prereqPhrase renders one unmet prerequisite.
func prereqPhrase(p feat.Prerequisite) string {
	switch p.Kind {
	case feat.PrereqAbilityScore:
		return fmt.Sprintf("requires %s %d+", strings.ToUpper(p.Target), p.Min)
	case feat.PrereqSkill:
		return fmt.Sprintf("requires %s proficiency %d+", p.Target, p.Min)
	case feat.PrereqFeat:
		return fmt.Sprintf("requires the %s feat", p.Target)
	case feat.PrereqLevel:
		return fmt.Sprintf("requires level %d", p.Min)
	default:
		return "has an unmet requirement"
	}
}
