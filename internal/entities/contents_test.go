package entities

import (
	"sync"
	"testing"
)

func TestContents_PutAndIn(t *testing.T) {
	t.Parallel()
	c := NewContents()
	c.Put("bag", "gem-1")
	c.Put("bag", "gem-2")
	got := c.In("bag")
	if len(got) != 2 || got[0] != "gem-1" || got[1] != "gem-2" {
		t.Errorf("In(bag) = %v, want [gem-1 gem-2]", got)
	}
	con, ok := c.ContainerOf("gem-1")
	if !ok || con != "bag" {
		t.Errorf("ContainerOf(gem-1) = (%q, %v), want (bag, true)", con, ok)
	}
}

func TestContents_PutIsTotal_MovesAcrossContainers(t *testing.T) {
	t.Parallel()
	c := NewContents()
	c.Put("bag", "gem")
	c.Put("chest", "gem")
	if got := c.In("bag"); len(got) != 0 {
		t.Errorf("bag still contains %v after move", got)
	}
	if got := c.In("chest"); len(got) != 1 || got[0] != "gem" {
		t.Errorf("chest = %v, want [gem]", got)
	}
}

func TestContents_PutSameContainerIsNoOp(t *testing.T) {
	t.Parallel()
	c := NewContents()
	c.Put("bag", "gem")
	c.Put("bag", "gem")
	if got := c.In("bag"); len(got) != 1 {
		t.Errorf("In(bag) = %v, want single entry", got)
	}
}

func TestContents_Take(t *testing.T) {
	t.Parallel()
	c := NewContents()
	c.Put("bag", "gem")
	if !c.Take("gem") {
		t.Fatal("Take returned false for tracked item")
	}
	if _, ok := c.ContainerOf("gem"); ok {
		t.Error("ContainerOf still resolves after Take")
	}
	if got := c.In("bag"); len(got) != 0 {
		t.Errorf("In(bag) = %v, want empty after Take", got)
	}
	if c.Take("gem") {
		t.Error("Take of untracked item returned true")
	}
}

func TestContents_EmptyBucketPruned(t *testing.T) {
	t.Parallel()
	c := NewContents()
	c.Put("bag", "gem")
	c.Take("gem")
	// After draining, In must hand back nil so callers don't see a
	// phantom empty slice for a container nothing is in.
	if got := c.In("bag"); got != nil {
		t.Errorf("In(empty bag) = %v, want nil", got)
	}
}

// TestContents_ConcurrentPutTake exercises the mutex under -race.
// Twenty workers each cycling put/take on disjoint item ids should
// converge without lost updates and without the race detector
// firing.
func TestContents_ConcurrentPutTake(t *testing.T) {
	t.Parallel()
	c := NewContents()
	const workers, ops = 20, 50

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		w := w
		go func() {
			defer wg.Done()
			item := EntityID(string(rune('a'+w)) + "-item")
			for i := 0; i < ops; i++ {
				c.Put("bag", item)
				_ = c.In("bag")
				c.Take(item)
			}
		}()
	}
	wg.Wait()

	if got := c.In("bag"); len(got) != 0 {
		t.Errorf("In(bag) = %v after full drain, want empty", got)
	}
}
