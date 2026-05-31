package telnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// GmcpHandler is the inbound dispatch callback for non-Core.Supports
// GMCP packages. Set via Conn.SetGmcpHandler; nil leaves inbound
// packages discarded (with a debug log).
//
// pkg is the dotted package name (e.g. "Char.Vitals"). payload is
// the raw UTF-8 JSON bytes that followed the first space in the
// frame; absent payload arrives as nil. Handlers MUST NOT mutate
// payload (the negotiator reuses its subneg buffer).
type GmcpHandler func(ctx context.Context, pkg string, payload []byte)

// gmcpState is the per-connection GMCP machinery. Mutated by the
// negotiator (single-goroutine read loop) and by SetGmcpHandler /
// SendGmcp from arbitrary goroutines; mu guards everything except
// the atomic active flag.
type gmcpState struct {
	mu sync.Mutex
	// active goes true when the client sends DO GMCP (spec §5.5).
	// Until then SendGmcp is a silent no-op.
	active bool
	// supports is the package set the client declared via
	// Core.Supports.Set / Add. Empty map AFTER an explicit Set
	// means "client supports nothing"; nil map means "client
	// hasn't sent Supports yet" — permissive default per §5.3.
	supports map[string]struct{}
	// supportsReceived is true once any Core.Supports.Set has
	// arrived. Distinguishes "never declared" (permissive) from
	// "declared empty" (deny-everything).
	supportsReceived bool
	// handler dispatches inbound packages other than
	// Core.Supports.*. nil is legal — the negotiator logs and
	// drops.
	handler GmcpHandler
}

// newGmcpState returns a fresh state. Always non-nil so the
// negotiator can read fields without nil-checking.
func newGmcpState() *gmcpState {
	return &gmcpState{}
}

// activate marks the connection GMCP-active (spec §5.5). Called
// from the negotiator when DO GMCP arrives. Idempotent.
func (g *gmcpState) activate() {
	g.mu.Lock()
	g.active = true
	g.mu.Unlock()
}

// deactivate marks the connection GMCP-inactive. Called on
// DONT GMCP from the client. The supports set is intentionally
// preserved across deactivation so a client that re-negotiates
// doesn't lose its package list.
func (g *gmcpState) deactivate() {
	g.mu.Lock()
	g.active = false
	g.mu.Unlock()
}

// isActive reports whether GMCP is currently negotiated.
func (g *gmcpState) isActive() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active
}

// supportsPackage implements the spec §5.3 match rule: an exact
// key match OR a dotted-prefix ancestor (`Char` covers `Char.Vitals`).
// Until the client has sent any Core.Supports the default is
// permissive (every package matches).
func (g *gmcpState) supportsPackage(name string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.supportsReceived {
		return true
	}
	if _, ok := g.supports[name]; ok {
		return true
	}
	for key := range g.supports {
		if strings.HasPrefix(name, key+".") {
			return true
		}
	}
	return false
}

// applyCoreSupportsSet replaces the supports set with the names in
// arr (spec §5.3). The `<name> <version>` form is split on the
// first space; only the name half is stored.
func (g *gmcpState) applyCoreSupportsSet(arr []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.supports = make(map[string]struct{}, len(arr))
	g.supportsReceived = true
	for _, s := range arr {
		if name := packageNameFromSupportsEntry(s); name != "" {
			g.supports[name] = struct{}{}
		}
	}
}

// applyCoreSupportsAdd merges the names in arr into the existing
// supports set. Calling Add before any Set marks the set as
// received (the client has expressed an explicit preference) and
// the permissive default ends.
func (g *gmcpState) applyCoreSupportsAdd(arr []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.supports == nil {
		g.supports = make(map[string]struct{}, len(arr))
	}
	g.supportsReceived = true
	for _, s := range arr {
		if name := packageNameFromSupportsEntry(s); name != "" {
			g.supports[name] = struct{}{}
		}
	}
}

