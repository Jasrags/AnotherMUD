package telnet

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/mssp"
)

// pairConn returns an in-memory client + server conn pair using
// net.Pipe and the Conn wrapper. Closes both via the cleanup. The
// test writes to client and observes either the server-side Read
// or the bytes the server sends back (which appear on the client
// side of the pipe).
func pairConn(t *testing.T) (server *Conn, client net.Conn) {
	t.Helper()
	c1, c2 := net.Pipe()
	server = New("test-1", c1)
	t.Cleanup(func() {
		_ = server.Close()
		_ = c2.Close()
	})
	return server, c2
}

// readBytes reads up to want bytes from c (with deadline). Helps
// tests inspect the server's outbound IAC commands.
func readBytes(t *testing.T, c net.Conn, want int) []byte {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, want)
	n, err := io.ReadFull(c, buf)
	if err != nil {
		t.Fatalf("readBytes(%d): n=%d err=%v (got %x)", want, n, err, buf[:n])
	}
	return buf
}

func TestNegotiator_FirstReadSendsInitialOffers(t *testing.T) {
	server, client := pairConn(t)

	// Kick the server Read in a goroutine so the negotiator emits
	// its initial offers without blocking the test.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = server.Read(context.Background())
	}()

	// Expect: IAC DO TTYPE  IAC DO NAWS  (6 bytes total).
	got := readBytes(t, client, 6)
	want := []byte{negIAC, negDO, optTTYPE, negIAC, negDO, optNAWS}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("initial offer[%d] = %#x, want %#x (full: %x)", i, got[i], b, got)
		}
	}

	// Send a newline so the Read returns and the goroutine exits.
	_, _ = client.Write([]byte("\n"))
	<-done
}

func TestNegotiator_TTYPENegotiationCapturesName(t *testing.T) {
	server, client := pairConn(t)

	got := make(chan string, 1)
	go func() {
		line, _ := server.Read(context.Background())
		got <- line
	}()

	// Drain the initial offers (6 bytes).
	_ = readBytes(t, client, 6)

	// Client agrees: IAC WILL TTYPE.
	_, _ = client.Write([]byte{negIAC, negWILL, optTTYPE})

	// Server should now request the name: IAC SB TTYPE SEND IAC SE.
	subneg := readBytes(t, client, 6)
	want := []byte{negIAC, negSB, optTTYPE, ttypeSEND, negIAC, negSE}
	for i, b := range want {
		if subneg[i] != b {
			t.Errorf("ttype SEND[%d] = %#x, want %#x", i, subneg[i], b)
		}
	}

	// Client responds: IAC SB TTYPE IS "Mudlet" IAC SE.
	reply := []byte{negIAC, negSB, optTTYPE, ttypeIS}
	reply = append(reply, []byte("Mudlet")...)
	reply = append(reply, negIAC, negSE)
	_, _ = client.Write(reply)

	// Second SEND should fire (rotation query).
	_ = readBytes(t, client, 6)

	// Client returns same name → rotation wrap, no further send.
	_, _ = client.Write(reply)

	// Send a line of data; the server returns it. Capabilities
	// should have "Mudlet" captured.
	_, _ = client.Write([]byte("hello\n"))
	if line := <-got; line != "hello" {
		t.Errorf("Read = %q, want %q", line, "hello")
	}

	caps := server.Capabilities()
	if len(caps.TerminalTypes) != 1 || caps.TerminalTypes[0] != "Mudlet" {
		t.Errorf("TerminalTypes = %v, want [Mudlet]", caps.TerminalTypes)
	}
}

func TestNegotiator_TTYPERotationCapturesMultiple(t *testing.T) {
	server, client := pairConn(t)

	got := make(chan string, 1)
	go func() {
		line, _ := server.Read(context.Background())
		got <- line
	}()

	_ = readBytes(t, client, 6) // initial offers
	_, _ = client.Write([]byte{negIAC, negWILL, optTTYPE})

	// Client rotates: Mudlet → MTTS → Mudlet (wrap).
	for i, name := range []string{"Mudlet", "MTTS 2575", "Mudlet"} {
		_ = readBytes(t, client, 6) // SEND request
		reply := []byte{negIAC, negSB, optTTYPE, ttypeIS}
		reply = append(reply, []byte(name)...)
		reply = append(reply, negIAC, negSE)
		_, _ = client.Write(reply)
		_ = i
	}

	_, _ = client.Write([]byte("x\n"))
	<-got

	caps := server.Capabilities()
	if len(caps.TerminalTypes) != 2 ||
		caps.TerminalTypes[0] != "Mudlet" ||
		caps.TerminalTypes[1] != "MTTS 2575" {
		t.Errorf("TerminalTypes = %v, want [Mudlet, MTTS 2575]", caps.TerminalTypes)
	}
}

