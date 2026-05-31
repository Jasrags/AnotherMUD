package telnet

// Telnet option codes the M16.1 negotiator handles. Defined here so
// the negotiator's switch table reads as names rather than literal
// hex. Numbers match the IANA telnet-options registry / the RFCs
// cited.
const (
	// optTTYPE — RFC 1091. Terminal-type negotiation. Server sends
	// IAC DO TTYPE; client responds WILL TTYPE; server then sends
	// the subneg IAC SB TTYPE SEND IAC SE; client replies
	// IAC SB TTYPE IS <ascii> IAC SE.
	optTTYPE byte = 24

	// optNAWS — RFC 1073. Negotiate About Window Size. Server
	// sends IAC DO NAWS; client responds WILL NAWS and starts
	// sending IAC SB NAWS w1 w0 h1 h0 IAC SE on every resize.
	optNAWS byte = 31
)

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
}
