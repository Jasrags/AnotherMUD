// Package queststore persists per-player quest state to disk and loads
// it on login (quests.md §6). It implements quest.Persister (Save) and
// provides Load, which reads the file, applies orphan filtering, and
// hands the state back to the caller to install into the quest service.
//
// File layout mirrors the player store: <root>/players/<lowercase
// name>/quests.yaml. State is keyed in memory by player id, but the file
// path is keyed by name, so the store caches the id→name mapping
// (populated on Load at login) to resolve Save's path.
package queststore

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/persistence"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// Store reads and writes per-player quest files under a players root.
type Store struct {
	root     string // <saveDir>/players
	registry *quest.Registry
	logger   *slog.Logger

	mu    sync.RWMutex
	names map[string]string // playerID → canonical name (for Save's path)
}

// NewStore returns a store rooted at <saveDir>/players. The registry is
// consulted for orphan filtering on load.
func NewStore(saveDir string, registry *quest.Registry) *Store {
	return &Store{
		root:     filepath.Join(saveDir, "players"),
		registry: registry,
		logger:   slog.Default(),
		names:    make(map[string]string),
	}
}

// WithLogger sets the logger used by Save (which has no ctx to carry one).
func (s *Store) WithLogger(l *slog.Logger) *Store {
	if l != nil {
		s.logger = l
	}
	return s
}

// path returns the quests.yaml path for a canonical name, guarding
// against traversal.
func (s *Store) path(canonName string) (string, error) {
	dir, err := persistence.SafeJoin(s.root, canonName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "quests.yaml"), nil
}

// Save writes the player's full quest state (quest.Persister, §6.2). The
// path is resolved from the id→name cache populated by Load at login; a
// Save for an uncached player (never logged in this process) is logged
// and skipped. The implementation serializes synchronously and does not
// retain the *State pointer.
func (s *Store) Save(playerID string, state *quest.State) {
	s.mu.RLock()
	name, ok := s.names[playerID]
	s.mu.RUnlock()
	if !ok {
		s.logger.Warn("quest save skipped: no cached name for player",
			slog.String("event", "quest.save.skip"),
			slog.String("entity_id", playerID))
		return
	}
	p, err := s.path(name)
	if err != nil {
		s.logger.Warn("quest save: bad path",
			slog.String("event", "quest.save.err"),
			slog.String("entity_id", playerID), slog.Any("err", err))
		return
	}
	data, err := yaml.Marshal(toFile(state))
	if err != nil {
		s.logger.Warn("quest save: marshal failed",
			slog.String("event", "quest.save.err"),
			slog.String("entity_id", playerID), slog.Any("err", err))
		return
	}
	if err := persistence.AtomicWrite(p, data); err != nil {
		s.logger.Warn("quest save: write failed",
			slog.String("event", "quest.save.err"),
			slog.String("entity_id", playerID), slog.Any("err", err))
	}
}

// Load reads and orphan-filters the player's quest state (§6.3). It
// always caches the id→name mapping (so a later Save resolves its path
// even when the player has no quests file yet) and returns (state, true)
// when a file was read, or (nil, false) when the file is missing or
// unreadable — errors are logged, never propagated (§6.3).
func (s *Store) Load(ctx context.Context, playerID, name string) (*quest.State, bool) {
	canon := player.CanonicalName(name)
	s.mu.Lock()
	s.names[playerID] = canon
	s.mu.Unlock()

	log := logging.From(ctx)
	p, err := s.path(canon)
	if err != nil {
		log.Warn("quest load: bad path",
			slog.String("event", "quest.load.err"),
			slog.String("entity_id", playerID), slog.Any("err", err))
		return nil, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warn("quest load: read failed",
				slog.String("event", "quest.load.err"),
				slog.String("entity_id", playerID), slog.Any("err", err))
		}
		return nil, false
	}
	var f questFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		log.Warn("quest load: parse failed",
			slog.String("event", "quest.load.err"),
			slog.String("entity_id", playerID), slog.Any("err", err))
		return nil, false
	}
	state := f.toState()
	s.orphanFilter(state)
	return state, true
}

// Forget drops the cached id→name mapping on logout.
func (s *Store) Forget(playerID string) {
	s.mu.Lock()
	delete(s.names, playerID)
	s.mu.Unlock()
}

// orphanFilter drops active and completed entries whose quest id is no
// longer in the registry (§6.4). When the registry is empty — typically
// a startup-order issue, not real content removal — filtering is skipped
// so a player's history is not wiped.
func (s *Store) orphanFilter(st *quest.State) {
	if s.registry.Len() == 0 {
		return
	}
	keptActive := st.Active[:0]
	for _, a := range st.Active {
		if _, ok := s.registry.Lookup(a.QuestID); ok {
			keptActive = append(keptActive, a)
		}
	}
	st.Active = keptActive

	keptCompleted := st.Completed[:0]
	for _, id := range st.Completed {
		if _, ok := s.registry.Lookup(id); ok {
			keptCompleted = append(keptCompleted, id)
		}
	}
	st.Completed = keptCompleted
}