func TestNegotiator_NAWSCapturesWindowSize(t *testing.T) {
	server, client := pairConn(t)

	got := make(chan string, 1)
	go func() {
		line, _ := server.Read(context.Background())
		got <- line
	}()

	_ = readBytes(t, client, 6) // initial offers
	_, _ = client.Write([]byte{negIAC, negWILL, optNAWS})

	// Client emits NAWS unsolicited: IAC SB NAWS 0 80 0 24 IAC SE
	// (width=80, height=24 big-endian).
	naws := []byte{negIAC, negSB, optNAWS, 0, 80, 0, 24, negIAC, negSE}
	_, _ = client.Write(naws)

	_, _ = client.Write([]byte("hi\n"))
	<-got

	caps := server.Capabilities()
	if caps.Width != 80 || caps.Height != 24 {
		t.Errorf("Width/Height = %d/%d, want 80/24", caps.Width, caps.Height)
	}
}

func TestNegotiator_NAWSReemitUpdatesCapabilities(t *testing.T) {
	server, client := pairConn(t)

	got := make(chan string, 1)
	go func() {
		// Two reads — between them the client resizes.
		_, _ = server.Read(context.Background())
		line, _ := server.Read(context.Background())
		got <- line
	}()

	_ = readBytes(t, client, 6)
	_, _ = client.Write([]byte{negIAC, negWILL, optNAWS})

	_, _ = client.Write([]byte{negIAC, negSB, optNAWS, 0, 80, 0, 24, negIAC, negSE})
	_, _ = client.Write([]byte("first\n"))

	// Resize between reads.
	_, _ = client.Write([]byte{negIAC, negSB, optNAWS, 0, 120, 0, 40, negIAC, negSE})
	_, _ = client.Write([]byte("second\n"))
	<-got

	caps := server.Capabilities()
	if caps.Width != 120 || caps.Height != 40 {
		t.Errorf("post-resize Width/Height = %d/%d, want 120/40", caps.Width, caps.Height)
	}
}

func TestNegotiator_UnknownWILLOptionRefused(t *testing.T) {
	server, client := pairConn(t)

	go func() { _, _ = server.Read(context.Background()) }()
	_ = readBytes(t, client, 6)

	// Client offers an unknown option (LINEMODE = 34).
	_, _ = client.Write([]byte{negIAC, negWILL, 34})

	// Server should respond IAC DONT 34 to break the loop.
	got := readBytes(t, client, 3)
	want := []byte{negIAC, negDONT, 34}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("refusal[%d] = %#x, want %#x (full %x)", i, got[i], want[i], got)
		}
	}

	_, _ = client.Write([]byte("\n"))
}

func TestNegotiator_UnknownDOOptionRefused(t *testing.T) {
	server, client := pairConn(t)

	go func() { _, _ = server.Read(context.Background()) }()
	_ = readBytes(t, client, 6)

	// Client asks us to DO an option we don't offer (e.g. SUPPRESS-GO-AHEAD = 3).
	_, _ = client.Write([]byte{negIAC, negDO, 3})

	// Server should respond IAC WONT 3.
	got := readBytes(t, client, 3)
	want := []byte{negIAC, negWONT, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("refusal[%d] = %#x, want %#x (full %x)", i, got[i], want[i], got)
		}
	}

	_, _ = client.Write([]byte("\n"))
}

func TestNegotiator_IACBetweenLinesDoesNotPolluteRead(t *testing.T) {
	// Subneg + negotiation interleaved with line data must not
	// surface protocol bytes through Read. Drives the byte-level
	// state machine through a torture test of mid-line IAC.
	server, client := pairConn(t)

	got := make(chan string, 2)
	go func() {
		for i := 0; i < 2; i++ {
			line, _ := server.Read(context.Background())
			got <- line
		}
	}()

	_ = readBytes(t, client, 6) // initial offers

	// Send: "hel" + IAC WILL NAWS + "lo\n" + NAWS subneg + "world\n"
	_, _ = client.Write([]byte("hel"))
	_, _ = client.Write([]byte{negIAC, negWILL, optNAWS})
	_, _ = client.Write([]byte("lo\n"))
	_, _ = client.Write([]byte{negIAC, negSB, optNAWS, 0, 100, 0, 30, negIAC, negSE})
	_, _ = client.Write([]byte("world\n"))

	if line := <-got; line != "hello" {
		t.Errorf("line 1 = %q, want %q", line, "hello")
	}
	if line := <-got; line != "world" {
		t.Errorf("line 2 = %q, want %q", line, "world")
	}
	caps := server.Capabilities()
	if caps.Width != 100 || caps.Height != 30 {
		t.Errorf("Width/Height after mid-line NAWS = %d/%d, want 100/30",
			caps.Width, caps.Height)
	}
}

