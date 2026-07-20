package session

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// ClassPathApplier applies a class's level-N path grants to a character
// (satisfied by *progression.ClassPathProcessor). Declared here, at the point of
// use, so the session package depends on the behavior, not the processor's
// concrete construction in the composition root.
type ClassPathApplier interface {
	Apply(ctx context.Context, entityID, classID, trackName string, level int)
}

// CreationGrants bundles the collaborators the one-time character.created grant
// pass needs: the content registries it reads, the class-path applier, and the
// background/item granter. Every field is optional — a nil registry/applier/
// granter simply skips its step, which keeps ApplyCharacterCreated unit-testable
// with only the pieces a given case exercises.
type CreationGrants struct {
	Races       *progression.RaceRegistry
	Classes     *progression.ClassRegistry
	Backgrounds *progression.BackgroundRegistry
	ClassPath   ClassPathApplier
	Granter     *BackgroundGranter
}

// ApplyCharacterCreated runs the one-time creation-grant sequence for a freshly
// created character, resolved by player id. Extracted from the composition root
// so its step ORDER — which is load-bearing — is unit-testable.
//
// Order (each step may raise a max via OnMaxChange→SetMax, which lifts the
// ceiling but never auto-heals current):
//  1. Metatype attribute skew (race StatBonuses — an ork/troll's larger Physical
//     monitor). Applied via ApplyStartingStats (additive AdjustBase, persisted).
//  2. Per class: level-1 Path grants, the class's StartingStats endowment (a
//     channeler's resource_max One Power pool), and the role "floor" kit.
//  3. The chosen background package (skills/items/gold + the pick-one feat, which
//     may be a Toughness-style max_hp grant).
//  4. The base feat slot (1 credit at creation).
//  5. FILL TO FULL — resource pools + HP — LAST, after every max-raising grant
//     above. This ordering is the fix for the 20/23 bug: a Toughness-granting
//     origin (step 3) raises hp_max AFTER the metatype skew (step 1), so filling
//     before the background grant would strand current below the new ceiling. No
//     max-raising grant ⇒ Heal/Fill cap at max ⇒ a no-op.
//  6. The first-entry commlink onboarding call (the background grant put a
//     commlink in inventory; the enter-world path ran before the grants, when a
//     new character's inventory was still empty). Shown-once, so this is the only
//     delivery.
//
// No-ops if the actor is offline. Never re-fires on relogin — the caller
// publishes character.created only for a freshly committed character, and
// RestoreBase carries the persisted base on subsequent logins.
func (m *Manager) ApplyCharacterCreated(ctx context.Context, playerID string, g CreationGrants) {
	if m == nil {
		return
	}
	a, ok := m.GetByPlayerID(playerID)
	if !ok || a == nil {
		return
	}

	// 1. Metatype starting-attribute skew. ApplyStartingStats no-ops an empty map,
	// so a metatype with no skew (human) is a clean pass-through.
	if rid := a.RaceID(); rid != "" && g.Races != nil {
		if race, ok := g.Races.Get(rid); ok {
			a.ApplyStartingStats(race.StatBonuses)
		}
	}

	// 2. Per-class level-1 features. Walk the live class list so a multiclass
	// character (wot-character-model D1 seam) gets all its starting features.
	for _, classID := range a.ClassIDs() {
		if g.ClassPath != nil {
			// character-created is treated as level 1 with no track gate (spec
			// §4.5 step 3); empty trackName short-circuits the gate check.
			g.ClassPath.Apply(ctx, playerID, classID, "", 1)
		}
		if g.Classes != nil {
			if cls, ok := g.Classes.Get(classID); ok {
				a.ApplyStartingStats(cls.StartingStats)
				if g.Granter != nil {
					g.Granter.GrantStartingItems(playerID, cls.StartingItems)
				}
			}
		}
	}

	// 3. The chosen background package (backgrounds §4), applying the pick-one
	// chooser selections (feat + equipment package) persisted at creation.
	if bgID := a.BackgroundID(); bgID != "" && g.Backgrounds != nil && g.Granter != nil {
		if bg, ok := g.Backgrounds.Get(bgID); ok {
			feat, equip := a.BackgroundChoices()
			g.Granter.Grant(ctx, playerID, bg, BackgroundChoices{Feat: feat, EquipmentIndex: equip})
		}
	}

	// 4. The base feat slot granted at creation (feats §2.2).
	a.CreditFeats(1)

	// 5. Fill to full — LAST. See the doc comment: this must follow the background
	// feat grant so a max_hp feat doesn't leave the character below its ceiling.
	a.FillResourcePools()
	a.FillVitals()

	// 6. First-entry commlink call (no-op when the feature is unconfigured).
	m.DeliverCommlinkCallFor(ctx, playerID)
}
