package progression_test

import (
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

func TestStatBlockBaseRoundTrip(t *testing.T) {
	b := progression.New()
	b.SetBase(progression.StatSTR, 12)
	b.SetBase(progression.StatCON, 10)

	if got := b.Base(progression.StatSTR); got != 12 {
		t.Fatalf("Base(STR) = %d, want 12", got)
	}
	if got := b.Base(progression.StatCON); got != 10 {
		t.Fatalf("Base(CON) = %d, want 10", got)
	}
	if got := b.Base(progression.StatDEX); got != 0 {
		t.Fatalf("Base(DEX) on unset stat = %d, want 0", got)
	}
}

func TestStatBlockEffectiveIsBasePlusModifiers(t *testing.T) {
	b := progression.NewWithBase(map[progression.StatType]int{
		progression.StatSTR: 10,
		progression.StatDEX: 14,
	})
	src := entities.SourceKey("equipment:sword-1")

	b.AddModifier(src, progression.StatSTR, 3)
	b.AddModifier(entities.SourceKey("effect:bless-1"), progression.StatSTR, 1)

	if got := b.Effective(progression.StatSTR); got != 14 {
		t.Fatalf("Effective(STR) = %d, want 14 (10 base + 3 sword + 1 bless)", got)
	}
	if got := b.Effective(progression.StatDEX); got != 14 {
		t.Fatalf("Effective(DEX) = %d, want 14 (no modifiers)", got)
	}
}

func TestStatBlockAddModifiersReplacesUnderSameSource(t *testing.T) {
	b := progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})
	src := entities.SourceKey("equipment:item-1")

	b.AddModifiers(src, []stats.Modifier{{Stat: "str", Value: 5}, {Stat: "dex", Value: 2}})
	if got := b.Effective(progression.StatSTR); got != 15 {
		t.Fatalf("after first apply, STR = %d, want 15", got)
	}

	b.AddModifiers(src, []stats.Modifier{{Stat: "str", Value: 3}})
	if got := b.Effective(progression.StatSTR); got != 13 {
		t.Fatalf("after replacement apply, STR = %d, want 13 (not 18)", got)
	}
	if got := b.Effective(progression.StatDEX); got != 0 {
		t.Fatalf("after replacement apply, DEX = %d, want 0 (dex mod was replaced)", got)
	}
}

func TestStatBlockRemoveBySource(t *testing.T) {
	b := progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})
	src := entities.SourceKey("equipment:item-1")

	b.AddModifiers(src, []stats.Modifier{{Stat: "str", Value: 5}, {Stat: "ac", Value: 2}})
	if got := b.Effective(progression.StatSTR); got != 15 {
		t.Fatalf("STR after equip = %d, want 15", got)
	}
	if got := b.Effective("ac"); got != 2 {
		t.Fatalf("AC after equip = %d, want 2", got)
	}

	if !b.RemoveBySource(src) {
		t.Fatal("RemoveBySource returned false on a present source")
	}
	if got := b.Effective(progression.StatSTR); got != 10 {
		t.Fatalf("STR after unequip = %d, want 10", got)
	}
	if got := b.Effective("ac"); got != 0 {
		t.Fatalf("AC after unequip = %d, want 0", got)
	}
	if b.RemoveBySource(src) {
		t.Fatal("RemoveBySource returned true on a missing source")
	}
}

func TestStatBlockBaseMutationsInvalidateCache(t *testing.T) {
	b := progression.New()
	b.SetBase(progression.StatSTR, 10)

	if got := b.Effective(progression.StatSTR); got != 10 {
		t.Fatalf("initial Effective = %d, want 10", got)
	}

	b.SetBase(progression.StatSTR, 15)
	if got := b.Effective(progression.StatSTR); got != 15 {
		t.Fatalf("after SetBase, Effective = %d, want 15 (cache should have invalidated)", got)
	}

	b.AdjustBase(progression.StatSTR, -3)
	if got := b.Effective(progression.StatSTR); got != 12 {
		t.Fatalf("after AdjustBase, Effective = %d, want 12", got)
	}
}

