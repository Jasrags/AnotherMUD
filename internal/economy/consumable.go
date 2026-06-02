package economy

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Consumables are the M11.5 eat/drink/use pipeline (spec
// economy-survival §6): an item carries content-driven knobs
// (consume_method, sustenance_value, effect_*, charges) and the
// ConsumableService runs the §6.2 operation — charge gate, cancellable
// item.consuming, sustenance replenishment, item.consumed emit, then
// destruction. Effect application is deliberately NOT done here (§6.3):
// the service emits item.consumed carrying the effect parameters and
// the effects feature subscribes, so content can add effect types
// without touching this path.

// Consumable property keys (spec §6.1). Shared with the fill operation
// for charges / max_charges (inventory-equipment-items §4.6).
const (
	PropConsumeMethod   = "consume_method"
	PropSustenanceValue = "sustenance_value"
	PropEffectID        = "effect_id"
	PropEffectDuration  = "effect_duration"
	PropEffectData      = "effect_data"
	PropCharges         = "charges"
	PropMaxCharges      = "max_charges"
	PropDestroyOnEmpty  = "destroy_on_empty"
)

// ConsumeOutcome enumerates the §6.2 result reasons.
type ConsumeOutcome int

const (
	// ConsumeOK — the item was consumed.
	ConsumeOK ConsumeOutcome = iota
	// ConsumeItemNotFound — the entity has no such item at the top level
	// of its contents (§6.2 steps 1-2; nested items are not consumable,
	// §6.5).
	ConsumeItemNotFound
	// ConsumeWrongMethod — the item's consume_method does not match the
	// verb used (e.g. `eat` on a drink). Carried so the caller can name
	// the right verb. Not a spec return code — the spec leaves the
	// method→command binding (§6.1) to the command layer; surfacing it
	// as an outcome keeps that decision in one place.
	ConsumeWrongMethod
	// ConsumeNoCharges — the item declares charges and they are at or
	// below zero (§6.2 step 3). The pre-event does NOT fire.
	ConsumeNoCharges
	// ConsumeCancelled — a listener vetoed the item.consuming pre-event
	// (§6.2 step 5). No charge spent, no destruction.
	ConsumeCancelled
)

// ConsumeResult is the outcome of a consume, returned to the caller and
// carried (on success) into the item.consumed event. The effect fields
// are populated from the item's properties so the effects subscriber
// can build an effect (§6.3).
type ConsumeResult struct {
	Outcome         ConsumeOutcome
	ItemID          entities.EntityID
	ItemName        string
	Method          string
	SustenanceValue int
	EffectID        string
	EffectDuration  int
	EffectData      map[string]int
}

// Consumer is the entity a consume operates on (spec §6.2). It must
// expose its top-level contents (Inventory), be able to drop a consumed
// item (RemoveFromInventory), and carry a sustenance pool for §6.2 step
// 8. The connActor satisfies it. Note ID() (from SustenanceEntity) is
// NOT used as the event actor id — the caller passes the player entity
// id explicitly, since connActor.ID() is the connection id.
type Consumer interface {
	SustenanceEntity
	Inventory() []entities.EntityID
	RemoveFromInventory(entities.EntityID) bool
}

// ConsumableSink bridges the two consume events to the bus. The
// composition root implements it. OnItemConsuming is the cancellable
// pre-event (returns cancelled); OnItemConsumed is the post
// notification fired while the item still exists.
type ConsumableSink interface {
	OnItemConsuming(ctx context.Context, actorID, itemID entities.EntityID, method string) (cancelled bool)
	OnItemConsumed(ctx context.Context, actorID entities.EntityID, r ConsumeResult)
}

// nopConsumableSink discards events and never cancels.
type nopConsumableSink struct{}

func (nopConsumableSink) OnItemConsuming(context.Context, entities.EntityID, entities.EntityID, string) bool {
	return false
}
func (nopConsumableSink) OnItemConsumed(context.Context, entities.EntityID, ConsumeResult) {}

// ConsumableService runs the §6.2 consume operation over the entity
// store. It mutates item charges and the consumer's sustenance, so it
// holds references to the store and the sustenance service; the sink
// bridges the two observable events.
type ConsumableService struct {
	store      *entities.Store
	sustenance *SustenanceService
	sink       ConsumableSink
}

// NewConsumableService wires a service. A nil sink becomes a nop.
func NewConsumableService(store *entities.Store, sustenance *SustenanceService, sink ConsumableSink) *ConsumableService {
	if sink == nil {
		sink = nopConsumableSink{}
	}
	return &ConsumableService{store: store, sustenance: sustenance, sink: sink}
}

// Consume runs the spec §6.2 pipeline. actorEntityID is the consumer's
// PLAYER entity id (used in the events); consumer is the live holder.
// viaMethod is the gate the calling verb imposes:
//   - a specific method ("eat"/"drink") consumes ONLY items whose
//     consume_method matches;
//   - an empty string is the generic fallback (the `use` verb): it
//     consumes any item that IS a consumable — i.e. declares a
//     non-empty consume_method — but still rejects non-consumables so
//     `use <sword>` can't destroy gear.
//
// itemID must be a top-level item in the consumer's inventory.
func (s *ConsumableService) Consume(ctx context.Context, consumer Consumer, actorEntityID, itemID entities.EntityID, viaMethod string) ConsumeResult {
	if consumer == nil || s.store == nil {
		return ConsumeResult{Outcome: ConsumeItemNotFound}
	}

	// Step 2: the item must be a direct child of the consumer's contents
	// (§6.5 — nested items are not consumable).
	if !containsID(consumer.Inventory(), itemID) {
		return ConsumeResult{Outcome: ConsumeItemNotFound}
	}
	e, ok := s.store.GetByID(itemID)
	if !ok {
		return ConsumeResult{Outcome: ConsumeItemNotFound}
	}
	it, ok := e.(*entities.ItemInstance)
	if !ok {
		return ConsumeResult{Outcome: ConsumeItemNotFound}
	}

	method := stringProp(it, PropConsumeMethod)
	// Method gate (§6.1). A specific verb consumes only its own method;
	// the generic `use` fallback (empty viaMethod) consumes any item
	// that declares a consume_method but rejects items with none, so it
	// never destroys a non-consumable (e.g. `use sword`).
	if viaMethod == "" {
		if method == "" {
			return ConsumeResult{Outcome: ConsumeWrongMethod, ItemID: itemID, ItemName: it.Name(), Method: method}
		}
	} else if method != viaMethod {
		return ConsumeResult{Outcome: ConsumeWrongMethod, ItemID: itemID, ItemName: it.Name(), Method: method}
	}

	// Step 3: charge gate. Only items that DECLARE charges are gated; a
	// single-use item (no charges key) skips straight through.
	hasCharges := hasProp(it, PropCharges)
	charges := intProp(it, PropCharges)
	if hasCharges && charges <= 0 {
		return ConsumeResult{Outcome: ConsumeNoCharges, ItemID: itemID, ItemName: it.Name(), Method: method}
	}

	// Step 5: cancellable pre-event, BEFORE any charge spend / destroy.
	if s.sink.OnItemConsuming(ctx, actorEntityID, itemID, method) {
		return ConsumeResult{Outcome: ConsumeCancelled, ItemID: itemID, ItemName: it.Name(), Method: method}
	}

	// Step 6: snapshot the effect + sustenance params BEFORE mutation so
	// the result/event reflect the pre-consume item.
	result := ConsumeResult{
		Outcome:         ConsumeOK,
		ItemID:          itemID,
		ItemName:        it.Name(),
		Method:          method,
		SustenanceValue: intProp(it, PropSustenanceValue),
		EffectID:        stringProp(it, PropEffectID),
		EffectDuration:  intProp(it, PropEffectDuration),
		EffectData:      intMapProp(it, PropEffectData),
	}

	// Step 7: decrement / mark for destruction.
	destroy := false
	if hasCharges {
		charges--
		it.SetProperty(PropCharges, charges)
		if charges <= 0 && destroyOnEmpty(it) {
			destroy = true
		}
	} else {
		destroy = true // single-use
	}

	// Step 8: apply sustenance, clamped at the engine cap by the service.
	if result.SustenanceValue > 0 && s.sustenance != nil {
		s.sustenance.Add(consumer, result.SustenanceValue)
	}

	// Step 9: emit item.consumed while the item still exists in memory.
	s.sink.OnItemConsumed(ctx, actorEntityID, result)

	// Step 10: destroy if marked — remove from the holder and untrack.
	if destroy {
		consumer.RemoveFromInventory(itemID)
		_ = s.store.Untrack(itemID)
	}
	return result
}

// destroyOnEmpty reads the destroy_on_empty flag, defaulting to true
// (spec §6.1 / §6.2 step 7).
func destroyOnEmpty(it *entities.ItemInstance) bool {
	v, ok := it.Property(PropDestroyOnEmpty)
	if !ok {
		return true
	}
	b, ok := v.(bool)
	if !ok {
		return true
	}
	return b
}

func containsID(ids []entities.EntityID, want entities.EntityID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// hasProp reports whether the property key is present (distinguishes
// "charges: 0" from "no charges key" for the §6.2 step 3 gate).
func hasProp(it *entities.ItemInstance, key string) bool {
	_, ok := it.Property(key)
	return ok
}

// intProp reads an integer property, normalizing the int/int64/float64
// shapes yaml.v3 produces. Zero when absent or non-numeric.
func intProp(it *entities.ItemInstance, key string) int {
	v, _ := it.Property(key)
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// stringProp reads a string property, empty when absent or non-string.
func stringProp(it *entities.ItemInstance, key string) string {
	if v, ok := it.Property(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// intMapProp reads a map[string]int property (effect_data), tolerating
// the map[string]any decode yaml.v3 produces. Nil when absent.
func intMapProp(it *entities.ItemInstance, key string) map[string]int {
	raw, ok := it.Property(key)
	if !ok {
		return nil
	}
	switch m := raw.(type) {
	case map[string]int:
		return m
	case map[string]any:
		out := make(map[string]int, len(m))
		for k, v := range m {
			switch n := v.(type) {
			case int:
				out[k] = n
			case int64:
				out[k] = int(n)
			case float64:
				out[k] = int(n)
			}
		}
		return out
	default:
		return nil
	}
}
