package telnet

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
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
