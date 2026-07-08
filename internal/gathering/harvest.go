package gathering

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// HarvestOutcome classifies a harvest attempt for the caller to render.
type HarvestOutcome int

const (
	HarvestOK HarvestOutcome = iota
	// HarvestNeedsTool — the node requires a tool the harvester lacks
	// (§3.3): the ONE permitted refusal in gathering. RequiredTool names it.
	HarvestNeedsTool
	// HarvestNoCharges — the node is already exhausted (defensive; a
	// depleted node is normally removed before this can be hit).
	HarvestNoCharges
	// HarvestEmptyTable — the node's yield table is missing/empty.
	HarvestEmptyTable
	// HarvestOutputUndefined — the picked yield item template is missing.
	HarvestOutputUndefined
	// HarvestFailed — the yield could not be spawned (store error).
	HarvestFailed
)

// HarvestResult is the structured outcome of Harvest. On HarvestOK the
// yield has been spawned into the harvester's inventory and the node's
// charges decremented; Depleted is true when that was the node's last
// charge, so the caller removes the node entity (untrack + unplace) and
// fires node.depleted — the §3.6 reset algorithm then respawns it.
type HarvestResult struct {
	Outcome      HarvestOutcome
	ItemID       string
	ItemName     string
	Qty          int
	QualityKey   string
	Gained       bool
	Depleted     bool
	RequiredTool string // set on HarvestNeedsTool
}

// Harvest performs one harvest of node against its yield table
// (gathering.md §3.2): it enforces the node's tool requirement (§3.3 — the
// only refusal), rolls quality from the harvester's gathering proficiency +
// the node yield table's richness/ceiling (§4), spawns the yield into the
// harvester's inventory, and decrements the node's charge count. Charges
// are the node's limiter (§5) — there is no separate cooldown. The caller
// owns removing a depleted node from the room (the Service has no placement
// index); Harvest reports Depleted so the caller can remove it + fire
// node.depleted.
func (s *Service) Harvest(_ context.Context, g Gatherer, node *entities.ItemInstance, yield *ForageTable) HarvestResult {
	if s == nil || node == nil {
		return HarvestResult{Outcome: HarvestFailed}
	}

	// Tool gate (§3.3) — the one allowed refusal. Empty requirement never
	// refuses; a missing tool does.
	if tool := propString(node, PropNodeRequiredTool); tool != "" && !s.hasToolTag(g, tool) {
		return HarvestResult{Outcome: HarvestNeedsTool, RequiredTool: tool}
	}

	// Cheap early-out (the authoritative claim is TakeCharge below).
	if nodeCharges(node) <= 0 {
		return HarvestResult{Outcome: HarvestNoCharges}
	}
	if yield == nil || !yield.hasSelectableEntry() {
		return HarvestResult{Outcome: HarvestEmptyTable}
	}
	if s.tpls == nil || s.store == nil {
		return HarvestResult{Outcome: HarvestFailed}
	}

	eid := entityID(g)
	skill := 0
	if s.prof != nil {
		skill, _ = s.prof.Proficiency(eid, GatheringAbility)
	}
	ceiling := ladderPosition(s.rarity, yield.Ceiling)
	qualityKey := s.rollQuality(QualityInputs{Skill: skill, Richness: yield.Richness, SourceCeiling: ceiling})
	entry := s.pickEntry(yield.Entries)

	tpl, err := s.tpls.Get(item.TemplateID(entry.Item))
	if err != nil || tpl == nil {
		return HarvestResult{Outcome: HarvestOutputUndefined, ItemID: entry.Item}
	}
	qty := max(entry.Qty, 1)
	// Stage the yield (spawn but don't file into the bag yet), THEN claim a
	// charge atomically. Spawning before the claim means a spawn failure
	// consumes no charge; claiming via TakeCharge means two concurrent
	// harvests of the same 1-charge node can't both win — the loser untracks
	// its staged yield and reports NoCharges (no dupe, §8).
	staged := make([]*entities.ItemInstance, 0, qty)
	for i := 0; i < qty; i++ {
		inst, err := s.store.Spawn(tpl)
		if err != nil {
			for _, p := range staged {
				_ = s.store.Untrack(p.ID())
			}
			return HarvestResult{Outcome: HarvestFailed, ItemID: entry.Item}
		}
		if qualityKey != "" {
			inst.SetProperty(propRarity, qualityKey)
		}
		staged = append(staged, inst)
	}

	remaining, taken := node.TakeCharge(PropNodeCharges)
	if !taken {
		// Lost the race for the last charge (or it emptied between the
		// early-out and here) — discard the staged yield, grant nothing.
		for _, p := range staged {
			_ = s.store.Untrack(p.ID())
		}
		return HarvestResult{Outcome: HarvestNoCharges}
	}

	var name string
	for _, inst := range staged {
		g.AddToInventory(inst.ID())
		name = inst.Name()
	}
	gained := s.GainFromUse(eid)

	return HarvestResult{
		Outcome:    HarvestOK,
		ItemID:     entry.Item,
		ItemName:   name,
		Qty:        qty,
		QualityKey: qualityKey,
		Gained:     gained,
		Depleted:   remaining <= 0,
	}
}

// hasToolTag reports whether the gatherer carries an item tagged tag
// (case-insensitive) — the §3.3 tool check.
func (s *Service) hasToolTag(g Gatherer, tag string) bool {
	if s.store == nil {
		return false
	}
	for _, id := range g.Inventory() {
		e, ok := s.store.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		for _, t := range it.Tags() {
			if strings.EqualFold(t, tag) {
				return true
			}
		}
	}
	return false
}

// nodeCharges reads the mutable charge count off a node instance (0 when
// absent), tolerating the int / int64 / float64 shapes yaml/property
// round-trips produce.
func nodeCharges(node *entities.ItemInstance) int {
	v, _ := node.Property(PropNodeCharges)
	return propInt(v)
}

// propString reads a string property off a node instance ("" when absent).
func propString(it *entities.ItemInstance, key string) string {
	if v, ok := it.Property(key); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