func TestNegotiator_EscapedIACInLineSurfacesAsData(t *testing.T) {
	// IAC IAC inside a data line decodes to one literal 0xFF byte.
	// Round-trip with Conn.Write's escaping (which also doubles
	// 0xFF) demonstrates symmetry: what Write sends, Read recovers.
	server, client := pairConn(t)

	got := make(chan string, 1)
	go func() {
		line, _ := server.Read(context.Background())
		got <- line
	}()

	_ = readBytes(t, client, 6)

	// Send doubled IAC mid-line.
	_, _ = client.Write([]byte{'a', negIAC, negIAC, 'b', '\n'})

	line := <-got
	if line != string([]byte{'a', 0xFF, 'b'}) {
		t.Errorf("escaped IAC line = %x, want a FF b", []byte(line))
	}
}

func TestNegotiator_SubnegBufferOverflowDiscardsBlock(t *testing.T) {
	// A peer streams an SB without an SE past MaxLineBytes. The
	// negotiator must drop the block (reset state) rather than
	// grow memory without bound. We then send a clean SB NAWS
	// after the dropped block to prove the state machine
	// recovered.
	server, client := pairConn(t)

	got := make(chan string, 1)
	go func() {
		line, _ := server.Read(context.Background())
		got <- line
	}()

	_ = readBytes(t, client, 6) // initial offers

	// IAC SB <fake-opt 200> + (MaxLineBytes+10) data bytes + NO IAC SE.
	header := []byte{negIAC, negSB, 200}
	if _, err := client.Write(header); err != nil {
		t.Fatalf("write header: %v", err)
	}
	junk := make([]byte, MaxLineBytes+10)
	for i := range junk {
		junk[i] = 'X'
	}
	if _, err := client.Write(junk); err != nil {
		t.Fatalf("write junk: %v", err)
	}

	// Recovery: SB NAWS 0 80 0 24 IAC SE then a data line.
	_, _ = client.Write([]byte{negIAC, negWILL, optNAWS})
	_, _ = client.Write([]byte{negIAC, negSB, optNAWS, 0, 80, 0, 24, negIAC, negSE})
	_, _ = client.Write([]byte("ok\n"))

	// The Read MUST surface a line (not hang). The exact contents
	// are an implementation detail of the overflow-recovery path:
	// once the SB buffer hits MaxLineBytes the block is discarded
	// and the state machine returns to Normal, so any bytes still
	// on the wire AFTER the overflow boundary flow through as data
	// — they end up suffixed by the "ok" the recovery line wrote.
	// The contract this test pins is "no hang, no memory leak, and
	// the post-recovery SB NAWS still dispatched cleanly."
	line := <-got
	if !strings.HasSuffix(line, "ok") {
		t.Errorf("post-overflow Read = %q, want suffix %q", line, "ok")
	}
	if caps := server.Capabilities(); caps.Width != 80 || caps.Height != 24 {
		t.Errorf("post-overflow Width/Height = %d/%d, want 80/24", caps.Width, caps.Height)
	}
}

func TestNegotiator_UnexpectedByteAfterIACInsideSBClosesBlock(t *testing.T) {
	// stateSBIAC's default branch: a byte that's neither SE nor IAC
	// arrives after IAC inside a subneg. Spec-compliant clients
	// never produce this, but the fallback path must close the
	// block gracefully rather than stall the state machine.
	server, client := pairConn(t)

	go func() { _, _ = server.Read(context.Background()) }()
	_ = readBytes(t, client, 6)

	// IAC SB NAWS 0 80 0 24 IAC <unexpected byte 0x42 'B'>.
	// The negotiator should treat this as end-of-block and
	// dispatch what it collected (which decodes as valid NAWS
	// since the four data bytes were captured before the IAC).
	_, _ = client.Write([]byte{negIAC, negWILL, optNAWS})
	_, _ = client.Write([]byte{negIAC, negSB, optNAWS, 0, 80, 0, 24, negIAC, 'B'})
	_, _ = client.Write([]byte("\n"))

	// Capabilities reflect that the SB dispatched (NAWS) even
	// though the closing byte was malformed.
	if caps := server.Capabilities(); caps.Width != 80 || caps.Height != 24 {
		t.Errorf("malformed-close Width/Height = %d/%d, want 80/24", caps.Width, caps.Height)
	}
}

