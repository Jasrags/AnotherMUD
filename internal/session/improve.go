package session

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// improve.go — the karma-ledger spend verb (shadowrun-mvp.md SR-M5b). `improve
// <target>` spends banked karma to raise one of three things, à la carte:
//
//   - an ATTRIBUTE (a trainable primary): +1 to the base, cost = new-value ×
//     AttributeMult, gated by the metatype's stat cap;
//   - a SKILL (an ability with category:skill): its trainer-cap rises one tier
//     (Novice→Apprentice→Journeyman→Master), cost = new-tier-rank × SkillMult;
//   - a QUALITY (a feat carrying karma_cost > 0): granted for its flat cost.
//
// Only a karma-ledger character (non-nil ledger) can improve; a level-track
// character advances by leveling and never reaches this path. All spends are
// atomic: SpendKarma is the gate, and the raise only applies after it succeeds.

// Improve resolves target and, on a match, spends karma to raise it. Returns the
// player-facing result line. An empty target renders the listing. Resolution
// order is attribute → skill → quality; the first kind that OWNS the target
// name handles it (even if the spend then fails for insufficient karma), so an
// unaffordable buy reports its price rather than falling through to "unknown".
func (a *connActor) Improve(ctx context.Context, target, param string) string {
	if !a.UsesKarmaLedger() {
		return "You do not advance by spending karma."
	}
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return a.ImproveListing()
	}
	if msg, handled := a.improveAttribute(target); handled {
		return msg
	}
	if msg, handled := a.improveSkill(target); handled {
		return msg
	}
	if msg, handled := a.improveQuality(ctx, target, param); handled {
		return msg
	}
	return fmt.Sprintf("You don't know how to improve %q. Type `improve` to see what you can raise.", target)
}

// improveAttribute raises a trainable primary attribute by 1. handled=false when
// target is not a trainable attribute (so Improve falls through to skills).
func (a *connActor) improveAttribute(target string) (string, bool) {
	if a.attrSet == nil || a.statBlock == nil {
		return "", false
	}
	attr, ok := a.attrSet.Get(progression.StatType(target))
	if !ok || !attr.Trainable {
		return "", false
	}
	stat := attr.ID
	name := attr.Name
	if name == "" {
		name = string(stat)
	}
	// Gate on the BASE rating (the permanent value karma buys), not Effective —
	// a temporary gear/effect buff must not block spending real base points, and
	// the metatype cap in SR bounds the natural rating.
	current := a.statBlock.Base(stat)
	cap := a.attributeCap(stat, attr.Cap)
	if cap > 0 && current >= cap {
		return fmt.Sprintf("Your %s is already at your metatype maximum (%d).", name, cap), true
	}
	newVal := current + 1
	cost := int64(newVal) * a.karmaCosts.AttributeMult
	if !a.SpendKarma(cost) {
		cur, _ := a.KarmaBalance()
		return fmt.Sprintf("Raising %s to %d costs %d karma; you have %d.", name, newVal, cost, cur), true
	}
	a.statBlock.AdjustBase(stat, 1) // channels/pools re-derive via the pull/OnMaxChange seams
	a.mu.Lock()
	a.syncStatsToSaveLocked()
	a.markDirtyLocked()
	a.mu.Unlock()
	cur, _ := a.KarmaBalance()
	return fmt.Sprintf("You spend %d karma. Your %s rises to %d. (%d karma left.)", cost, name, newVal, cur), true
}

// attributeCap resolves the ceiling for a stat: the metatype (race) stat_cap
// wins; else the attribute-set's declared Cap; else 0 (no cap). Mirrors the
// train verb's cap resolution (training.go), minus the config default — an
// SR metatype always declares caps, so a 0 here means genuinely uncapped.
func (a *connActor) attributeCap(stat progression.StatType, setCap int) int {
	if a.races != nil && a.raceID != "" {
		if r, ok := a.races.Get(a.raceID); ok {
			if v, has := r.StatCaps[stat]; has {
				return v
			}
		}
	}
	return setCap
}

