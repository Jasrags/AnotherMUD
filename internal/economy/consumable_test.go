package economy

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// fakeConsumer satisfies economy.Consumer over a slice-backed
// inventory + an embedded sustenance value.
type fakeConsumer struct {
	id   string
	inv  []entities.EntityID
	sust int
}

func (c *fakeConsumer) ID() string                     { return c.id }
func (c *fakeConsumer) Sustenance() int                { return c.sust }
func (c *fakeConsumer) SetSustenance(v int)            { c.sust = v }
func (c *fakeConsumer) Inventory() []entities.EntityID { return c.inv }
func (c *fakeConsumer) RemoveFromInventory(id entities.EntityID) bool {
	for i, x := range c.inv {
		if x == id {
			c.inv = append(c.inv[:i], c.inv[i+1:]...)
			return true
		}
	}
	return false
}

// recordingConsumableSink captures events and can veto.
type recordingConsumableSink struct {
	consuming int
	consumed  int
	veto      bool
	last      ConsumeResult
}

func (s *recordingConsumableSink) OnItemConsuming(context.Context, entities.EntityID, entities.EntityID, string) bool {
	s.consuming++
	return s.veto
}
func (s *recordingConsumableSink) OnItemConsumed(_ context.Context, _ entities.EntityID, r ConsumeResult) {
	s.consumed++
	s.last = r
}

// spawnItem registers a template + instance in a fresh store and
// returns the store + the instance id. props become the template
// properties (copied onto the instance at spawn).
func spawnItem(t *testing.T, props map[string]any) (*entities.Store, entities.EntityID) {
	t.Helper()
	tpl := &item.Template{ID: "ration", Name: "a trail ration", Type: "item", Keywords: []string{"ration"}, Properties: props}
	store := entities.NewStore()
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	return store, inst.ID()
}

func newConsumeService(t *testing.T, props map[string]any, sink ConsumableSink) (*ConsumableService, *fakeConsumer, entities.EntityID) {
	t.Helper()
	store, id := spawnItem(t, props)
	svc := NewConsumableService(store, NewSustenanceService(DefaultSustenanceConfig()), sink)
	consumer := &fakeConsumer{id: "p1", inv: []entities.EntityID{id}, sust: 50}
	return svc, consumer, id
}

func TestConsume_SingleUseDestroysAndFeeds(t *testing.T) {
	sink := &recordingConsumableSink{}
	svc, consumer, id := newConsumeService(t, map[string]any{
		"consume_method":   "eat",
		"sustenance_value": 40,
	}, sink)

	res := svc.Consume(context.Background(), consumer, "p1", id, "eat")
	if res.Outcome != ConsumeOK {
		t.Fatalf("outcome = %v, want OK", res.Outcome)
	}
	if consumer.sust != 90 { // 50 + 40
		t.Errorf("sustenance = %d, want 90", consumer.sust)
	}
	if len(consumer.inv) != 0 {
		t.Errorf("inventory = %v, want empty (single-use destroyed)", consumer.inv)
	}
	if sink.consuming != 1 || sink.consumed != 1 {
		t.Errorf("events consuming=%d consumed=%d, want 1/1", sink.consuming, sink.consumed)
	}
}

func TestConsume_SustenanceClampsAt100(t *testing.T) {
	svc, consumer, id := newConsumeService(t, map[string]any{
		"consume_method":   "eat",
		"sustenance_value": 80,
	}, &recordingConsumableSink{})
	consumer.sust = 70
	svc.Consume(context.Background(), consumer, "p1", id, "eat")
	if consumer.sust != MaxSustenance {
		t.Errorf("sustenance = %d, want %d (clamped)", consumer.sust, MaxSustenance)
	}
}

func TestConsume_WrongMethod(t *testing.T) {
	sink := &recordingConsumableSink{}
	svc, consumer, id := newConsumeService(t, map[string]any{"consume_method": "drink"}, sink)
	res := svc.Consume(context.Background(), consumer, "p1", id, "eat")
	if res.Outcome != ConsumeWrongMethod {
		t.Fatalf("outcome = %v, want WrongMethod", res.Outcome)
	}
	if sink.consuming != 0 {
		t.Error("wrong-method must not fire the consuming pre-event")
	}
	if len(consumer.inv) != 1 {
		t.Error("item must survive a wrong-method attempt")
	}
}

