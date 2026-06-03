package session

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/conn"
)

// installCharMode wires server-side character-at-a-time Tab completion
// (tab-completion Phase 2) on a connection that supports it (telnet). It
// installs the completion provider and, for RAW clients (those that did
// not negotiate GMCP), enables char-mode by default — GMCP clients use the
// better Phase 1 Input.Complete path, and WS has no char-mode. Called at
// pump start (post-login), so login/password stay line-mode.
func installCharMode(ctx context.Context, c conn.Connection, a *connActor, cfg Config) {
	cm, ok := c.(conn.CharModeConn)
	if !ok {
		return // ws / non-char-mode transport
	}
	cm.SetCompletionProvider(func(pctx context.Context, line string) conn.Completion {
		return toConnCompletion(cfg.Commands.CompleteLine(commandEnv(cfg), a, line))
	})
	// Route each client to its best surface: a GMCP-capable client keeps
	// line-mode (it has Input.Complete); only raw telnet defaults to
	// char-mode. `tabcomplete on/off` overrides either way.
	if g, ok := c.(interface{ GmcpActive() bool }); ok && g.GmcpActive() {
		return
	}
	cm.SetCharMode(ctx, true)
}

// toConnCompletion maps a command.CompletionResult onto the transport-
// neutral conn.Completion the char-mode editor consumes, computing the
// longest-common-prefix the editor completes to (§12).
func toConnCompletion(res command.CompletionResult) conn.Completion {
	items := make([]conn.CompletionItem, 0, len(res.Candidates))
	values := make([]string, 0, len(res.Candidates))
	for _, cand := range res.Candidates {
		items = append(items, conn.CompletionItem{Value: cand.Completion, Display: cand.Display})
		values = append(values, cand.Completion)
	}
	return conn.Completion{Common: command.LongestCommonPrefix(values), Candidates: items}
}

// SetCharMode toggles server-side char-mode editing on the actor's
// connection (command.CharModeController). Returns false when the
// transport doesn't support it (ws, test actors).
func (a *connActor) SetCharMode(ctx context.Context, on bool) bool {
	cm, ok := a.conn.(conn.CharModeConn)
	if !ok {
		return false
	}
	cm.SetCharMode(ctx, on)
	return true
}

// CharModeActive reports whether char-mode editing is on
// (command.CharModeController).
func (a *connActor) CharModeActive() bool {
	if cm, ok := a.conn.(conn.CharModeConn); ok {
		return cm.CharModeActive()
	}
	return false
}
