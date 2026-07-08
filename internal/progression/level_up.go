package progression

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// AbilityGranter teaches an ability to an entity. Returns the
// human-facing ability name and whether the grant succeeded. M9
// supplies a real proficiency-backed implementation; M8.4 wires a
// log-only stub so the path processor's plumbing is testable
// today.
//
// A miss (false) means the ability id is unknown; the caller logs
// and skips per spec §4.5 step 4.
type AbilityGranter interface {
	Teach(ctx context.Context, entityID, abilityID string) (name string, ok bool)
}

// Notifier surfaces a player-visible message via whatever side
// channel the host supplies. M8.4 uses it for the "You have
// learned <ability>!" notification (spec §4.5 step 4 second
// bullet). Production wires through session.Manager + actor.Write;
// tests use a fake.
type Notifier interface {
	Notify(ctx context.Context, entityID, msg string)
}

// TrainsCrediter is the host-side seam for crediting trains. M8.4
// stores the count on the player save; M8.6 (training) will read
// it back to gate `train` verb usage.
type TrainsCrediter interface {
	CreditTrains(ctx context.Context, entityID string, n int)
}

// ClassPathProcessor applies a class's level-up grants. It owns no
// state of its own — every call resolves the class through the
// registry and consults the supplied granter + notifier.
//
// Spec §4.5: runs once per level-up step (and at character
// creation, treated as level 1). For each path entry whose Level
// matches AND whose UnlockedVia is empty, teach the ability and
// enqueue a player-visible notification. Unknown ability ids are
// logged and skipped without raising — quest unlocks may register
// later.
type ClassPathProcessor struct {
	Classes  *ClassRegistry
	Granter  AbilityGranter
	Notifier Notifier
}

// Apply runs the path-processor logic for entityID's class at the
// supplied level. trackName is the bound track that just leveled
// (case-insensitive); pass "" for character.created to treat as
// level 1 / no gate. classID is the entity's resolved class id;
// empty short-circuits.
//
// Caller is responsible for the §4.5 step 2 track gate when this
// is a level-up event: pass the event's track to trackName so
// Apply can compare it against the class's BoundTrack and
// short-circuit on mismatch.
//
// Returns nil on success. Errors today are reserved for future
// fatal misuse; M8.4 only logs the per-entry skips.
func (p *ClassPathProcessor) Apply(ctx context.Context, entityID, classID, trackName string, level int) {
	if p == nil || p.Classes == nil {
		return
	}
	if classID == "" {
		return
	}
	cls, ok := p.Classes.Get(classID)
	if !ok {
		return
	}
	if cls.BoundTrack == "" {
		return
	}
	// Track gate: only enforced when a track name is supplied.
	// character.created passes "" (treat as level 1, no gate per
	// spec §4.5 step 3).
	if trackName != "" && !strings.EqualFold(trackName, cls.BoundTrack) {
		return
	}
	log := logging.From(ctx)
	for _, entry := range cls.Path {
		if entry.Level != level {
			continue
		}
		if strings.TrimSpace(entry.UnlockedVia) != "" {
			// Owned by another subsystem (quest, scripted hook).
			continue
		}
		abilityID := strings.TrimSpace(entry.AbilityID)
		if abilityID == "" {
			continue
		}
		var name string
		var taught bool
		if p.Granter != nil {
			name, taught = p.Granter.Teach(ctx, entityID, abilityID)
		}
		if !taught {
			log.Warn("class path: unknown ability; skipping",
				slog.String("event", "progression.class_path.unknown_ability"),
				slog.String("entity_id", entityID),
				slog.String("class", cls.ID),
				slog.String("ability", abilityID),
				slog.Int("level", level))
			continue
		}
		if p.Notifier != nil {
			display := name
			if display == "" {
				display = abilityID
			}
			p.Notifier.Notify(ctx, entityID, "You have learned "+display+"!")
		}
	}
}

// ApplyStatGrowth implements the spec §4.6 stat-growth handler. For
// every (stat → dice) in class.StatGrowth, rolls the dice using r,
// adds the §4.1 growth-bonus if GrowthBonuses declares a source
// stat for the same key, applies the delta to base via
// AdjustBase, and credits TrainsPerLevel via the crediter.
//
// **Track-gating choice:** spec §4.6 step 2 specifies "no track
// gate" — the handler runs on every level-up regardless of which
// track triggered it. The ROADMAP M8.4 acceptance criterion calls
// this out as a deliberate decision; we honor it here. Callers
// gate elsewhere if they want different behavior.
//
// classID/trackName are accepted for symmetry with
// ClassPathProcessor.Apply and so future track-gating policy can
// land without callers changing.
//
// Returns the total delta added to each stat (post-bonus). Useful
// for tests + future renderers ("you gained +6 hp"). Empty class
// or empty growth map returns nil.
func ApplyStatGrowth(
	ctx context.Context,
	cls *Class,
	sb *StatBlock,
	r combat.Roller,
	crediter TrainsCrediter,
	entityID string,
) map[StatType]int {
	if cls == nil || sb == nil {
		return nil
	}
	if len(cls.StatGrowth) == 0 && cls.TrainsPerLevel == 0 {
		return nil
	}
	out := make(map[StatType]int, len(cls.StatGrowth))
	for stat, dice := range cls.StatGrowth {
		roll := 0
		if !dice.IsZero() {
			roll = dice.Roll(r)
		}
		if src, has := cls.GrowthBonuses[stat]; has && src != "" {
			eff := sb.Effective(src)
			bonus := max((eff-10)/2, 0)
			roll += bonus
		}
		if roll != 0 {
			sb.AdjustBase(stat, roll)
		}
		out[stat] = roll
	}
	if cls.TrainsPerLevel > 0 && crediter != nil {
		crediter.CreditTrains(ctx, entityID, cls.TrainsPerLevel)
	}
	return out
}

// nopGranter is a granter that always misses. Used by host wiring
// that has no ability registry yet (M9 absent) so the path
// processor logs an unknown-ability warning per entry — exactly
// the behavior the M8.4 acceptance criterion describes ("path
// processor logs and skips unknown ability ids without raising").
type nopGranter struct{}

// NewNopGranter returns the no-op granter used by hosts that have
// no ability registry wired yet.
func NewNopGranter() AbilityGranter { return nopGranter{} }

func (nopGranter) Teach(context.Context, string, string) (string, bool) {
	return "", false
}
