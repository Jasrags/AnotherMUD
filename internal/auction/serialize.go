package auction

import (
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// Serialize captures a live item instance as a durable SerializedItem: its
// template id plus a copy of the per-instance property bag with the reserved
// keys removed. The reserved keys are engine-managed at (re)spawn —
// template_id is set by Spawn from the template, and room_id is filtered at
// instantiation and must never be re-added (entities §2.3) — so persisting
// them would be redundant at best and a write to a forbidden key at worst.
//
// Everything else in the bag IS persisted: the quality grade (incl. a
// craft's instance override), decorations (rarity/essence reserved
// properties), fill amounts, condition. That is the fidelity auction-house
// §4 requires and that the template-id-only player-save format does not
// provide.
func Serialize(inst *entities.ItemInstance) SerializedItem {
	si := SerializedItem{
		Template: string(inst.TemplateID()),
		Name:     inst.Name(),
	}
	props := inst.Properties() // snapshot, safe to mutate
	for k := range props {
		if k == entities.PropTemplateID || k == entities.PropRoomID {
			delete(props, k)
		}
	}
	if len(props) > 0 {
		si.Properties = props
	}
	return si
}

// Rehydrate reconstructs a live ItemInstance from a SerializedItem: spawn a
// fresh instance from the template (which seeds the reserved keys, the
// derived weapon/armor fields, tags, and keywords), then overlay the saved
// property bag so the grade, decorations, and other per-instance state come
// back intact. Reserved keys are skipped on overlay (Serialize already
// dropped them; this is belt-and-suspenders, since SetProperty rejects
// them). The instance is tracked in the store with a fresh runtime id —
// entity ids are reassigned each session, exactly like inventory respawn.
//
// Returns ErrTemplateGone (wrapping the registry error) when the template no
// longer exists — a content edit removed it since the listing was posted.
// The caller decides what to do with real player value tied to a vanished
// template (log + skip, refund); Rehydrate does not silently drop it.
func Rehydrate(store *entities.Store, tpls *item.Templates, si SerializedItem) (*entities.ItemInstance, error) {
	tpl, err := tpls.Get(item.TemplateID(si.Template))
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %v", ErrTemplateGone, si.Template, err)
	}
	inst, err := store.Spawn(tpl)
	if err != nil {
		return nil, fmt.Errorf("auction rehydrate spawn %q: %w", si.Template, err)
	}
	for k, v := range si.Properties {
		if k == entities.PropTemplateID || k == entities.PropRoomID {
			continue
		}
		inst.SetProperty(k, v)
	}
	return inst, nil
}
