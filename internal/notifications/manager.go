package notifications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Sink is the per-entity delivery surface. Session code implements
// Sink by writing the notification text to the connected client.
// Implementations MUST NOT block long; they should hand the text to
// the underlying writer and return. A returned error causes the
// Manager to re-enqueue the notification at its original priority
// and PublishedAt (spec §5).
type Sink interface {
	Deliver(ctx context.Context, n Notification) error
}

// Manager is the per-entity notification routing layer. It owns
// in-memory state for currently-tracked entities (logged-in players)
// and routes publishes through to immediate delivery or persisted
// enqueue. For offline recipients it loads / appends / saves via the
// Store synchronously.
//
// Spec: docs/specs/notifications.md §4-§7.
type Manager struct {
	store *Store
	cap   int
	clock clock.Clock

	mu    sync.Mutex
	state map[string]*entityState // entityID → state

	// fallbackSeq disambiguates IDs minted on the crypto/rand
	// failure path. Combined with clock.Now() it guarantees per-
	// process uniqueness without the rand failure cascading into
	// a silent ID collision.
	fallbackSeq atomic.Uint64
}

// entityState holds the in-memory queue + bound sink for a tracked
// entity. When sink is nil the entity is offline-but-still-tracked
// (e.g., link-dead). Removed from state entirely after Unregister
// flushes the queue.
type entityState struct {
	queue *Queue
	sink  Sink
	name  string // canonical save-key name
	dirty bool   // queue mutated since last save
}

// NewManager returns a Manager. cap bounds the per-entity queue
// size; clk is used to stamp PublishedAt at publish time.
func NewManager(store *Store, cap int, clk clock.Clock) *Manager {
	return &Manager{
		store: store,
		cap:   cap,
		clock: clk,
		state: make(map[string]*entityState),
	}
}

// Register loads the entity's persisted queue (if any), binds the
// delivery sink, and caches the canonical name for later Save calls.
// Call when a session enters the active phase.
//
// Spec: docs/specs/notifications.md §7.
func (m *Manager) Register(ctx context.Context, entityID, name string, sink Sink) error {
	if entityID == "" {
		return fmt.Errorf("notifications.Register: empty entityID")
	}
	if sink == nil {
		return fmt.Errorf("notifications.Register: nil sink")
	}

	q, err := m.store.Load(ctx, entityID, name)
	if err != nil {
		return fmt.Errorf("notifications.Register: load: %w", err)
	}

	m.mu.Lock()
	m.state[entityID] = &entityState{
		queue: q,
		sink:  sink,
		name:  name,
		dirty: false,
	}
	m.mu.Unlock()
	return nil
}

// Unregister flushes any dirty queue to disk and drops the entity
// from in-memory state. Call on logout / full session teardown.
// A subsequent Publish to this entity will go through the load-
// append-save path (offline routing).
func (m *Manager) Unregister(ctx context.Context, entityID string) error {
	m.mu.Lock()
	st, ok := m.state[entityID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.state, entityID)
	dirty := st.dirty
	// Snapshot under the lock so the bytes handed to Save cannot
	// tear if a concurrent goroutine raced our delete (e.g., a
	// Publish that captured st before we removed it).
	var snap []Notification
	if dirty {
		snap = st.queue.Snapshot()
	}
	m.mu.Unlock()

	if dirty {
		if err := m.store.Save(entityID, snap); err != nil {
			return fmt.Errorf("notifications.Unregister: save: %w", err)
		}
	}
	m.store.Forget(entityID)
	return nil
}

// Drain delivers every queued notification for the entity through
// its bound sink in priority order. If a Deliver call fails, the
// failing notification stays at the head of the queue (along with
// everything after it) and Drain returns the error. A successful
// drain leaves the queue empty.
//
// Spec: docs/specs/notifications.md §7.
func (m *Manager) Drain(ctx context.Context, entityID string) error {
	m.mu.Lock()
	st, ok := m.state[entityID]
	if !ok || st.sink == nil {
		m.mu.Unlock()
		return nil
	}
	pending := st.queue.DrainAll()
	st.dirty = st.dirty || len(pending) > 0
	sink := st.sink
	m.mu.Unlock()

	log := logging.From(ctx)
	for i, n := range pending {
		if err := sink.Deliver(ctx, n); err != nil {
			// Re-enqueue this notification and everything after
			// it (preserve original publish order on retry).
			m.mu.Lock()
			if st2, ok := m.state[entityID]; ok {
				for _, leftover := range pending[i:] {
					st2.queue.Append(leftover)
				}
			}
			m.mu.Unlock()
			log.Warn("notify drain: deliver failed",
				slog.String("event", "notify.drain.err"),
				slog.String("entity_id", entityID),
				slog.String("id", n.ID),
				slog.Any("err", err))
			return fmt.Errorf("notifications.Drain: %w", err)
		}
		log.Debug("notify drained",
			slog.String("event", "notify.drained"),
			slog.String("entity_id", entityID),
			slog.String("id", n.ID),
			slog.String("kind", n.Kind),
			slog.String("priority", n.Priority.String()))
	}
	return nil
}