func TestConsume_NoChargesFailsBeforePreEvent(t *testing.T) {
	sink := &recordingConsumableSink{}
	svc, consumer, id := newConsumeService(t, map[string]any{
		"consume_method": "use",
		"charges":        0,
	}, sink)
	res := svc.Consume(context.Background(), consumer, "p1", id, "use")
	if res.Outcome != ConsumeNoCharges {
		t.Fatalf("outcome = %v, want NoCharges", res.Outcome)
	}
	if sink.consuming != 0 {
		t.Error("no-charges must not fire the consuming pre-event")
	}
	if len(consumer.inv) != 1 {
		t.Error("item must survive a no-charges attempt")
	}
}

func TestConsume_CancelledKeepsItemAndCharge(t *testing.T) {
	sink := &recordingConsumableSink{veto: true}
	svc, consumer, id := newConsumeService(t, map[string]any{
		"consume_method": "use",
		"charges":        3,
	}, sink)
	res := svc.Consume(context.Background(), consumer, "p1", id, "use")
	if res.Outcome != ConsumeCancelled {
		t.Fatalf("outcome = %v, want Cancelled", res.Outcome)
	}
	inst, _ := svc.store.GetByID(id)
	if got := intProp(inst.(*entities.ItemInstance), "charges"); got != 3 {
		t.Errorf("charges = %d, want 3 (unspent on cancel)", got)
	}
	if len(consumer.inv) != 1 {
		t.Error("item must survive a cancelled consume")
	}
	if sink.consumed != 0 {
		t.Error("cancelled consume must not fire item.consumed")
	}
}

func TestConsume_ChargedDecrementsAndKeepsWhenDestroyOnEmptyFalse(t *testing.T) {
	svc, consumer, id := newConsumeService(t, map[string]any{
		"consume_method":   "drink",
		"charges":          2,
		"destroy_on_empty": false,
	}, &recordingConsumableSink{})

	// First drink: 2 → 1, survives.
	svc.Consume(context.Background(), consumer, "p1", id, "drink")
	inst, ok := svc.store.GetByID(id)
	if !ok {
		t.Fatal("item destroyed too early")
	}
	if got := intProp(inst.(*entities.ItemInstance), "charges"); got != 1 {
		t.Errorf("charges = %d, want 1", got)
	}

	// Second drink: 1 → 0, but destroy_on_empty=false keeps it for refill.
	svc.Consume(context.Background(), consumer, "p1", id, "drink")
	if _, ok := svc.store.GetByID(id); !ok {
		t.Error("destroy_on_empty=false item should survive at 0 charges")
	}
	if len(consumer.inv) != 1 {
		t.Error("empty refillable item should stay in inventory")
	}
}

func TestConsume_NotInInventory(t *testing.T) {
	svc, consumer, id := newConsumeService(t, map[string]any{"consume_method": "eat"}, &recordingConsumableSink{})
	consumer.inv = nil // empty it
	res := svc.Consume(context.Background(), consumer, "p1", id, "eat")
	if res.Outcome != ConsumeItemNotFound {
		t.Fatalf("outcome = %v, want ItemNotFound", res.Outcome)
	}
}

func TestConsume_EventConsumedCarriesEffectFields(t *testing.T) {
	sink := &recordingConsumableSink{}
	svc, consumer, id := newConsumeService(t, map[string]any{
		"consume_method":  "drink",
		"effect_id":       "bless",
		"effect_duration": 30,
		"effect_data":     map[string]any{"power": 5},
	}, sink)
	svc.Consume(context.Background(), consumer, "p1", id, "drink")
	if sink.last.EffectID != "bless" || sink.last.EffectDuration != 30 {
		t.Errorf("effect fields = %q/%d, want bless/30", sink.last.EffectID, sink.last.EffectDuration)
	}
	if sink.last.EffectData["power"] != 5 {
		t.Errorf("effect_data[power] = %d, want 5", sink.last.EffectData["power"])
	}
}