// applyCoreSupportsRemove removes the names in arr from the
// supports set. The version field (if any) is ignored per spec.
// Removing from an absent set is a no-op; the permissive default
// remains in effect.
func (g *gmcpState) applyCoreSupportsRemove(arr []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.supports == nil {
		return
	}
	for _, s := range arr {
		if name := packageNameFromSupportsEntry(s); name != "" {
			delete(g.supports, name)
		}
	}
}

// packageNameFromSupportsEntry returns the package-name half of
// a `Core.Supports.*` array entry. Entries are `<name>` or
// `<name> <version>`; we strip whitespace and the version.
func packageNameFromSupportsEntry(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	return s
}

// setHandler installs the inbound-package callback. Safe to call
// at any time; nil disables dispatch.
func (g *gmcpState) setHandler(h GmcpHandler) {
	g.mu.Lock()
	g.handler = h
	g.mu.Unlock()
}

// dispatch invokes the handler for a non-Core.Supports inbound
// package. Reads the handler under the lock to publish-coherent
// writes from setHandler, then calls outside the lock to keep the
// callback free of the negotiator's serialization.
func (g *gmcpState) dispatch(ctx context.Context, pkg string, payload []byte) {
	g.mu.Lock()
	h := g.handler
	g.mu.Unlock()
	if h == nil {
		logging.From(ctx).Debug("telnet.gmcp: inbound package dropped (no handler)",
			slog.String("package", pkg),
			slog.Int("payload_len", len(payload)))
		return
	}
	h(ctx, pkg, payload)
}

// ErrGmcpNotActive is returned by SendGmcp when GMCP has not been
// negotiated. Distinct from a write error so callers can
// distinguish "transport refused" from "I/O failed."
var ErrGmcpNotActive = errors.New("telnet.gmcp: not active")

// SendGmcp emits a GMCP package frame to the peer per spec §5.1.
// Wire shape: IAC SB GMCP <pkg> SPACE <json> IAC SE, with IAC bytes
// in the payload doubled. The payload MAY be nil (absent payload
// is read as JSON null).
//
// Behavior:
//   - returns ErrGmcpNotActive (no I/O) when the peer hasn't
//     activated GMCP via DO GMCP;
//   - returns nil silently when the peer's Core.Supports set
//     excludes pkg (no I/O), so callers can hand every event to
//     SendGmcp without checking SupportsPackage first;
//   - otherwise serializes the frame and writes it through the
//     underlying Conn write mutex.
//
// Safe for concurrent callers.
func (c *Conn) SendGmcp(ctx context.Context, pkg string, payload []byte) error {
	g := c.gmcp
	if !g.isActive() {
		return ErrGmcpNotActive
	}
	if !g.supportsPackage(pkg) {
		return nil
	}
	frame := buildGmcpFrame(pkg, payload)
	if _, err := c.WriteCommand(ctx, frame); err != nil {
		return fmt.Errorf("telnet.SendGmcp: %w", err)
	}
	return nil
}

// SupportsPackage exposes the spec §5.3 match check so callers can
// short-circuit expensive payload construction when the peer
// hasn't subscribed. nil-payload callers can skip the check and
// let SendGmcp drop the frame.
func (c *Conn) SupportsPackage(name string) bool {
	return c.gmcp.supportsPackage(name)
}

// SetGmcpHandler installs (or replaces) the inbound GMCP package
// callback. Pass nil to detach. Safe to call at any time including
// before the connection has activated GMCP — pre-activation
// frames are impossible anyway because peers don't send GMCP
// until they've negotiated it themselves.
func (c *Conn) SetGmcpHandler(h GmcpHandler) {
	c.gmcp.setHandler(h)
}

// GmcpActive reports whether the connection has negotiated GMCP.
// Mirrors the spec §5.5 query the engine uses to decide whether
// to skip a panel update.
func (c *Conn) GmcpActive() bool {
	return c.gmcp.isActive()
}

