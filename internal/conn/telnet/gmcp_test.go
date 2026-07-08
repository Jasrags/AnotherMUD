package telnet

import (
	"strconv"
	"testing"
)

// makeNames returns n synthetic package names "pkg-0", "pkg-1", …
// for the cap tests. Distinct values so every insert is a real
// growth attempt (not a no-op re-add).
func makeNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "pkg-" + strconv.Itoa(i)
	}
	return out
}

func TestGmcpSupportsCap_SetDropsPastLimit(t *testing.T) {
	// A Set with more than maxSupportsEntries truncates at the cap
	// and reports the dropped count. Existing entries are
	// discarded by Set semantics, so the dropped count equals
	// (input - cap).
	g := newGmcpState()
	dropped := g.applyCoreSupportsSet(makeNames(maxSupportsEntries + 17))

	if dropped != 17 {
		t.Errorf("dropped = %d, want 17", dropped)
	}
	g.mu.Lock()
	size := len(g.supports)
	g.mu.Unlock()
	if size != maxSupportsEntries {
		t.Errorf("size after Set = %d, want %d", size, maxSupportsEntries)
	}
}

func TestGmcpSupportsCap_AddStopsWhenFull(t *testing.T) {
	// Fill to the cap via Set, then Add more — every new entry
	// counts as dropped, existing ones stay.
	g := newGmcpState()
	if dropped := g.applyCoreSupportsSet(makeNames(maxSupportsEntries)); dropped != 0 {
		t.Fatalf("setup: Set should not drop at the cap exactly: dropped=%d", dropped)
	}

	extras := []string{"extra-a", "extra-b", "extra-c"}
	dropped := g.applyCoreSupportsAdd(extras)
	if dropped != len(extras) {
		t.Errorf("dropped = %d, want %d", dropped, len(extras))
	}
	// Cap still respected.
	g.mu.Lock()
	size := len(g.supports)
	g.mu.Unlock()
	if size != maxSupportsEntries {
		t.Errorf("size after Add = %d, want %d", size, maxSupportsEntries)
	}
	// Confirm none of the extras leaked in.
	for _, name := range extras {
		if g.supportsPackage(name) {
			t.Errorf("extra %q leaked past cap", name)
		}
	}
}

func TestGmcpSupportsCap_AddIgnoresRedundantNamesAgainstCap(t *testing.T) {
	// Re-adding names that are already present is a no-op and
	// must NOT count against the cap or get reported as dropped.
	// Verifies the duplicate-check sits before the cap-check.
	g := newGmcpState()
	g.applyCoreSupportsSet(makeNames(maxSupportsEntries))

	// Re-add the first 50 names that are already present.
	already := makeNames(50)
	if dropped := g.applyCoreSupportsAdd(already); dropped != 0 {
		t.Errorf("re-adding present names reported %d dropped, want 0", dropped)
	}
}

func TestGmcpSupportsCap_RepeatedAddCallsBoundedAcrossInvocations(t *testing.T) {
	// The attack model the cap defends against: a peer that sends
	// many small Add calls in succession. Each call is bounded
	// (subneg payload cap), but without the per-state cap the map
	// would grow unboundedly across calls. Verify that after 10
	// Add rounds of 100 distinct names each (1000 attempts total),
	// the map still respects the cap.
	g := newGmcpState()
	totalDropped := 0
	for round := range 10 {
		// Each round adds 100 fresh names.
		names := make([]string, 100)
		for i := range names {
			names[i] = "round-" + strconv.Itoa(round) + "-pkg-" + strconv.Itoa(i)
		}
		totalDropped += g.applyCoreSupportsAdd(names)
	}
	g.mu.Lock()
	size := len(g.supports)
	g.mu.Unlock()
	if size > maxSupportsEntries {
		t.Errorf("repeated Add grew past cap: size = %d, max = %d", size, maxSupportsEntries)
	}
	want := 1000 - maxSupportsEntries
	if totalDropped != want {
		t.Errorf("totalDropped = %d, want %d (1000 attempts - %d cap)",
			totalDropped, want, maxSupportsEntries)
	}
}
