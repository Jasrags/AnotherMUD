package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Character-creation completion pipeline (spec character-creation §6.4).
// M12.2 lands the commit half of the pipeline: a new character's entity
// is assembled in run() (race/class/alignment/sustenance seeded) but not
// persisted until commitCreation runs, so a disconnect before this point
// leaves nothing on disk (§8). The interactive wizard that sits between
// entity assembly and commit — plus restart-on-validation-failure (§7)
// and input routing (§4) — lands in M12.3; M12.2 exercises the §2
// "no flow registered → immediate commit" path.

// ErrNameConflict is the §6.4 last-chance name collision: another
// new-player commit persisted the same canonical name while this one was
// in creation. The session writes a message and closes the connection.
var ErrNameConflict = errors.New("session: character name taken at commit")

// creationCommitMu serializes the §6.4 commit step across all
// connections so two concurrent new-player commits for the same name
// cannot both pass the Exists re-check and write. A single process-wide
// mutex is the right grain: commits are rare (once per character ever)
// and the critical section is a stat + two small file writes.
var creationCommitMu sync.Mutex

// commitCreation runs the spec §6.4 commit for a freshly-created
// character: under the commit mutex it re-checks the canonical name is
// still free, then persists the assembled save and links it to the
// owning account. It does NOT place the entity in the world or flip the
// session phase — run() does that next (so the existing Add / events /
// render path stays in one place). Returns ErrNameConflict on a
// last-chance collision (nothing is written in that case).
//
// The save passed in is the fully-assembled baseline (name, account,
// location, plus the race/class/alignment/sustenance the seeds wrote);
// persisting here is the first time the character touches disk.
func commitCreation(ctx context.Context, cfg Config, a *connActor) error {
	name := a.Name()

	creationCommitMu.Lock()
	defer creationCommitMu.Unlock()

	// §6.4 step 1: last-chance name conflict. The soft pre-check in
	// login.Run can race a concurrent new-player who committed first;
	// this re-check under the mutex is authoritative.
	if cfg.Players != nil && cfg.Players.Exists(name) {
		return ErrNameConflict
	}

	// §6.4 step 2: persist the new character (records the owning
	// account id alongside the entity state via Save's AccountID field).
	a.mu.Lock()
	save := snapshotSave(a.save)
	a.mu.Unlock()
	persisted := false
	if cfg.Players != nil {
		if err := cfg.Players.Save(ctx, &save); err != nil {
			return fmt.Errorf("commit creation: save: %w", err)
		}
		persisted = true
	}
	// Link the character name to its account (deferred from login). A
	// failure here leaves the character file written but unlinked; the
	// player can still log in (returningPlayer loads by name), so we log
	// and continue rather than abort a committed character.
	if cfg.Login.Accounts != nil {
		if err := cfg.Login.Accounts.AddCharacter(ctx, a.accountID, name); err != nil {
			logging.From(ctx).Warn("commit creation: link character to account failed",
				slog.String("name", name),
				slog.String("account_id", a.accountID),
				slog.Any("err", err))
		}
	}

	// Clear the save dirty bit ONLY if we actually wrote the snapshot:
	// the on-disk state now matches, so the next autosave needn't rewrite
	// it. When Players is nil (tests) nothing was written, so leaving the
	// bit set keeps the in-memory character eligible for a later autosave.
	if persisted {
		a.mu.Lock()
		a.dirty = false
		a.mu.Unlock()
	}

	logging.From(ctx).Info("character created",
		slog.String("name", name),
		slog.String("account_id", a.accountID))
	return nil
}
