package script_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/script"
)

func TestRegistry_RegisterAndAllRoundTrip(t *testing.T) {
	r := script.New()
	entries := []script.Entry{
		{PackID: "core", Path: "scripts/a.lua", Source: "-- a", LoadOrder: 10},
		{PackID: "core", Path: "scripts/b.lua", Source: "-- b", LoadOrder: 10},
		{PackID: "other", Path: "scripts/c.lua", Source: "-- c", LoadOrder: 5},
	}
	for _, e := range entries {
		if err := r.Register(e); err != nil {
			t.Fatalf("Register %+v: %v", e, err)
		}
	}
	if got := r.Len(); got != 3 {
		t.Errorf("Len = %d, want 3", got)
	}
}

func TestRegistry_All_StableLoadOrderThenPath(t *testing.T) {
	r := script.New()
	// Insert out of order; All should sort by LoadOrder, then Path.
	in := []script.Entry{
		{PackID: "z", Path: "scripts/zeta.lua", LoadOrder: 20},
		{PackID: "core", Path: "scripts/b.lua", LoadOrder: 10},
		{PackID: "core", Path: "scripts/a.lua", LoadOrder: 10},
		{PackID: "early", Path: "scripts/init.lua", LoadOrder: 5},
	}
	for _, e := range in {
		_ = r.Register(e)
	}
	got := r.All()
	wantPaths := []string{
		"scripts/init.lua", // LoadOrder=5
		"scripts/a.lua",    // LoadOrder=10, alphabetic first
		"scripts/b.lua",    // LoadOrder=10
		"scripts/zeta.lua", // LoadOrder=20
	}
	if len(got) != len(wantPaths) {
		t.Fatalf("len(All) = %d, want %d", len(got), len(wantPaths))
	}
	for i, e := range got {
		if e.Path != wantPaths[i] {
			t.Errorf("All[%d].Path = %q, want %q", i, e.Path, wantPaths[i])
		}
	}
}

func TestRegistry_DuplicateRegistration_Rejected(t *testing.T) {
	r := script.New()
	e := script.Entry{PackID: "core", Path: "scripts/dup.lua", Source: "-- x"}
	if err := r.Register(e); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := r.Register(e)
	if err == nil {
		t.Fatal("duplicate Register should have errored")
	}
	var de *script.DuplicateError
	if !errors.As(err, &de) {
		t.Fatalf("expected *DuplicateError, got %T: %v", err, err)
	}
	if de.PackID != "core" || de.Path != "scripts/dup.lua" {
		t.Errorf("DuplicateError = %+v, want core/scripts/dup.lua", de)
	}
}

func TestRegistry_DifferentPackSamePath_Allowed(t *testing.T) {
	// Two packs each carrying their own scripts/init.lua must
	// register independently — the (PackID, Path) tuple is the
	// uniqueness key, not Path alone.
	r := script.New()
	if err := r.Register(script.Entry{PackID: "core", Path: "scripts/init.lua"}); err != nil {
		t.Fatalf("core: %v", err)
	}
	if err := r.Register(script.Entry{PackID: "other", Path: "scripts/init.lua"}); err != nil {
		t.Errorf("other pack same path: %v", err)
	}
}

func TestRegistry_ConcurrentRegister_Serialized(t *testing.T) {
	r := script.New()
	const N = 32
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := "scripts/p" + string(rune('a'+i%26)) + ".lua"
			_ = r.Register(script.Entry{
				PackID: "core",
				Path:   path + string(rune('0'+i/26)),
			})
		}(i)
	}
	wg.Wait()
	if got := r.Len(); got != N {
		t.Errorf("Len after %d concurrent registers = %d", N, got)
	}
}

func TestRegistry_All_ReturnsSnapshotNotAlias(t *testing.T) {
	// Mutating the returned slice must not affect future All()
	// calls — All returns a copy.
	r := script.New()
	_ = r.Register(script.Entry{PackID: "core", Path: "scripts/a.lua"})
	first := r.All()
	first[0].PackID = "MUTATED"
	second := r.All()
	if second[0].PackID == "MUTATED" {
		t.Error("All() returned shared slice — mutation leaked back to registry")
	}
}
