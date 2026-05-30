package notifications

import (
	"fmt"
	"sort"
)

// Queue is a per-entity bounded priority queue. Entries are kept in
// drain order (highest Priority first; ties broken by earliest
// PublishedAt). The zero value is not usable — call NewQueue.
//
// Queue is NOT safe for concurrent use. The Manager (M13.1c) owns
// per-entity locking. Documented here so a future caller does not
// reach for a *Queue directly under contention.
//
// Spec: docs/specs/notifications.md §3, §6.1, §7.
type Queue struct {
	items []Notification
	cap   int
}

// AppendResult describes the outcome of an Append call.
//
// Evicted is non-nil when the queue was at cap and a lower-or-equal
// priority entry was dropped to make room. Refused is true when the
// queue was at cap and every entry had strictly higher priority than
// the new one — nothing could be dropped, so the new entry is
// rejected. Evicted and Refused are mutually exclusive.
type AppendResult struct {
	Evicted *Notification
	Refused bool
}

// NewQueue returns a queue with the given capacity. Cap of zero is
// valid but degenerate: every Append is refused.
func NewQueue(cap int) *Queue {
	if cap < 0 {
		cap = 0
	}
	return &Queue{cap: cap}
}

// Len returns the number of held notifications.
func (q *Queue) Len() int { return len(q.items) }

// Cap returns the configured capacity.
func (q *Queue) Cap() int { return q.cap }

// Append inserts n into the queue at its sorted position. If the
// queue would exceed cap, the oldest entry at the lowest-priority
// tier present is evicted — but only when at least one entry has
// strictly lower priority than n (so higher-priority entries are
// never sacrificed for lower-priority arrivals). If no such entry
// exists, the append is refused.
//
// Spec: docs/specs/notifications.md §6.1.
func (q *Queue) Append(n Notification) AppendResult {
	if q.cap == 0 {
		return AppendResult{Refused: true}
	}

	if len(q.items) < q.cap {
		q.insertSorted(n)
		return AppendResult{}
	}

	// At cap. The lowest-priority entry in the queue is the *last*
	// element because items are sorted drain-order (highest first).
	// Eviction is allowed when that lowest-tier entry is at or below
	// the incoming priority — i.e., we never evict a strictly
	// higher-priority entry to make room for a lower-priority one.
	tail := q.items[len(q.items)-1]
	if tail.Priority > n.Priority {
		return AppendResult{Refused: true}
	}

	// Among entries at the tail's tier, evict the OLDEST (earliest
	// PublishedAt), not necessarily the literal last element. The
	// sort breaks ties by PublishedAt ascending, so within a tier
	// the oldest sits *first* among that tier's run, not last.
	evictIdx := q.oldestAtTier(tail.Priority)
	evicted := q.items[evictIdx]
	q.items = append(q.items[:evictIdx], q.items[evictIdx+1:]...)

	q.insertSorted(n)
	return AppendResult{Evicted: &evicted}
}

// DrainAll returns every held notification in priority order (FIFO
// within tier) and empties the queue.
//
// Spec: docs/specs/notifications.md §7.
func (q *Queue) DrainAll() []Notification {
	if len(q.items) == 0 {
		return nil
	}
	out := q.items
	q.items = nil
	return out
}

// Snapshot returns a copy of the held notifications in priority
// order without mutating the queue. Used by the persistence layer
// (M13.1b) to write to disk without disturbing live state.
func (q *Queue) Snapshot() []Notification {
	if len(q.items) == 0 {
		return nil
	}
	out := make([]Notification, len(q.items))
	copy(out, q.items)
	return out
}

// Restore replaces the queue contents with items. Used by the
// persistence layer to rehydrate a saved queue at process start.
// Items are sorted into drain order; the caller does not need to
// pre-sort. Restore fails if items exceed the configured cap.
func (q *Queue) Restore(items []Notification) error {
	if len(items) > q.cap {
		return fmt.Errorf("restore: %d items exceeds cap %d", len(items), q.cap)
	}
	dup := make([]Notification, len(items))
	copy(dup, items)
	sort.SliceStable(dup, func(i, j int) bool {
		return drainLess(dup[i], dup[j])
	})
	q.items = dup
	return nil
}

// insertSorted assumes len(items) < cap. It places n at the unique
// position that keeps drain ordering intact.
func (q *Queue) insertSorted(n Notification) {
	idx := sort.Search(len(q.items), func(i int) bool {
		return drainLess(n, q.items[i])
	})
	q.items = append(q.items, Notification{})
	copy(q.items[idx+1:], q.items[idx:])
	q.items[idx] = n
}

// oldestAtTier returns the index of the earliest-PublishedAt entry
// whose Priority == tier. Caller guarantees at least one such entry
// exists. Because items are sorted by (priority desc, publishedAt
// asc), the first index in a tier's run is the oldest.
func (q *Queue) oldestAtTier(tier Priority) int {
	for i, n := range q.items {
		if n.Priority == tier {
			return i
		}
	}
	// Caller-side invariant: tail tier must exist when this is
	// called. If it doesn't, the eviction logic is wrong; panic
	// rather than silently corrupt.
	panic(fmt.Sprintf("notifications: oldestAtTier: tier %s absent in queue of %d", tier, len(q.items)))
}

// drainLess reports whether a should drain before b. Higher
// Priority drains first; within a tier, earlier PublishedAt drains
// first.
func drainLess(a, b Notification) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	return a.PublishedAt.Before(b.PublishedAt)
}
