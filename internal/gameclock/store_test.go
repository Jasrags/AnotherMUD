package gameclock_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/gameclock"
)

func TestStore_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := gameclock.NewStore(dir)

	want := gameclock.SavedTime{CurrentHour: 14, DayCount: 7}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok := store.Load(context.Background())
	if !ok {
		t.Fatal("Load: ok=false after Save, want true")
	}
	if got != want {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
}

func TestStore_LoadMissingColdStarts(t *testing.T) {
	store := gameclock.NewStore(t.TempDir())
	got, ok := store.Load(context.Background())
	if ok {
		t.Fatalf("Load on missing file: ok=true, want false (got %+v)", got)
	}
	if got != (gameclock.SavedTime{}) {
		t.Fatalf("Load on missing file = %+v, want zero", got)
	}
}

func TestStore_LoadCorruptColdStarts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, gameclock.ClockFileName), []byte("::not yaml::\n\t- ["), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	store := gameclock.NewStore(dir)
	if _, ok := store.Load(context.Background()); ok {
		t.Fatal("Load on corrupt file: ok=true, want false")
	}
}

func TestStore_LoadOutOfRangeHourColdStarts(t *testing.T) {
	dir := t.TempDir()
	store := gameclock.NewStore(dir)
	// Hand off a value that round-trips structurally but is out of range.
	if err := store.Save(gameclock.SavedTime{CurrentHour: 25, DayCount: 3}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := store.Load(context.Background()); ok {
		t.Fatal("Load with hour=25: ok=true, want false (out of range)")
	}
}

func TestStore_LoadReadErrorColdStarts(t *testing.T) {
	// A clock.yaml that is a directory, not a file, makes os.ReadFile
	// return a non-ErrNotExist error — the defensive read-failed
	// branch. It must cold-start, not crash.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, gameclock.ClockFileName), 0o755); err != nil {
		t.Fatalf("seed dir-as-file: %v", err)
	}
	store := gameclock.NewStore(dir)
	if _, ok := store.Load(context.Background()); ok {
		t.Fatal("Load when clock.yaml is a directory: ok=true, want false")
	}
}

func TestStore_SaveWriteErrorReturnsError(t *testing.T) {
	// Root the store under a path whose parent is a regular file, so
	// AtomicWrite's MkdirAll cannot create the target directory. Save
	// must surface a wrapped error rather than silently succeeding.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker file: %v", err)
	}
	store := gameclock.NewStore(filepath.Join(blocker, "nested"))
	if err := store.Save(gameclock.SavedTime{CurrentHour: 1}); err == nil {
		t.Fatal("Save into unwritable path: err=nil, want error")
	}
}

func TestNew_SeedsFromSavedTime(t *testing.T) {
	// Default boundaries [5,8,18,20]: hour 14 → Day.
	c := gameclock.New(gameclock.Config{InitialHour: 14, InitialDay: 9})
	if got := c.CurrentHour(); got != 14 {
		t.Fatalf("CurrentHour = %d, want 14", got)
	}
	if got := c.DayCount(); got != 9 {
		t.Fatalf("DayCount = %d, want 9", got)
	}
	if got := c.CurrentPeriod(); got != gameclock.PeriodDay {
		t.Fatalf("CurrentPeriod = %q, want %q", got, gameclock.PeriodDay)
	}
}

func TestNew_SeedClampsOutOfRangeHour(t *testing.T) {
	c := gameclock.New(gameclock.Config{InitialHour: -3})
	if got := c.CurrentHour(); got != 0 {
		t.Fatalf("CurrentHour = %d, want 0 (clamped)", got)
	}
}

func TestSnapshot_MatchesAccessors(t *testing.T) {
	c := gameclock.New(gameclock.Config{InitialHour: 20, InitialDay: 2})
	snap := c.Snapshot()
	if snap.CurrentHour != c.CurrentHour() || snap.DayCount != c.DayCount() {
		t.Fatalf("Snapshot %+v disagrees with accessors (hour=%d day=%d)",
			snap, c.CurrentHour(), c.DayCount())
	}
}
