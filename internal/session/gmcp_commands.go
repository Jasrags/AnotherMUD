package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// SetCommandCatalog pre-builds the two role tiers of the Char.Commands catalog
// (ui-rendering-help §10.4) from the command registry and stores them for the
// emit-once push. Called once at startup from the composition root, after the
// builtins are registered. The catalog is static per role for the engine's
// lifetime, so it is marshalled here rather than per-flush. A nil registry
// no-ops; a marshal failure (not expected for this shape) is logged and leaves
// that tier unset, which safely disables its push.
func (m *Manager) SetCommandCatalog(ctx context.Context, r *command.Registry, adminRole string) {
	if r == nil {
		return
	}
	player, err := marshalCommandCatalog(r.Catalog(false))
	if err != nil {
		logging.From(ctx).Warn("gmcp char.commands: player catalog marshal failed", slog.Any("err", err))
	}
	admin, err := marshalCommandCatalog(r.Catalog(true))
	if err != nil {
		logging.From(ctx).Warn("gmcp char.commands: admin catalog marshal failed", slog.Any("err", err))
	}
	m.mu.Lock()
	m.cmdCatalogPlayer = player
	m.cmdCatalogAdmin = admin
	m.cmdCatalogAdminRole = adminRole
	m.mu.Unlock()
}

// marshalCommandCatalog projects the command-package catalog into the gmcp
// payload shape and marshals it. Returns (nil, err) on a marshal failure so the
// caller can log it and treat the tier as unset.
func marshalCommandCatalog(cats []command.CatalogCategory) ([]byte, error) {
	payload := gmcp.CharCommands{Categories: make([]gmcp.CharCommandCategory, 0, len(cats))}
	for _, c := range cats {
		cmds := make([]gmcp.CharCommand, 0, len(c.Commands))
		for _, cc := range c.Commands {
			cmds = append(cmds, gmcp.CharCommand{
				Keyword: cc.Keyword,
				// Briefs may carry color markup; strip it so a graphical menu
				// renders text, not escape codes (same treatment as room labels).
				Brief:  gmcpPlain(cc.Brief),
				Syntax: cc.Syntax,
			})
		}
		payload.Categories = append(payload.Categories, gmcp.CharCommandCategory{
			Key:      c.Key,
			Title:    c.Title,
			Commands: cmds,
		})
	}
	return json.Marshal(payload)
}

// FlushGmcpCommands walks every live session and emits the Char.Commands catalog
// to any that haven't received it yet (emit-once, like Char.StatusVars). Called
// once per simulation tick from the gmcp-commands-flush handler; the per-actor
// sent flag makes it a cheap no-op after the first push.
func (m *Manager) FlushGmcpCommands(ctx context.Context) {
	m.mu.RLock()
	player, admin, adminRole := m.cmdCatalogPlayer, m.cmdCatalogAdmin, m.cmdCatalogAdminRole
	if player == nil && admin == nil {
		m.mu.RUnlock()
		return
	}
	snapshot := make([]*connActor, 0, len(m.byConn))
	for _, a := range m.byConn {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		if a.isLinkDead() {
			continue
		}
		a.flushGmcpCommands(ctx, player, admin, adminRole)
	}
}

// flushGmcpCommands ships the actor's role-appropriate command catalog once.
// Silent no-op when the conn doesn't speak GMCP, GMCP isn't negotiated, the
// catalog was already sent, or the chosen tier is unset. An actor holding the
// admin role gets the admin-inclusive catalog; everyone else gets the player
// tier — mirroring the bare-help index's role gate.
func (a *connActor) flushGmcpCommands(ctx context.Context, player, admin []byte, adminRole string) {
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}

	payload := player
	if adminRole != "" && a.HasRole(adminRole) {
		payload = admin
	}
	if payload == nil {
		return
	}

	a.gmcpCommandsMu.Lock()
	if a.gmcpCommandsSent {
		a.gmcpCommandsMu.Unlock()
		return
	}
	a.gmcpCommandsSent = true
	a.gmcpCommandsMu.Unlock()

	if err := sender.SendGmcp(ctx, gmcp.PackageCharCommands, payload); err != nil {
		logging.From(ctx).Debug("gmcp char.commands send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// resetGmcpCommandsShadow clears the sent flag so the next flush re-emits the
// catalog. Called on link-dead reattach: the new peer's menu needs a baseline
// even though the catalog itself is unchanged.
func (a *connActor) resetGmcpCommandsShadow() {
	a.gmcpCommandsMu.Lock()
	a.gmcpCommandsSent = false
	a.gmcpCommandsMu.Unlock()
}
