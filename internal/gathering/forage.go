package gathering

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// Gatherer is the player surface the forage operation reads/writes — its
// progression id, its carried inventory (the yield is filed here), and its
// transient forage cooldown. The command layer's actor satisfies it
// structurally (no adapter). Mirrors crafting.Crafter.
type Gatherer interface {
	PlayerID() string
	ID() string
	// Inventory returns the carried item ids — scanned by harvest for the
	// node's required tool (§3.3).
	Inventory() []entities.EntityID
	AddToInventory(entities.EntityID)
	// ForageReadyAt / SetForageReadyAt hold the per-character forage
	// cooldown as an engine tick (§5). Transient (never persisted), like
	// the crafting busy-state and rest machine.
	ForageReadyAt() uint64
	SetForageReadyAt(tick uint64)
}

// ForageOutcome classifies a forage attempt for the caller to render.
type ForageOutcome int

const (
	ForageOK ForageOutcome = iota
	// ForageCoolingDown — the per-character cooldown has not elapsed (§5).
	// This is a WAIT, never a refusal (§1.1) — availability is rate-limited.
	ForageCoolingDown
	// ForageEmptyTable — the table has no selectable entry (a content bug;
	// DecodeForageTable rejects this, so it should not occur at runtime).
	ForageEmptyTable
	// ForageOutputUndefined — the picked entry's item template is missing
	// from the world (content bug). The cooldown is NOT started.
	ForageOutputUndefined
	// ForageFailed — the item could not be spawned (store error). Cooldown
	// NOT started; no yield.
	ForageFailed
)

// propRarity is the reserved item-instance property the decoration system
// reads to render an item's tier (shared with crafting). A foraged yield is
// stamped with its rolled tier under this key.
const propRarity = "rarity"

// ForageResult is the structured outcome of Forage. On ForageOK the item
// has already been spawned into the gatherer's inventory; ItemID/ItemName/
// Qty/QualityKey describe it for the caller's message + the gathered event.
type ForageResult struct {
	Outcome        ForageOutcome
	ItemID         string // yielded item template id (qualified)
	ItemName       string // spawned item's display name
	Qty            int
	QualityKey     string // rolled rarity tier ("" = none)
	Gained         bool   // gathering proficiency rose this forage
	RemainingTicks uint64 // cooldown remaining on ForageCoolingDown
}

// Forage performs one ambient forage against table (gathering.md §2): it
// enforces the per-character cooldown (§5), rolls quality from the
// gatherer's gathering proficiency + the table richness clamped to the
// table ceiling (§4), and weighted-picks an entry. It does NOT create the
// item create step (§2): it spawns the picked yield into the gatherer's
// inventory (the Service owns the store, like crafting). On a successful
// yield it then starts the cooldown and rolls a use-based skill gain. now
// is the current engine tick. A missing template or spawn failure yields
// nothing and does NOT start the cooldown.
//
// Forage is never refused for lack of a tool (§1.1); a tool would only
// improve the roll (forage tools are a later refinement — the roll uses the
// baseline tool weight here).
func (s *Service) Forage(_ context.Context, g Gatherer, table *ForageTable, now uint64) ForageResult {
	if s == nil || table == nil {
		return ForageResult{Outcome: ForageEmptyTable}
	}
	if !table.hasSelectableEntry() {
		return ForageResult{Outcome: ForageEmptyTable}
	}

	// Cooldown gate (§5) — a wait, not a refusal.
	if ready := g.ForageReadyAt(); now < ready {
		return ForageResult{Outcome: ForageCoolingDown, RemainingTicks: ready - now}
	}

	eid := entityID(g)
	skill := 0
	if s.prof != nil {
		skill, _ = s.prof.Proficiency(eid, GatheringAbility)
	}

	// The table's content-declared ceiling key → ladder position (−1 when
	// absent/unknown → uncapped, the rollQuality sentinel).
	ceiling := ladderPosition(s.rarity, table.Ceiling)

	qualityKey := s.rollQuality(QualityInputs{
		Skill:         skill,
		Richness:      table.Richness,
		SourceCeiling: ceiling,
	})

	entry := s.pickEntry(table.Entries)

	// Resolve + spawn the yield. A missing template or spawn error does NOT
	// start the cooldown (the forage didn't really happen).
	if s.tpls == nil || s.store == nil {
		return ForageResult{Outcome: ForageFailed}
	}
	tpl, err := s.tpls.Get(item.TemplateID(entry.Item))
	if err != nil || tpl == nil {
		return ForageResult{Outcome: ForageOutputUndefined, ItemID: entry.Item}
	}
	qty := max(entry.Qty, 1)
	// Spawn transactionally: stage all instances first, and only file them
	// into the bag once every spawn succeeded. A mid-loop spawn failure
	// untracks the partial spawns and yields nothing — no orphaned items,
	// no partial yield (matters once a table uses qty >= 2; today all are 1).
	staged := make([]*entities.ItemInstance, 0, qty)
	for i := 0; i < qty; i++ {
		inst, err := s.store.Spawn(tpl)
		if err != nil {
			for _, p := range staged {
				_ = s.store.Untrack(p.ID())
			}
			return ForageResult{Outcome: ForageFailed, ItemID: entry.Item}
		}
		if qualityKey != "" {
			inst.SetProperty(propRarity, qualityKey)
		}
		staged = append(staged, inst)
	}
	var name string
	for _, inst := range staged {
		g.AddToInventory(inst.ID())
		name = inst.Name()
	}

	// A real yield happened — start the cooldown and roll the use-gain.
	g.SetForageReadyAt(now + s.cfg.ForageCooldownTicks)
	gained := s.GainFromUse(eid)

	return ForageResult{
		Outcome:    ForageOK,
		ItemID:     entry.Item,
		ItemName:   name,
		Qty:        qty,
		QualityKey: qualityKey,
		Gained:     gained,
	}
}

// pickEntry weighted-selects one entry from the pool (entries with weight
// <= 0 are excluded), serializing the roller through rollMu. The caller
// (Forage) guarantees hasSelectableEntry, so total > 0 — IntN is never
// called with 0 and the trailing fallback return is provably unreachable.
func (s *Service) pickEntry(entries []ForageEntry) ForageEntry {
	total := 0
	for _, e := range entries {
		if e.Weight > 0 {
			total += e.Weight
		}
	}
	s.rollMu.Lock()
	pick := s.roller.IntN(total)
	s.rollMu.Unlock()
	for _, e := range entries {
		if e.Weight <= 0 {
			continue
		}
		if pick < e.Weight {
			return e
		}
		pick -= e.Weight
	}
	// Unreachable: pick < total and the positive weights sum to total.
	return entries[len(entries)-1]
}

// entityID resolves the gatherer's progression key (PlayerID, falling back
// to the connection id), matching the command-layer convention.
func entityID(g Gatherer) string {
	if id := strings.TrimSpace(g.PlayerID()); id != "" {
		return id
	}
	return g.ID()
}
