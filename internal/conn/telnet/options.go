package telnet

import (
	"github.com/Jasrags/AnotherMUD/internal/mssp"
	"github.com/Jasrags/AnotherMUD/internal/render"
)

// Telnet option codes the negotiator handles. Defined here so
// the negotiator's switch table reads as names rather than literal
// hex. Numbers match the IANA telnet-options registry / the RFCs
// cited.
const (
	// optEcho — RFC 857. Server-side echo control. The login flow
	// (internal/login) drives this option OUT OF BAND via
	// Conn.WriteCommand to mask the password prompt: IAC WILL ECHO
	// suppresses the client's local echo, IAC WONT ECHO restores it.
	// The negotiator does not initiate ECHO; it only needs to NOT
	// contradict the login flow — so a client's IAC DO ECHO reply is
	// acknowledged silently rather than refused with WONT (which would
	// re-enable local echo and leak the password in cleartext).
	optEcho byte = 1

	// optTTYPE — RFC 1091. Terminal-type negotiation. Server sends
	// IAC DO TTYPE; client responds WILL TTYPE; server then sends
	// the subneg IAC SB TTYPE SEND IAC SE; client replies
	// IAC SB TTYPE IS <ascii> IAC SE.
	optTTYPE byte = 24

	// optNAWS — RFC 1073. Negotiate About Window Size. Server
	// sends IAC DO NAWS; client responds WILL NAWS and starts
	// sending IAC SB NAWS w1 w0 h1 h0 IAC SE on every resize.
	optNAWS byte = 31

	// optMSSP — MUD Server Status Protocol (M16.2). Server may
	// advertise WILL MSSP at boot; crawlers send DO MSSP; server
	// replies once with IAC SB MSSP <vars> IAC SE then has no
	// further session-long work (spec §8.4). The conn's mssp
	// config (set via WithMssp) provides the variable table; a
	// conn without one refuses DO MSSP with WONT.
	optMSSP byte = 70

	// optGMCP — Generic MUD Communication Protocol (M16.3).
	// Server advertises WILL GMCP at boot; client responds
	// DO GMCP to activate. After activation both sides exchange
	// IAC SB GMCP <utf-8 package + space + json> IAC SE frames.
	// Spec networking-protocols §5.
	optGMCP byte = 201
)

// Option is the functional-options shape for telnet.New. Each
// option mutates a fresh Conn at construction; safe to apply in
// any order. Defined here at the same site as the option codes
// so the surface stays discoverable.
type Option func(*Conn)

// WithMssp attaches an MSSP config to a new connection. The
// negotiator's IAC DO MSSP handler reads through this pointer to
// build the subneg payload; a nil/missing config makes the
// handler refuse with WONT MSSP. The config is shared across
// every connection that gets the same Option — the composition
// root typically builds one and threads it through Server.
func WithMssp(cfg *mssp.Config) Option {
	return func(c *Conn) { c.mssp = cfg }
}

// TTYPE subnegotiation sub-commands (RFC 1091 §4).
const (
	ttypeIS   byte = 0
	ttypeSEND byte = 1
)

// Capabilities is the per-connection client-capability snapshot
// produced by the IAC negotiator. All fields are zero-valued
// before negotiation completes (or for transports that don't
// speak telnet at all, like test fakes); callers that read them
// MUST tolerate the zero state.
//
// TerminalTypes preserves the response ORDER from RFC 1091's
// "query twice (or more) to walk a client's TTYPE rotation"
// pattern. Most clients (Mudlet, MUSHclient, TinTin++) return
// the same value on every query, so the slice is typically
// length 1. Some clients rotate through terminal-name → MTTS
// hex → display-name; the first one is the most-specific
// client identifier we got.
//
// Width / Height come from the latest NAWS subneg. Clients
// re-emit NAWS on every resize, so these reflect the current
// window. Zero means "client never sent NAWS" or "client
// refused the option" — renderers should fall back to a
// configured default.
type Capabilities struct {
	TerminalTypes []string
	Width         int
	Height        int

	// ClientName is the most-specific client identifier received via
	// TTYPE — the first entry of TerminalTypes, normalized. Empty
	// when no TTYPE was received. Used by the IsMudClient match
	// and by the ColorSupport tier derivation.
	ClientName string

	// IsMudClient reports whether ClientName matches the
	// known-MUD-client allowlist (spec §7.2). Drives:
	//   - Echo policy (§7.3): MUD clients handle their own echo,
	//     so server-side echo defaults to off for them.
	//   - Color tier (§7.2): a known MUD client gets Extended
	//     color even without a TRUECOLOR/256COLOR TTYPE hint.
	IsMudClient bool

	// ColorSupport is the derived color tier (spec §7.2). The
	// canonical type lives in internal/render so the rendering
	// consumer (M16.6b) doesn't drag a telnet import; telnet
	// re-derives it at snapshot time from TTYPE + IsMudClient.
	ColorSupport render.ColorTier
}