// improveSkill raises a skill ability's trainer-cap by one tier. handled=false
// when target is not a skill ability (so Improve falls through to qualities).
func (a *connActor) improveSkill(target string) (string, bool) {
	if a.abilities == nil || a.prof == nil {
		return "", false
	}
	ab, ok := a.abilities.Get(target)
	if !ok || ab.Category != progression.AbilitySkill {
		return "", false
	}
	id := strings.ToLower(strings.TrimSpace(ab.ID))
	_, _, learned := a.prof.GetCap(a.playerID, id)
	current := 0
	if learned {
		current = a.prof.Cap(a.playerID, id)
	}
	next := progression.NextTier(current)
	if next == progression.CapNone {
		return fmt.Sprintf("Your %s is already at the master ceiling.", ab.DisplayName), true
	}
	rank := int64(next) / int64(progression.CapNovice) // 1..4
	cost := rank * a.karmaCosts.SkillMult
	if !a.SpendKarma(cost) {
		cur, _ := a.KarmaBalance()
		return fmt.Sprintf("Raising %s to %s costs %d karma; you have %d.", ab.DisplayName, tierName(next), cost, cur), true
	}
	if !learned {
		a.prof.Learn(a.playerID, id, 1) // establish the skill (proficiency 1) so it is usable
	}
	a.prof.SetCap(a.playerID, id, int(next))
	// No markDirtyLocked here (unlike the attribute path): the cap/learn write
	// lives in the ProficiencyManager, which Persist pulls in via
	// syncAbilitiesToSaveLocked on the next autosave/logout — the same
	// pull-based persistence vitals/pools use. Adding a dirty flag would be
	// redundant, not a fix.
	cur, _ := a.KarmaBalance()
	return fmt.Sprintf("You spend %d karma. Your %s ceiling rises to %s. (%d karma left.)", cost, ab.DisplayName, tierName(next), cur), true
}

// improveQuality grants a karma-buyable feat (KarmaCost > 0). handled=false when
// target is not such a feat (so Improve falls through to "unknown"). Mirrors
// TakeFeat's validation (already-held, per-param, prerequisites) but spends
// karma instead of a feat slot.
func (a *connActor) improveQuality(ctx context.Context, target, param string) (string, bool) {
	if a.feats == nil {
		return "", false
	}
	f, ok := a.feats.Get(target)
	if !ok || f.KarmaCost <= 0 {
		return "", false
	}
	param = strings.ToLower(strings.TrimSpace(param))
	if f.MultiTake == feat.MultiTakeParam && param == "" {
		return fmt.Sprintf("%s must be improved for a specific target, e.g. `improve %s <target>`.", f.DisplayName, f.ID), true
	}
	if el := feat.Eligible(f, a.buildFeatCharView()); !el.OK {
		return featIneligibleMsg(f, el), true
	}
	cost := int64(f.KarmaCost)
	// Re-check held, spend, and record ATOMICALLY under a.mu (mirrors TakeFeat's
	// spend block): doing the already-taken guard and the karma spend in one lock
	// hold makes it impossible to charge for a no-op grant if a concurrent path
	// (a future async feat reward) grants the same feat in a race. The ledger
	// lock nests under a.mu — the same a.mu -> ledger.mu order syncKarmaToSaveLocked
	// uses, so there is no cycle. recordFeatLocked (not GrantFeat) does the write,
	// since GrantFeat would re-acquire a.mu.
	a.mu.Lock()
	if f.MultiTake != feat.MultiTakeStackable && a.featTakenLocked(f, param) {
		a.mu.Unlock()
		if f.MultiTake == feat.MultiTakeParam {
			return fmt.Sprintf("You already have %s (%s).", f.DisplayName, param), true
		}
		return fmt.Sprintf("You already have %s.", f.DisplayName), true
	}
	if !a.karma.Spend(cost) {
		a.mu.Unlock()
		return fmt.Sprintf("%s costs %d karma; you have %d.", f.DisplayName, cost, a.karma.Current()), true
	}
	a.recordFeatLocked(f, param)
	a.markDirtyLocked()
	a.mu.Unlock()
	a.applyFeatGrants() // reinstall the conferred bonuses (outside the lock, like TakeFeat)
	cur := a.karma.Current()
	if f.MultiTake == feat.MultiTakeParam {
		return fmt.Sprintf("You spend %d karma and gain %s (%s). (%d karma left.)", cost, f.DisplayName, param, cur), true
	}
	return fmt.Sprintf("You spend %d karma and gain %s. (%d karma left.)", cost, f.DisplayName, cur), true
}

