package session

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/render"
)

// FlushPrompts renders and sends a fresh prompt to every playing session
// that has content owed since its last prompt (session-lifecycle §3.5).
// Called at the end of every tick by the game loop so a prompt appears
// after content settles, never mid-stream on raw input echo.
//
// Link-dead sessions are skipped (their connection is gone). Flow /
// prompt-mode skips from the spec are N/A until character-creation
// (M12) introduces those input modes.
func (m *Manager) FlushPrompts(ctx context.Context) {
	m.mu.RLock()
	snapshot := make([]*connActor, 0, len(m.byConn))
	for _, a := range m.byConn {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		if a.isLinkDead() {
			continue
		}
		if err := a.flushPrompt(ctx); err != nil {
			logging.From(ctx).Debug("FlushPrompts: write failed",
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
		}
	}
}

// flushPrompt renders the actor's prompt and writes it on its own line
// when a refresh is owed. It bypasses connActor.Write so it does not
// re-arm needsPromptRefresh (which would loop every tick) and does not
// append a trailing newline (the cursor should rest after the prompt).
func (a *connActor) flushPrompt(ctx context.Context) error {
	a.mu.Lock()
	if !a.needsPromptRefresh {
		a.mu.Unlock()
		return nil
	}
	a.needsPromptRefresh = false
	a.promptDisplayed = true
	var template string
	if a.save != nil {
		template = a.save.PromptTemplate
	}
	a.mu.Unlock()

	text := render.RenderPrompt(template, a.promptVitals())
	// Leading CR-LF so the prompt sits on its own line beneath whatever
	// content triggered the refresh; no trailing newline so the player's
	// input echoes right after it.
	_, err := a.conn.Write(ctx, []byte("\r\n"+a.render(text)))
	return err
}

// promptVitals snapshots the values the prompt tokens read. HP comes
// from the combat Vitals; mana/movement report their max stats (thin
// pools — no current-pool tracking until M11, so current == max). Gold
// is 0 until economy-survival (M11) adds currency.
func (a *connActor) promptVitals() render.PromptVitals {
	var hp, maxHP int
	if a.vitals != nil {
		hp, maxHP = a.vitals.Snapshot()
	}
	// statBlock is always set in production (actor construction); guard
	// nil so the prompt path doesn't panic on minimal test actors.
	var mana, mv int
	if a.statBlock != nil {
		mana = a.Mana()
		mv = a.Movement()
	}
	return render.PromptVitals{
		HP:      hp,
		MaxHP:   maxHP,
		Mana:    mana,
		MaxMana: mana,
		MV:      mv,
		MaxMV:   mv,
		Gold:    0,
	}
}
