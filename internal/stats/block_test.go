package stats_test

import (
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

func TestApplyAndHas(t *testing.T) {
	b := stats.New()
	src := entities.EquipmentSourceKey("entity-1")

	if b.Has(src) {
		t.Fatal("Has on empty block returned true")
	}
	b.Apply(src, []stats.Modifier{{Stat: "str", Value: 1}})
	if !b.Has(src) {
		t.Fatal("Has after Apply returned false")
	}
}

func TestApplyEmptyDeletesEntry(t *testing.T) {
	b := stats.New()
	src := entities.EquipmentSourceKey("entity-1")
	b.Apply(src, []stats.Modifier{{Stat: "str", Value: 1}})
	b.Apply(src, nil)
	if b.Has(src) {
		t.Fatal("empty Apply did not clear entry")
	}
}

func TestApplyReplacesNotAppends(t *testing.T) {
	b := stats.New()
	src := entities.EquipmentSourceKey("entity-1")
	b.Apply(src, []stats.Modifier{{Stat: "str", Value: 1}})
	b.Apply(src, []stats.Modifier{{Stat: "dex", Value: 2}})

	snap := b.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snap))
	}
	if len(snap[0].Modifiers) != 1 || snap[0].Modifiers[0].Stat != "dex" {
		t.Errorf("modifiers = %+v, want only {dex,2}", snap[0].Modifiers)
	}
}

func TestRemove(t *testing.T) {
	b := stats.New()
	src := entities.EquipmentSourceKey("entity-1")

	if b.Remove(src) {
		t.Error("Remove on missing src returned true")
	}
	b.Apply(src, []stats.Modifier{{Stat: "str", Value: 1}})
	if !b.Remove(src) {
		t.Error("Remove on present src returned false")
	}
	if b.Has(src) {
		t.Error("Has after Remove returned true")
	}
}

func TestRebindSource(t *testing.T) {
	b := stats.New()
	old := entities.EquipmentSourceKey("entity-1")
	new := entities.EquipmentSourceKey("entity-7")

	b.Apply(old, []stats.Modifier{{Stat: "str", Value: 1}, {Stat: "dex", Value: 2}})
	if !b.RebindSource(old, new) {
		t.Fatal("RebindSource returned false")
	}
	if b.Has(old) {
		t.Error("old key still present after rebind")
	}
	if !b.Has(new) {
		t.Error("new key absent after rebind")
	}
	snap := b.Snapshot()
	if len(snap) != 1 || len(snap[0].Modifiers) != 2 {
		t.Fatalf("snapshot lost modifiers: %+v", snap)
	}
}

func TestRebindSourceSameKey(t *testing.T) {
	b := stats.New()
	src := entities.EquipmentSourceKey("entity-1")
	b.Apply(src, []stats.Modifier{{Stat: "str", Value: 1}})
	if !b.RebindSource(src, src) {
		t.Error("RebindSource(same,same) returned false")
	}
	if !b.Has(src) {
		t.Error("self-rebind lost modifiers")
	}
}

func TestRebindSourceMissingOld(t *testing.T) {
	b := stats.New()
	if b.RebindSource(entities.EquipmentSourceKey("nope"), entities.EquipmentSourceKey("new")) {
		t.Error("RebindSource on missing old returned true")
	}
}

func TestRebindSourceCollisionFails(t *testing.T) {
	b := stats.New()
	old := entities.EquipmentSourceKey("entity-1")
	new := entities.EquipmentSourceKey("entity-2")

	b.Apply(old, []stats.Modifier{{Stat: "str", Value: 1}})
	b.Apply(new, []stats.Modifier{{Stat: "dex", Value: 2}})

	if b.RebindSource(old, new) {
		t.Fatal("RebindSource overwrote a populated target")
	}
	// Both keys should still be present and intact.
	if !b.Has(old) || !b.Has(new) {
		t.Error("collision rebind disturbed existing entries")
	}
}

func TestSnapshotIsDeterministic(t *testing.T) {
	b := stats.New()
	b.Apply(entities.EquipmentSourceKey("entity-9"), []stats.Modifier{{Stat: "str", Value: 1}})
	b.Apply(entities.EquipmentSourceKey("entity-1"), []stats.Modifier{{Stat: "dex", Value: 1}})
	b.Apply(entities.EquipmentSourceKey("entity-5"), []stats.Modifier{{Stat: "con", Value: 1}})

	snap := b.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("len = %d, want 3", len(snap))
	}
	// Sources sort lexicographically: equipment:entity-1, equipment:entity-5, equipment:entity-9.
	want := []entities.SourceKey{
		entities.EquipmentSourceKey("entity-1"),
		entities.EquipmentSourceKey("entity-5"),
		entities.EquipmentSourceKey("entity-9"),
	}
	for i, w := range want {
		if snap[i].Source != w {
			t.Errorf("snap[%d].Source = %q, want %q", i, snap[i].Source, w)
		}
	}
}

func TestSnapshotEmpty(t *testing.T) {
	if got := stats.New().Snapshot(); got != nil {
		t.Errorf("empty Snapshot = %v, want nil", got)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	src := entities.EquipmentSourceKey("entity-1")
	snap := stats.Snapshot{
		{Source: src, Modifiers: []stats.Modifier{{Stat: "str", Value: 1}}},
	}
	b := stats.New()
	b.Restore(snap)
	if !b.Has(src) {
		t.Fatal("Restore did not install modifiers")
	}
	got := b.Snapshot()
	if len(got) != 1 || got[0].Source != src || got[0].Modifiers[0].Value != 1 {
		t.Errorf("Restore round-trip lost data: %+v", got)
	}
}

func TestRestoreSkipsEmptyEntries(t *testing.T) {
	b := stats.New()
	b.Restore(stats.Snapshot{
		{Source: entities.EquipmentSourceKey("a"), Modifiers: nil},
	})
	if b.Snapshot() != nil {
		t.Error("Restore kept an empty entry")
	}
}

func TestConcurrentApplyRemove(t *testing.T) {
	// Race detector smoke test — interleave Apply/Remove/Snapshot from
	// many goroutines to ensure the internal lock guards everything.
	b := stats.New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			src := entities.EquipmentSourceKey(entities.EntityID(rune('a' + i%10)))
			b.Apply(src, []stats.Modifier{{Stat: "str", Value: i}})
			b.Snapshot()
			b.Remove(src)
		}()
	}
	wg.Wait()
}
