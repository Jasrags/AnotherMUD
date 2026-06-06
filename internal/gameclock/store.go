package gameclock

// Global in-game-time persistence (light-and-darkness §7, resolving
// time-and-clock §3.6). The saved time is a single world-global
// artifact — one clock for the world, written alongside other global
// state (cf. the channel-scrollback store), never part of any player
// save. It exists so darkness, which now gates gameplay, does not
// reset the world to night on every restart.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/persistence"
	"gopkg.in/yaml.v3"
)

// ClockFileName is the global clock artifact, written at the save-dir
// root next to the accounts/ and players/ subtrees.
const ClockFileName = "clock.yaml"

// SavedTime is the persisted shape of the in-game clock: the hour of
// day and the running day count. Sub-hour position is not preserved
// (light-and-darkness §7) — a restart resumes at the start of the
// saved hour.
type SavedTime struct {
	CurrentHour int    `yaml:"current_hour"`
	DayCount    uint64 `yaml:"day_count"`
}

// Store reads and writes the global SavedTime artifact using the same
// atomic tmp→bak→rename rotation every other store uses.
type Store struct {
	path string
}

// NewStore builds a Store rooted at saveDir; the artifact is
// saveDir/clock.yaml.
func NewStore(saveDir string) *Store {
	return &Store{path: filepath.Join(saveDir, ClockFileName)}
}

// Load returns the saved time and true when a valid artifact exists.
// A missing file (fresh world), an unreadable/corrupt file, or an
// out-of-range hour all return ok=false so the caller cold-starts at
// the documented initial state (time-and-clock §3.5: hour 0, day 0)
// rather than crashing. Only genuinely unexpected read/parse failures
// are logged; an absent file is the normal first-boot path and stays
// quiet.
func (s *Store) Load(ctx context.Context) (SavedTime, bool) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logging.From(ctx).Warn("gameclock.load: read failed, cold-starting",
				slog.String("path", s.path), slog.String("err", err.Error()))
		}
		return SavedTime{}, false
	}
	var st SavedTime
	if err := yaml.Unmarshal(data, &st); err != nil {
		logging.From(ctx).Warn("gameclock.load: corrupt clock file, cold-starting",
			slog.String("path", s.path), slog.String("err", err.Error()))
		return SavedTime{}, false
	}
	if st.CurrentHour < 0 || st.CurrentHour > 23 {
		logging.From(ctx).Warn("gameclock.load: hour out of range, cold-starting",
			slog.Int("hour", st.CurrentHour))
		return SavedTime{}, false
	}
	return st, true
}

// Save atomically writes st to disk. Callers flush on a bounded
// cadence (hour advance and/or autosave) plus a final flush at
// shutdown, so an unclean stop loses at most that cadence of in-game
// time.
func (s *Store) Save(st SavedTime) error {
	data, err := yaml.Marshal(st)
	if err != nil {
		return fmt.Errorf("gameclock save: marshal: %w", err)
	}
	if err := persistence.AtomicWrite(s.path, data); err != nil {
		return fmt.Errorf("gameclock save: write: %w", err)
	}
	return nil
}