// buildGmcpFrame serializes one GMCP frame:
// IAC SB GMCP <pkg> SPACE <payload-with-IAC-doubled> IAC SE.
//
// IAC doubling protects against the (rare) JSON payload containing
// a literal 0xFF byte — spec §3.5 IAC-escaping rule applies to
// subneg payloads too. Pre-allocates the worst case (every byte
// doubled) so a payload-heavy frame doesn't reallocate.
func buildGmcpFrame(pkg string, payload []byte) []byte {
	// Worst case: every payload byte is 0xFF → doubled.
	out := make([]byte, 0, 5+len(pkg)+1+len(payload)*2+2)
	out = append(out, negIAC, negSB, optGMCP)
	out = append(out, pkg...)
	if len(payload) > 0 {
		out = append(out, ' ')
		for _, b := range payload {
			if b == negIAC {
				out = append(out, negIAC, negIAC)
			} else {
				out = append(out, b)
			}
		}
	}
	out = append(out, negIAC, negSE)
	return out
}

// parseGmcpFrame splits the payload of an IAC SB GMCP ... IAC SE
// frame into (package, payload). The wire format is
// "<package> <json>" with a single space delimiter; an absent
// payload (no space, no JSON) returns nil payload (the spec §5.1
// "read as JSON null" case).
//
// Caller MUST NOT mutate the returned payload — it aliases the
// negotiator's subneg buffer (IAC-IAC escapes have already been
// collapsed back to single 0xFF bytes by feed()).
func parseGmcpFrame(raw []byte) (pkg string, payload []byte) {
	idx := bytes.IndexByte(raw, ' ')
	if idx < 0 {
		return string(raw), nil
	}
	pkg = string(raw[:idx])
	if idx+1 >= len(raw) {
		return pkg, nil
	}
	return pkg, raw[idx+1:]
}

// handleGmcpSubneg is the negotiator entrypoint for an inbound
// SB GMCP frame. Routes Core.Supports.* to the per-state set
// handlers and everything else to the engine callback.
func (g *gmcpState) handleSubneg(ctx context.Context, payload []byte) {
	pkg, body := parseGmcpFrame(payload)
	switch pkg {
	case "Core.Supports.Set":
		arr, ok := decodeStringArray(body)
		if !ok {
			logging.From(ctx).Debug("telnet.gmcp: malformed Core.Supports.Set", slog.Int("len", len(body)))
			return
		}
		g.applyCoreSupportsSet(arr)
	case "Core.Supports.Add":
		arr, ok := decodeStringArray(body)
		if !ok {
			logging.From(ctx).Debug("telnet.gmcp: malformed Core.Supports.Add", slog.Int("len", len(body)))
			return
		}
		g.applyCoreSupportsAdd(arr)
	case "Core.Supports.Remove":
		arr, ok := decodeStringArray(body)
		if !ok {
			logging.From(ctx).Debug("telnet.gmcp: malformed Core.Supports.Remove", slog.Int("len", len(body)))
			return
		}
		g.applyCoreSupportsRemove(arr)
	default:
		// Copy the payload — the negotiator buffer is reused on
		// the next frame, and the callback may run beyond this
		// stack frame.
		var dup []byte
		if len(body) > 0 {
			dup = append([]byte(nil), body...)
		}
		g.dispatch(ctx, pkg, dup)
	}
}

// decodeStringArray parses a JSON array of strings. Returns false
// on parse error or any non-string element. Tolerates nil / empty
// body by returning an empty slice (a client sending
// `Core.Supports.Set` with no payload is legitimately "subscribe
// to nothing").
func decodeStringArray(body []byte) ([]string, bool) {
	if len(body) == 0 {
		return nil, true
	}
	var arr []string
	if err := json.Unmarshal(body, &arr); err != nil {
		return nil, false
	}
	return arr, true
}
