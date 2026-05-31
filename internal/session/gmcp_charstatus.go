package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// charStatusVarCatalogue is the static var→caption map shipped in
// every Char.StatusVars frame. Lives at package scope because the
// catalogue never changes during the engine's lifetime — every
// session sees the same vars. Keys match the JSON field names in
// gmcp.CharStatus so a client can correlate "catalogue says
// `alignment` exists" with "Char.Status carries `alignment`".
var charStatusVarCatalogue = map[string]string{
	"race":          "Race",
	"class":         "Class",
	"alignment":     "Alignment",
	"alignment_tag": "Alignment Bucket",
}

// FlushGmcpCharStatus walks every live session and emits the
// boot-identity GMCP packages: Char.Login (emit-once), Char.
// StatusVars (emit-once), and Char.Status (poll-and-diff). Called
// once per simulation tick from the gmcp-charstatus-flush handler.
//
// The login + vars packages are bundled with the poll-and-diff
// Char.Status emit so a single tick handler covers the whole
// identity surface. Idempotence is per-actor (sent flags) and
// per-snapshot (last-status shadow).
func (m *Manager) FlushGmcpCharStatus(ctx context.Context) {
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
		a.flushGmcpCharStatus(ctx)
	}
}

// flushGmcpCharStatus emits the three Char.* identity packages
// when their respective preconditions are met:
//
//   - Char.Login: first GMCP-active flush after login (or after
//     link-dead reattach reset the sent flag).
//   - Char.StatusVars: same trigger as Char.Login — clients pair
//     them.
//   - Char.Status: poll-and-diff per tick, same shape as Vitals.
//
// Silent no-op when the conn doesn't speak GMCP or GMCP hasn't
// been negotiated. The three "decide what to send" branches run
// under gmcpCharStatusMu so a concurrent reattach reset can't
// race the compare-and-set on the sent flags.
func (a *connActor) flushGmcpCharStatus(ctx context.Context) {
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}

	status := a.snapshotCharStatus()

	a.gmcpCharStatusMu.Lock()
	sendLogin := !a.gmcpLoginSent
	sendVars := !a.gmcpStatusVarsSent
	sendStatus := !a.gmcpLastStatusValid || a.gmcpLastStatus != status
	if sendLogin {
		a.gmcpLoginSent = true
	}
	if sendVars {
		a.gmcpStatusVarsSent = true
	}
	if sendStatus {
		a.gmcpLastStatus = status
		a.gmcpLastStatusValid = true
	}
	a.gmcpCharStatusMu.Unlock()

	if sendLogin {
		a.sendCharLogin(ctx, sender)
	}
	if sendVars {
		a.sendCharStatusVars(ctx, sender)
	}
	if sendStatus {
		a.sendCharStatus(ctx, sender, status)
	}
}

// snapshotCharStatus builds the runtime CharStatus payload from
// the actor's race, class, alignment, and alignment bucket tag.
// Reads through the existing accessors so race+class+alignment
// each take their own lock briefly — none of them nest.
func (a *connActor) snapshotCharStatus() gmcp.CharStatus {
	return gmcp.CharStatus{
		Race:         a.RaceID(),
		Class:        a.ClassID(),
		Alignment:    a.Alignment(),
		AlignmentTag: a.AlignmentTag(),
	}
}

// sendCharLogin marshals + ships one Char.Login frame. Today the
// engine carries no separate full-name surface, so `fullname`
// mirrors `name` — reserved for future title/honorific work.
func (a *connActor) sendCharLogin(ctx context.Context, sender gmcpSender) {
	name := a.Name()
	payload := gmcp.CharLogin{
		Name:     name,
		FullName: name,
		Account:  a.accountID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageCharLogin, data); err != nil {
		logging.From(ctx).Debug("gmcp char.login send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// sendCharStatusVars marshals + ships one Char.StatusVars frame
// carrying the static var catalogue. Catalogue is package-level
// state — every session sees the same map — so no per-actor
// snapshot is needed.
func (a *connActor) sendCharStatusVars(ctx context.Context, sender gmcpSender) {
	payload := gmcp.CharStatusVars{Vars: charStatusVarCatalogue}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageCharStatusVars, data); err != nil {
		logging.From(ctx).Debug("gmcp char.statusvars send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// sendCharStatus marshals + ships one Char.Status frame.
func (a *connActor) sendCharStatus(ctx context.Context, sender gmcpSender, status gmcp.CharStatus) {
	data, err := json.Marshal(status)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageCharStatus, data); err != nil {
		logging.From(ctx).Debug("gmcp char.status send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// resetGmcpCharStatusShadow clears all three sent flags + the
// status shadow so the next flushGmcpCharStatus call re-emits
// the full Char.Login + Char.StatusVars + Char.Status triple.
// Called on link-dead reattach: the new peer's panels need
// baseline identity frames even when the engine-side state
// hasn't changed across the drop.
func (a *connActor) resetGmcpCharStatusShadow() {
	a.gmcpCharStatusMu.Lock()
	a.gmcpLoginSent = false
	a.gmcpStatusVarsSent = false
	a.gmcpLastStatusValid = false
	a.gmcpCharStatusMu.Unlock()
}
