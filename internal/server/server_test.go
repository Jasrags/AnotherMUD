package server_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/conn/telnet"
	"github.com/Jasrags/AnotherMUD/internal/server"
)

// listenLoopback opens a TCP listener on a free loopback port.
func listenLoopback(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln
}

// runServer starts s.Serve in a goroutine and returns a shutdown func
// that cancels ctx and waits for Serve to return.
func runServer(t *testing.T, s *server.Server, ln net.Listener) (addr string, shutdown func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()

	shutdown = func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, server.ErrServerClosed) {
				t.Errorf("Serve returned unexpected error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("Serve did not return within 2s of cancel")
		}
	}
	return ln.Addr().String(), shutdown
}

func TestEchoSingleClient(t *testing.T) {
	s := &server.Server{Handler: server.EchoHandler}
	addr, shutdown := runServer(t, s, listenLoopback(t))
	defer shutdown()

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	r := bufio.NewReader(c)
	if _, err := r.ReadString('\n'); err != nil { // greeting
		t.Fatalf("read greeting: %v", err)
	}
	drainTelnetOffers(t, r)

	if _, err := c.Write([]byte("hello world\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if got, want := strings.TrimRight(line, "\r\n"), "hello world"; got != want {
		t.Fatalf("echo mismatch: got %q want %q", got, want)
	}

	// "quit" should produce "bye\r\n" and close.
	if _, err := c.Write([]byte("quit\r\n")); err != nil {
		t.Fatalf("write quit: %v", err)
	}
	bye, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read bye: %v", err)
	}
	if got := strings.TrimRight(bye, "\r\n"); got != "bye" {
		t.Fatalf("bye mismatch: got %q", got)
	}
}

func TestEchoConcurrentClients(t *testing.T) {
	s := &server.Server{Handler: server.EchoHandler}
	addr, shutdown := runServer(t, s, listenLoopback(t))
	defer shutdown()

	const clients = 20
	const lines = 5
	var wg sync.WaitGroup
	errs := make(chan error, clients)

	for i := range clients {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := net.Dial("tcp", addr)
			if err != nil {
				errs <- err
				return
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))

			r := bufio.NewReader(c)
			if _, err := r.ReadString('\n'); err != nil {
				errs <- err
				return
			}
			if err := drainTelnetOffersErr(r); err != nil {
				errs <- err
				return
			}

			for j := range lines {
				msg := "client-" + itoa(i) + "-line-" + itoa(j)
				if _, err := c.Write([]byte(msg + "\r\n")); err != nil {
					errs <- err
					return
				}
				echoed, err := r.ReadString('\n')
				if err != nil {
					errs <- err
					return
				}
				if got := strings.TrimRight(echoed, "\r\n"); got != msg {
					errs <- errors.New("echo mismatch: " + got + " != " + msg)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("client error: %v", err)
		}
	}
}

func TestOversizedLineIsRejected(t *testing.T) {
	s := &server.Server{Handler: server.EchoHandler}
	addr, shutdown := runServer(t, s, listenLoopback(t))
	defer shutdown()

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	r := bufio.NewReader(c)
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	drainTelnetOffers(t, r)

	// Send a payload larger than telnet.MaxLineBytes (1024) with no
	// newline, then a normal line. Server must reject the oversized
	// line, recover, and echo the next normal line.
	huge := strings.Repeat("A", 4096)
	if _, err := c.Write([]byte(huge + "\r\n")); err != nil {
		t.Fatalf("write huge: %v", err)
	}

	got, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read reject msg: %v", err)
	}
	if want := "input too long"; !strings.Contains(got, want) {
		t.Fatalf("expected reject message containing %q, got %q", want, got)
	}

	if _, err := c.Write([]byte("hi\r\n")); err != nil {
		t.Fatalf("write normal: %v", err)
	}
	echo, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read echo after reject: %v", err)
	}
	if got := strings.TrimRight(echo, "\r\n"); got != "hi" {
		t.Fatalf("post-recovery echo mismatch: got %q", got)
	}
}

func TestServeReturnsAfterContextCancel(t *testing.T) {
	s := &server.Server{Handler: server.EchoHandler}
	ln := listenLoopback(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()

	// Give Serve a moment to actually start accepting.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, server.ErrServerClosed) {
			t.Fatalf("expected ErrServerClosed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}

// Tiny itoa to avoid pulling strconv into the test for one call.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// drainTelnetOffers reads the initial IAC sequence the telnet
// negotiator emits on every connection's first server-side Read:
// IAC DO TTYPE + IAC DO NAWS + IAC WILL GMCP (telnet.InitialOfferBytes
// total). The echo-style tests that pre-date negotiation MUST drop
// these bytes before reading the application-layer reply or string
// comparisons see "����reply" instead of "reply".
func drainTelnetOffers(t *testing.T, r *bufio.Reader) {
	t.Helper()
	if err := drainTelnetOffersErr(r); err != nil {
		t.Fatalf("drain telnet offers: %v", err)
	}
}

// drainTelnetOffersErr is the goroutine-friendly twin of
// drainTelnetOffers: returns the error instead of calling Fatalf
// so callers running off the main test goroutine can route it
// through their own error channel.
func drainTelnetOffersErr(r *bufio.Reader) error {
	_, err := r.Discard(telnet.InitialOfferBytes)
	return err
}