func TestNegotiator_DONTForUnneededOptionIsSilent(t *testing.T) {
	// Peer sends DONT for an option we never enabled. Per Q-method
	// we silently acknowledge (no reply byte) — the wire after the
	// initial offers should carry exactly the bytes the peer sent
	// later, with no insertion from our side.
	server, client := pairConn(t)

	go func() { _, _ = server.Read(context.Background()) }()
	_ = readBytes(t, client, 6)

	// DONT for a random option (LINEMODE = 34). We never offered
	// to enable it — no reply expected.
	_, _ = client.Write([]byte{negIAC, negDONT, 34})

	// Now provoke an actual reply so we can verify the next byte
	// from the server is THAT reply, not a stray response to DONT.
	// Use unknown WILL: server must reply IAC DONT 33.
	_, _ = client.Write([]byte{negIAC, negWILL, 33})

	got := readBytes(t, client, 3)
	want := []byte{negIAC, negDONT, 33}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("next reply[%d] = %#x, want %#x (full %x)", i, got[i], want[i], got)
		}
	}

	_, _ = client.Write([]byte("\n"))
}

func TestNegotiator_DOMSSPEmitsSubnegWhenConfigured(t *testing.T) {
	// MSSP-enabled conn: a crawler sends DO MSSP and must receive
	// IAC SB MSSP <var-table> IAC SE in reply.
	c1, c2 := net.Pipe()
	cfg := &mssp.Config{
		Name:    "TestMUD",
		ANSI:    true,
		Players: func() int { return 7 },
	}
	server := New("mssp-1", c1, WithMssp(cfg))
	t.Cleanup(func() {
		_ = server.Close()
		_ = c2.Close()
	})

	go func() { _, _ = server.Read(context.Background()) }()
	_ = readBytes(t, c2, 6) // initial offers

	_, _ = c2.Write([]byte{negIAC, negDO, optMSSP})

	// Read the framing bytes and verify the wrapper.
	header := readBytes(t, c2, 3)
	if header[0] != negIAC || header[1] != negSB || header[2] != optMSSP {
		t.Fatalf("framing = %x, want IAC SB MSSP", header)
	}
	// Read until IAC SE.
	payload := readUntilIACSE(t, c2)
	if len(payload) == 0 {
		t.Fatal("empty MSSP payload")
	}
	// Spec §8.2 standard variables must be present. Smoke check a
	// couple of them; the full table is covered by mssp_test.go.
	if !bytes.Contains(payload, []byte("NAME")) {
		t.Errorf("payload missing NAME: %x", payload)
	}
	if !bytes.Contains(payload, []byte("PLAYERS")) {
		t.Errorf("payload missing PLAYERS: %x", payload)
	}

	_, _ = c2.Write([]byte("\n"))
}

func TestNegotiator_DOMSSPWithoutConfigRefused(t *testing.T) {
	// A conn without an mssp config refuses DO MSSP like any
	// other unsupported option.
	server, client := pairConn(t)

	go func() { _, _ = server.Read(context.Background()) }()
	_ = readBytes(t, client, 6)

	_, _ = client.Write([]byte{negIAC, negDO, optMSSP})

	got := readBytes(t, client, 3)
	want := []byte{negIAC, negWONT, optMSSP}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("refusal[%d] = %#x, want %#x (full %x)", i, got[i], want[i], got)
		}
	}

	_, _ = client.Write([]byte("\n"))
}

func TestNegotiator_InboundSBMSSPSilentlyIgnored(t *testing.T) {
	// Spec §8.3: the server NEVER receives MSSP subneg from a
	// well-behaved peer. A malformed crawler that sends one MUST
	// be silently ignored — the server must not crash, hang, or
	// reply.
	server, client := pairConn(t)

	got := make(chan string, 1)
	go func() {
		line, _ := server.Read(context.Background())
		got <- line
	}()

	_ = readBytes(t, client, 6)

	// Junk SB MSSP block followed by a normal line.
	_, _ = client.Write([]byte{negIAC, negSB, optMSSP, 'g', 'a', 'r', 'b', 'a', 'g', 'e', negIAC, negSE})
	_, _ = client.Write([]byte("ok\n"))

	if line := <-got; line != "ok" {
		t.Errorf("post-junk Read = %q, want %q", line, "ok")
	}
}

// readUntilIACSE reads from c until it sees IAC SE, returning the
// payload bytes (everything before the IAC SE). Bounded read so a
// runaway server doesn't hang the test.
func readUntilIACSE(t *testing.T, c net.Conn) []byte {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	var out []byte
	buf := make([]byte, 1)
	for len(out) < 4096 {
		n, err := c.Read(buf)
		if err != nil || n == 0 {
			t.Fatalf("readUntilIACSE: err=%v after %d bytes: %x", err, len(out), out)
		}
		out = append(out, buf[0])
		// Look for trailing IAC SE.
		if len(out) >= 2 && out[len(out)-2] == negIAC && out[len(out)-1] == negSE {
			return out[:len(out)-2]
		}
	}
	t.Fatalf("no IAC SE within 4096 bytes: %x", out)
	return nil
}