// Publish routes a notification to its recipients. For each
// recipient ID present in routeNames:
//
//   - If the recipient is currently registered with an empty queue,
//     Deliver immediately. If Deliver fails, fall through to enqueue.
//   - If registered with a non-empty queue, append to the queue at
//     its priority position (Drain handles ordering).
//   - If not registered (offline), load-append-save through the
//     Store using routeNames[id] as the canonical save key.
//
// routeNames must contain a canonical-name entry for every
// recipient. An ID missing from routeNames is logged and skipped.
// Stamps PublishedAt from the clock and assigns an ID if empty.
//
// Returns the first non-skip error encountered; partial delivery
// is OK per spec §11.
func (m *Manager) Publish(ctx context.Context, n Notification, routeNames map[string]string) error {
	if n.ID == "" {
		n.ID = m.newID()
	}
	if n.PublishedAt.IsZero() {
		n.PublishedAt = m.clock.Now()
	}

	log := logging.From(ctx)
	var firstErr error
	for _, rid := range n.Recipients {
		name, ok := routeNames[rid]
		if !ok {
			log.Warn("notify publish: recipient has no route name",
				slog.String("event", "notify.publish.no_name"),
				slog.String("id", n.ID),
				slog.String("recipient", rid))
			continue
		}
		if err := m.routeOne(ctx, n, rid, name); err != nil {
			log.Warn("notify publish: per-recipient failure",
				slog.String("event", "notify.publish.failed"),
				slog.String("id", n.ID),
				slog.String("recipient", rid),
				slog.Any("err", err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// routeOne handles a single recipient. Tries immediate delivery
// for online+empty-queue; falls back to enqueue. For offline
// recipients goes through the Store.
func (m *Manager) routeOne(ctx context.Context, n Notification, entityID, name string) error {
	log := logging.From(ctx)

	m.mu.Lock()
	st, online := m.state[entityID]
	if online && st.sink != nil && st.queue.Len() == 0 {
		sink := st.sink
		m.mu.Unlock()
		if err := sink.Deliver(ctx, n); err == nil {
			log.Debug("notify delivered immediate",
				slog.String("event", "notify.deliver.immediate"),
				slog.String("entity_id", entityID),
				slog.String("id", n.ID),
				slog.String("kind", n.Kind))
			return nil
		}
		// Deliver failed; fall back to enqueue with original
		// priority/PublishedAt (spec §5).
		m.mu.Lock()
		st, online = m.state[entityID]
	}

	if online {
		res := st.queue.Append(n)
		st.dirty = true
		queueLen := st.queue.Len()
		m.mu.Unlock()
		m.logAppend(ctx, entityID, n, res, queueLen)
		return nil
	}
	m.mu.Unlock()

	// Offline: route through Store.AppendOne, which serialises
	// the load-append-save sequence per canonical name so two
	// concurrent publishes to the same offline recipient cannot
	// race and lose a message.
	res, queueLen, err := m.store.AppendOne(ctx, entityID, name, n)
	if err != nil {
		return err
	}
	m.logAppend(ctx, entityID, n, res, queueLen)
	return nil
}

func (m *Manager) logAppend(ctx context.Context, entityID string, n Notification, res AppendResult, queueSize int) {
	log := logging.From(ctx)
	if res.Refused {
		log.Warn("notify enqueue refused",
			slog.String("event", "notify.refused.cap"),
			slog.String("entity_id", entityID),
			slog.String("id", n.ID),
			slog.String("priority", n.Priority.String()))
		return
	}
	if res.Evicted != nil {
		log.Warn("notify queue evicted",
			slog.String("event", "notify.queue.evicted"),
			slog.String("entity_id", entityID),
			slog.String("evicted_id", res.Evicted.ID),
			slog.String("evicted_priority", res.Evicted.Priority.String()),
			slog.String("new_id", n.ID),
			slog.String("new_priority", n.Priority.String()))
	}
	log.Debug("notify enqueued",
		slog.String("event", "notify.enqueued"),
		slog.String("entity_id", entityID),
		slog.String("id", n.ID),
		slog.String("priority", n.Priority.String()),
		slog.Int("queue_size", queueSize))
}

// SaveAll flushes every dirty in-memory queue to disk. Called by
// the autosave tick handler. Per-entity errors are logged and
// isolated; one bad save does not abort the batch.
//
// Snapshots are taken under m.mu so the bytes handed to Save are
// stable even if a concurrent Publish / Drain mutates the live
// Queue between this collection and the per-entry Save call.
func (m *Manager) SaveAll(ctx context.Context) {
	m.mu.Lock()
	type entry struct {
		id   string
		snap []Notification
	}
	pending := make([]entry, 0, len(m.state))
	for id, st := range m.state {
		if st.dirty {
			pending = append(pending, entry{id: id, snap: st.queue.Snapshot()})
			st.dirty = false
		}
	}
	m.mu.Unlock()

	log := logging.From(ctx)
	for _, e := range pending {
		if err := m.store.Save(e.id, e.snap); err != nil {
			// Re-mark dirty so a subsequent SaveAll retries.
			// If the entity was Unregistered between the snapshot
			// and now, the in-memory state is gone — log a warn so
			// the data-loss case is observable rather than silent.
			m.mu.Lock()
			if st, ok := m.state[e.id]; ok {
				st.dirty = true
			} else {
				log.Warn("notify SaveAll: entity unregistered before save retry",
					slog.String("event", "notify.save.lost"),
					slog.String("entity_id", e.id))
			}
			m.mu.Unlock()
			log.Warn("notify SaveAll: per-entity save failed",
				slog.String("event", "notify.save.err"),
				slog.String("entity_id", e.id),
				slog.Any("err", err))
		}
	}
}

// newID returns an opaque per-process unique notification id.
// On the (extraordinarily rare) crypto/rand failure path the
// fallback combines the engine clock and an atomic per-Manager
// sequence number so concurrent failures still produce unique
// IDs — the previous implementation emitted the same zeroed-
// buffer string for every fallback caller, defeating dedup
// downstream.
func (m *Manager) newID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "n-" + hex.EncodeToString(buf[:])
	}
	seq := m.fallbackSeq.Add(1)
	return fmt.Sprintf("notif-fallback-%d-%d", m.clock.Now().UnixNano(), seq)
}