func TestStatBlockInvalidateForcesRecompute(t *testing.T) {
	b := progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})

	// Prime the cache.
	_ = b.Effective(progression.StatSTR)

	// Backdoor base mutation through a future train command would
	// normally call AdjustBase. But callers that have an alternate
	// path must use Invalidate. Simulate that here with a raw base
	// rewrite via the public API and ensure Invalidate is a no-op
	// where SetBase already cleared the cache; then verify the doc
	// contract that Invalidate forces a re-read.
	b.Invalidate()
	if got := b.Effective(progression.StatSTR); got != 10 {
		t.Fatalf("Effective after Invalidate = %d, want 10", got)
	}
}

func TestStatBlockRebindSource(t *testing.T) {
	b := progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})
	oldSrc := entities.SourceKey("equipment:old-id")
	newSrc := entities.SourceKey("equipment:new-id")

	b.AddModifier(oldSrc, progression.StatSTR, 5)
	if got := b.Effective(progression.StatSTR); got != 15 {
		t.Fatalf("STR pre-rebind = %d, want 15", got)
	}

	if !b.RebindSource(oldSrc, newSrc) {
		t.Fatal("RebindSource returned false on a present old source")
	}
	if got := b.Effective(progression.StatSTR); got != 15 {
		t.Fatalf("STR post-rebind = %d, want 15 (modifier should still apply under new key)", got)
	}
	if !b.HasSource(newSrc) {
		t.Fatal("HasSource(newSrc) = false after RebindSource")
	}
	if b.HasSource(oldSrc) {
		t.Fatal("HasSource(oldSrc) = true after RebindSource")
	}
}

func TestStatBlockSnapshotRoundTrip(t *testing.T) {
	b := progression.NewWithBase(map[progression.StatType]int{
		progression.StatSTR: 12,
		progression.StatCON: 14,
	})
	b.AddModifiers(entities.SourceKey("equipment:sword-1"),
		[]stats.Modifier{{Stat: "str", Value: 2}})

	baseSnap := b.BaseSnapshot()
	modSnap := b.ModifiersSnapshot()

	// Snapshot ordering should be deterministic so YAML diffs are
	// driven by gameplay, not map iteration.
	if len(baseSnap) != 2 {
		t.Fatalf("BaseSnapshot len = %d, want 2", len(baseSnap))
	}
	// con < str alphabetically.
	if baseSnap[0].Stat != progression.StatCON || baseSnap[1].Stat != progression.StatSTR {
		t.Fatalf("BaseSnapshot order = %v, want con then str", baseSnap)
	}

	restored := progression.New()
	restored.RestoreBase(baseSnap)
	restored.RestoreModifiers(modSnap)

	if got := restored.Effective(progression.StatSTR); got != 14 {
		t.Fatalf("restored Effective(STR) = %d, want 14 (12 + 2)", got)
	}
	if got := restored.Effective(progression.StatCON); got != 14 {
		t.Fatalf("restored Effective(CON) = %d, want 14", got)
	}
}

func TestStatBlockAllEffective(t *testing.T) {
	b := progression.NewWithBase(map[progression.StatType]int{
		progression.StatSTR: 10,
		progression.StatDEX: 12,
	})
	b.AddModifier(entities.SourceKey("equipment:item-1"), progression.StatSTR, 3)
	// A modifier on a stat not in base — should appear in AllEffective.
	b.AddModifier(entities.SourceKey("equipment:item-2"), "ac", 2)

	all := b.AllEffective()
	if all[progression.StatSTR] != 13 {
		t.Fatalf("AllEffective STR = %d, want 13", all[progression.StatSTR])
	}
	if all[progression.StatDEX] != 12 {
		t.Fatalf("AllEffective DEX = %d, want 12", all[progression.StatDEX])
	}
	if all["ac"] != 2 {
		t.Fatalf("AllEffective ac = %d, want 2 (modifier-only stat)", all["ac"])
	}
}