// ImproveListing renders what the character can currently raise with karma and
// what each costs, plus the balance — the no-argument `improve` view.
func (a *connActor) ImproveListing() string {
	if !a.UsesKarmaLedger() {
		return "You do not advance by spending karma."
	}
	cur, total := a.KarmaBalance()
	var b strings.Builder
	fmt.Fprintf(&b, "Karma: %d spendable (%d earned).\r\n", cur, total)

	// Attributes — the trainable primaries, with the +1 price and cap state.
	if a.attrSet != nil && a.statBlock != nil {
		var lines []string
		for _, attr := range a.attrSet.Attributes {
			if !attr.Trainable {
				continue
			}
			name := attr.Name
			if name == "" {
				name = string(attr.ID)
			}
			base := a.statBlock.Base(attr.ID)
			cap := a.attributeCap(attr.ID, attr.Cap)
			if cap > 0 && base >= cap {
				lines = append(lines, fmt.Sprintf("  %-12s %d (metatype max)", name, base))
				continue
			}
			cost := int64(base+1) * a.karmaCosts.AttributeMult
			lines = append(lines, fmt.Sprintf("  %-12s %d -> %d for %d karma", name, base, base+1, cost))
		}
		if len(lines) > 0 {
			b.WriteString("\r\nAttributes (improve <name>):\r\n")
			b.WriteString(strings.Join(lines, "\r\n"))
			b.WriteString("\r\n")
		}
	}

	// Skills — the abilities the character has learned, with the next-tier price.
	if a.abilities != nil && a.prof != nil {
		var lines []string
		for _, ab := range a.abilities.All() {
			if ab.Category != progression.AbilitySkill {
				continue
			}
			id := strings.ToLower(strings.TrimSpace(ab.ID))
			_, _, learned := a.prof.GetCap(a.playerID, id)
			if !learned {
				continue // known skills only; use one to learn it, then raise the ceiling
			}
			current := a.prof.Cap(a.playerID, id)
			next := progression.NextTier(current)
			if next == progression.CapNone {
				lines = append(lines, fmt.Sprintf("  %-14s %s (mastered)", ab.DisplayName, tierName(progression.CapMaster)))
				continue
			}
			rank := int64(next) / int64(progression.CapNovice)
			lines = append(lines, fmt.Sprintf("  %-14s -> %s for %d karma", ab.DisplayName, tierName(next), rank*a.karmaCosts.SkillMult))
		}
		if len(lines) > 0 {
			sort.Strings(lines)
			b.WriteString("\r\nSkills (improve <name>):\r\n")
			b.WriteString(strings.Join(lines, "\r\n"))
			b.WriteString("\r\n")
		}
	}

	// Qualities — karma-buyable feats the character does not yet hold.
	if a.feats != nil {
		var lines []string
		for _, f := range a.feats.All() {
			if f.KarmaCost <= 0 {
				continue
			}
			a.mu.Lock()
			held := a.featTakenLocked(f, "")
			a.mu.Unlock()
			if held && f.MultiTake != feat.MultiTakeStackable && f.MultiTake != feat.MultiTakeParam {
				continue
			}
			lines = append(lines, fmt.Sprintf("  %-16s %d karma", f.DisplayName, f.KarmaCost))
		}
		if len(lines) > 0 {
			sort.Strings(lines)
			b.WriteString("\r\nQualities (improve <name>):\r\n")
			b.WriteString(strings.Join(lines, "\r\n"))
			b.WriteString("\r\n")
		}
	}
	return strings.TrimRight(b.String(), "\r\n")
}

// tierName maps a cap value to its trainer-ladder tier label for display.
func tierName(cap progression.CapTier) string {
	switch cap {
	case progression.CapNovice:
		return "Novice"
	case progression.CapApprentice:
		return "Apprentice"
	case progression.CapJourneyman:
		return "Journeyman"
	case progression.CapMaster:
		return "Master"
	default:
		return fmt.Sprintf("cap %d", int(cap))
	}
}
