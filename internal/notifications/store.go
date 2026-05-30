package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/persistence"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// Store persists per-entity notification queues alongside player
// data. Today the only persisted entities are players, so the on-
// disk layout is <root>/players/<canonical-name>/notifications.yaml
// — the same convention as quest state (see internal/queststore).
//
// Spec: docs/specs/notifications.md §6.3.
type Store struct {
	root   string // <saveDir>/players
	cap    int
	logger *slog.Logger

	mu    sync.RWMutex
	names map[string]string // entityID → canonical name (for Save's path)
}

// NewStore returns a store rooted under saveDir. Cap bounds the
// in-memory queue size produced by Load; files containing more
// entries are truncated to drain-order priority (highest-priority
// entries survive) and a warning is logged.
func NewStore(saveDir string, cap int) *Store {
	return &Store{
		root:   filepath.Join(saveDir, "players"),
		cap:    cap,
		logger: slog.Default(),
		names:  make(map[string]string),
	}
}

// WithLogger overrides the Save-path logger (Save has no ctx to
// carry one).
func (s *Store) WithLogger(l *slog.Logger) *Store {
	if l != nil {
		s.logger = l
	}
	return s
}

// path returns the notifications.yaml path for a canonical name,
// guarding against traversal.
func (s *Store) path(canonName string) (string, error) {
	dir, err := persistence.SafeJoin(s.root, canonName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "notifications.yaml"), nil
}

// Load reads the entity's persisted queue. A missing file is not
// an error — it represents "no backlog" and yields an empty queue.
// The id→name mapping is cached so a later Save resolves its path
// even if no file existed at load time.
//
// Errors that are *not* "file missing" (parse failures, unreadable
// path) are logged at warn level and an empty queue is returned;
// they are not propagated. This matches the queststore convention.
func (s *Store) Load(ctx context.Context, entityID, name string) (*Queue, error) {
	canon := player.CanonicalName(name)
	s.mu.Lock()
	s.names[entityID] = canon
	s.mu.Unlock()

	log := logging.From(ctx)
	q := NewQueue(s.cap)

	p, err := s.path(canon)
	if err != nil {
		log.Warn("notifications load: bad path",
			slog.String("event", "notify.load.err"),
			slog.String("entity_id", entityID),
			slog.Any("err", err))
		return q, nil
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return q, nil
		}
		log.Warn("notifications load: read failed",
			slog.String("event", "notify.load.err"),
			slog.String("entity_id", entityID),
			slog.Any("err", err))
		return q, nil
	}

	var f notificationFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		log.Warn("notifications load: parse failed",
			slog.String("event", "notify.load.err"),
			slog.String("entity_id", entityID),
			slog.Any("err", err))
		return q, nil
	}

	items := make([]Notification, 0, len(f.Entries))
	for _, e := range f.Entries {
		pri, ok := parsePriority(e.Priority)
		if !ok {
			log.Warn("notifications load: unknown priority skipped",
				slog.String("event", "notify.load.unknown_priority"),
				slog.String("entity_id", entityID),
				slog.String("id", e.ID),
				slog.String("priority", e.Priority))
			continue
		}
		items = append(items, Notification{
			ID:          e.ID,
			Recipients:  append([]string(nil), e.Recipients...),
			Priority:    pri,
			Kind:        e.Kind,
			Text:        e.Text,
			PublishedAt: e.PublishedAt,
			Sender:      e.Sender,
		})
	}

	// If the file has more entries than the cap, keep the highest-
	// priority ones (drain-order is sorted in Restore; we truncate
	// after sort by routing through Append, which preserves
	// highest-priority survival when at cap).
	if len(items) > s.cap {
		log.Warn("notifications load: file exceeds cap, truncating",
			slog.String("event", "notify.load.truncated"),
			slog.String("entity_id", entityID),
			slog.Int("file_count", len(items)),
			slog.Int("cap", s.cap))
		// Route through Append so the same eviction logic that
		// guards live publishes also governs the load path. The
		// file is sorted by drain order on disk; appending in
		// that order with cap enforcement keeps the most
		// important entries.
		for _, n := range items {
			q.Append(n)
		}
		return q, nil
	}

	if err := q.Restore(items); err != nil {
		// Restore only fails on cap overflow; we just checked.
		// Treat as corruption — empty queue, warn loudly.
		log.Warn("notifications load: restore failed",
			slog.String("event", "notify.load.err"),
			slog.String("entity_id", entityID),
			slog.Any("err", err))
		return NewQueue(s.cap), nil
	}

	return q, nil
}

// Save writes the entity's queue snapshot to disk. The path is
// resolved from the entityID→name cache populated by Load. A Save
// for an uncached entity (never logged in this process) is logged
// and skipped — same posture as queststore.
//
// An empty queue writes an empty-entries file (rather than
// deleting); the loader treats empty file and missing file
// identically (§6.3).
func (s *Store) Save(entityID string, q *Queue) error {
	s.mu.RLock()
	name, ok := s.names[entityID]
	s.mu.RUnlock()
	if !ok {
		s.logger.Warn("notifications save skipped: no cached name",
			slog.String("event", "notify.save.skip"),
			slog.String("entity_id", entityID))
		return nil
	}

	p, err := s.path(name)
	if err != nil {
		s.logger.Warn("notifications save: bad path",
			slog.String("event", "notify.save.err"),
			slog.String("entity_id", entityID),
			slog.Any("err", err))
		return fmt.Errorf("notifications save: %w", err)
	}

	data, err := yaml.Marshal(toFile(q.Snapshot()))
	if err != nil {
		s.logger.Warn("notifications save: marshal failed",
			slog.String("event", "notify.save.err"),
			slog.String("entity_id", entityID),
			slog.Any("err", err))
		return fmt.Errorf("notifications save: marshal: %w", err)
	}

	if err := persistence.AtomicWrite(p, data); err != nil {
		s.logger.Warn("notifications save: write failed",
			slog.String("event", "notify.save.err"),
			slog.String("entity_id", entityID),
			slog.Any("err", err))
		return fmt.Errorf("notifications save: write: %w", err)
	}
	return nil
}

// Forget drops the cached entityID→name mapping. Call on logout.
func (s *Store) Forget(entityID string) {
	s.mu.Lock()
	delete(s.names, entityID)
	s.mu.Unlock()
}