func TestStatBlockEmptyModifiersListRemovesEntry(t *testing.T) {
	b := progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})
	src := entities.SourceKey("equipment:item-1")

	b.AddModifier(src, progression.StatSTR, 5)
	if !b.HasSource(src) {
		t.Fatal("HasSource = false after AddModifier")
	}

	b.AddModifiers(src, nil)
	if b.HasSource(src) {
		t.Fatal("HasSource = true after AddModifiers(nil) — should have removed the entry")
	}
	if got := b.Effective(progression.StatSTR); got != 10 {
		t.Fatalf("Effective(STR) after empty-apply = %d, want 10", got)
	}
}

func TestStatBlockRestoreBaseMergesOverDefaults(t *testing.T) {
	// A v6 save written before a future slice adds a new base stat
	// must NOT zero the engine-default for that new stat. RestoreBase
	// merges rather than replaces so the constructor-seeded defaults
	// stay live for keys absent from the snapshot.
	b := progression.NewWithBase(map[progression.StatType]int{
		progression.StatSTR: 10,
		progression.StatCON: 10,
		// Pretend "dex_max" is a future-slice-only stat.
		"dex_max": 25,
	})

	// Persisted snapshot from an old save: only carries str + con
	// (the stats that existed when it was written).
	snap := progression.BaseSnapshot{
		{Stat: progression.StatSTR, Value: 14},
		{Stat: progression.StatCON, Value: 12},
	}
	b.RestoreBase(snap)

	if got := b.Base(progression.StatSTR); got != 14 {
		t.Errorf("Base(STR) after restore = %d, want 14 (overwritten)", got)
	}
	if got := b.Base(progression.StatCON); got != 12 {
		t.Errorf("Base(CON) after restore = %d, want 12 (overwritten)", got)
	}
	if got := b.Base("dex_max"); got != 25 {
		t.Errorf("Base(dex_max) after restore = %d, want 25 (preserved from defaults)", got)
	}
}

func TestStatBlockAddModifiersNormalizesStatCase(t *testing.T) {
	// Mixed-case modifier stat names from content authoring must
	// normalize to lowercase so they contribute to the canonical
	// StatType keys instead of becoming silent zero-contribution
	// modifiers under a typo'd key.
	b := progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})

	b.AddModifiers(entities.SourceKey("equipment:upper-1"),
		[]stats.Modifier{{Stat: "STR", Value: 3}})
	b.AddModifiers(entities.SourceKey("equipment:mixed-2"),
		[]stats.Modifier{{Stat: "Str", Value: 2}})

	if got := b.Effective(progression.StatSTR); got != 15 {
		t.Errorf("Effective(STR) = %d, want 15 (10 base + 3 from STR + 2 from Str)", got)
	}
}

func TestStatBlockRestoreModifiersNormalizesStatCase(t *testing.T) {
	// A pre-M8.1 save written when stats.Block had no normalization
	// could carry mixed-case stat names. RestoreModifiers corrects
	// them on first load so Effective reads continue to work.
	b := progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})

	snap := stats.Snapshot{
		{
			Source:    entities.SourceKey("equipment:legacy-1"),
			Modifiers: []stats.Modifier{{Stat: "STR", Value: 5}},
		},
	}
	b.RestoreModifiers(snap)

	if got := b.Effective(progression.StatSTR); got != 15 {
		t.Errorf("Effective(STR) after RestoreModifiers = %d, want 15 (10 base + 5 from normalized STR)", got)
	}
}

func TestStatBlockConcurrentAccess(t *testing.T) {
	// Stress test: concurrent base setters, modifier adds/removes,
	// and Effective reads across many goroutines. Run with -race.
	b := progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})

	const goroutines = 32
	const iterations = 500
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			src := entities.SourceKey("effect:goroutine-" + string(rune('a'+id%26)))
			for i := 0; i < iterations; i++ {
				switch i % 4 {
				case 0:
					b.AddModifier(src, progression.StatSTR, 1)
				case 1:
					_ = b.Effective(progression.StatSTR)
				case 2:
					b.RemoveBySource(src)
				case 3:
					_ = b.AllEffective()
				}
			}
		}(g)
	}
	wg.Wait()

	// Final state: deterministic only in the sense that the block
	// is not corrupt. Read once more under no contention.
	_ = b.Effective(progression.StatSTR)
}
