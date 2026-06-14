package telnettest

import "io"

// Telnet IAC (Interpret As Command) control bytes — RFC 854. The engine
// negotiates options (echo, etc.) at connect and inline, so the raw byte
// stream is peppered with these sequences; a test client that matched against
// the raw stream would trip over the 0xFF command bytes.
const (
	iac  = 255 // Interpret As Command
	se   = 240 // end of subnegotiation
	sb   = 250 // begin subnegotiation
	will = 251
	wont = 252
	doo  = 253 // DO
	dont = 254
)

// iacReader wraps an io.Reader and strips telnet IAC command sequences, yielding
// only the data octets a human would see. It is a read-only filter: it does NOT
// answer negotiation. Our engine proceeds without a reply, so a test client that
// simply ignores DO/WILL works; responding is a possible future addition. An
// escaped data byte (IAC IAC) is emitted as a single 0xFF.
//
// Parse state is preserved across Read calls, so a sequence split over two
// underlying reads is handled correctly.
type iacReader struct {
	src   io.Reader
	state iacState
	scrat []byte // reused buffer for raw reads
	out   []byte // clean bytes not yet handed to the caller
}

type iacState int

const (
	stNormal iacState = iota
	stIAC             // saw IAC
	stOption          // saw IAC WILL/WONT/DO/DONT — expect the option byte
	stSub             // inside IAC SB … (subnegotiation payload)
	stSubIAC          // inside SB, saw IAC — expect SE or an escaped IAC
)

func newIACReader(src io.Reader) *iacReader {
	return &iacReader{src: src, scrat: make([]byte, 4096)}
}

// Read returns clean (IAC-stripped) data bytes. It loops over the underlying
// reader when a read yields only command bytes, so the caller never sees a
// spurious (0, nil) for pure negotiation. On an underlying error with no
// buffered clean bytes, the error is returned; buffered clean bytes are always
// drained first.
func (r *iacReader) Read(p []byte) (int, error) {
	for len(r.out) == 0 {
		n, err := r.src.Read(r.scrat)
		if n > 0 {
			r.process(r.scrat[:n])
		}
		if err != nil {
			if len(r.out) == 0 {
				return 0, err
			}
			break
		}
		if n == 0 {
			// Underlying reader returned (0, nil): pass it through rather than
			// spinning.
			return 0, nil
		}
	}
	n := copy(p, r.out)
	r.out = r.out[n:]
	return n, nil
}

// process runs the IAC state machine over a raw chunk, appending data bytes to
// r.out and dropping command bytes.
func (r *iacReader) process(b []byte) {
	for _, c := range b {
		switch r.state {
		case stNormal:
			if c == iac {
				r.state = stIAC
			} else {
				r.out = append(r.out, c)
			}
		case stIAC:
			switch c {
			case iac:
				r.out = append(r.out, iac) // escaped literal 0xFF
				r.state = stNormal
			case will, wont, doo, dont:
				r.state = stOption
			case sb:
				r.state = stSub
			default:
				r.state = stNormal // two-byte command (GA, NOP, …): drop
			}
		case stOption:
			r.state = stNormal // drop the option byte
		case stSub:
			if c == iac {
				r.state = stSubIAC
			}
			// else: drop subnegotiation payload
		case stSubIAC:
			switch c {
			case se:
				r.state = stNormal
			case iac:
				r.state = stSub // escaped IAC inside SB: drop
			default:
				r.state = stSub
			}
		}
	}
}
