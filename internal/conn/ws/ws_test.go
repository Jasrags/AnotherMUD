package ws_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/conn/ws"
)

// dialServer spins up an httptest.Server that upgrades any
// request to a websocket, hands the server-side ws.Conn to the
// supplied handler, and returns a client-side *websocket.Conn the
// test drives.
func dialServer(t *testing.T, handler func(t *testing.T, c *ws.Conn)) (*websocket.Conn, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverWS, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
			OriginPatterns:     []string{"*"},
		})
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		defer serverWS.Close(websocket.StatusInternalError, "test cleanup")
		c := ws.New("test-1", serverWS)
		defer c.Close()
		handler(t, c)
	}))
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	clientWS, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("Dial: %v", err)
	}
	return clientWS, srv
}

func TestConn_ID(t *testing.T) {
	clientWS, srv := dialServer(t, func(t *testing.T, c *ws.Conn) {
		if c.ID() != "test-1" {
			t.Errorf("ID = %q, want test-1", c.ID())
		}
		// Wait for client close so handler exits cleanly.
		_, _ = c.Read(context.Background())
	})
	defer srv.Close()
	clientWS.Close(websocket.StatusNormalClosure, "test done")
}

func TestConn_Write_EmitsTextEnvelope(t *testing.T) {
	written := make(chan struct{})
	clientWS, srv := dialServer(t, func(t *testing.T, c *ws.Conn) {
		if _, err := c.Write(context.Background(), []byte("hello\r\n")); err != nil {
			t.Errorf("Write: %v", err)
		}
		close(written)
		_, _ = c.Read(context.Background())
	})
	defer srv.Close()
	defer clientWS.Close(websocket.StatusNormalClosure, "")

	<-written
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	mtype, data, err := clientWS.Read(ctx)
	if err != nil {
		t.Fatalf("client Read: %v", err)
	}
	if mtype != websocket.MessageText {
		t.Errorf("frame type = %v, want text", mtype)
	}
	var env struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != "text" {
		t.Errorf("envelope type = %q, want text", env.Type)
	}
	if env.Data != "hello\r\n" {
		t.Errorf("envelope data = %q, want hello\\r\\n", env.Data)
	}
}

func TestConn_SendGmcp_EmitsGmcpEnvelope(t *testing.T) {
	sent := make(chan struct{})
	clientWS, srv := dialServer(t, func(t *testing.T, c *ws.Conn) {
		if !c.GmcpActive() {
			t.Errorf("GmcpActive should be true for WS")
		}
		payload := []byte(`{"hp":50,"maxhp":100}`)
		if err := c.SendGmcp(context.Background(), "Char.Vitals", payload); err != nil {
			t.Errorf("SendGmcp: %v", err)
		}
		close(sent)
		_, _ = c.Read(context.Background())
	})
	defer srv.Close()
	defer clientWS.Close(websocket.StatusNormalClosure, "")

	<-sent
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, data, err := clientWS.Read(ctx)
	if err != nil {
		t.Fatalf("client Read: %v", err)
	}
	var env struct {
		Type    string          `json:"type"`
		Package string          `json:"package"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != "gmcp" {
		t.Errorf("envelope type = %q, want gmcp", env.Type)
	}
	if env.Package != "Char.Vitals" {
		t.Errorf("envelope package = %q, want Char.Vitals", env.Package)
	}
	if string(env.Data) != `{"hp":50,"maxhp":100}` {
		t.Errorf("envelope data = %s, want raw payload", env.Data)
	}
}

func TestConn_Read_ReturnsCommandData(t *testing.T) {
	got := make(chan string, 1)
	clientWS, srv := dialServer(t, func(t *testing.T, c *ws.Conn) {
		line, err := c.Read(context.Background())
		if err != nil {
			t.Errorf("Read: %v", err)
		}
		got <- line
		// Hold the handler open until the client closes.
		_, _ = c.Read(context.Background())
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	envelope := []byte(`{"type":"command","data":"look"}`)
	if err := clientWS.Write(ctx, websocket.MessageText, envelope); err != nil {
		t.Fatalf("client Write: %v", err)
	}
	line := <-got
	if line != "look" {
		t.Errorf("Read line = %q, want look", line)
	}
	clientWS.Close(websocket.StatusNormalClosure, "test done")
}

func TestConn_Read_SkipsUnknownTypes(t *testing.T) {
	// Send three envelopes: text (ignored), unknown (ignored),
	// command (returned). Read() must skip past the first two.
	got := make(chan string, 1)
	clientWS, srv := dialServer(t, func(t *testing.T, c *ws.Conn) {
		line, err := c.Read(context.Background())
		if err != nil {
			t.Errorf("Read: %v", err)
		}
		got <- line
		_, _ = c.Read(context.Background())
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for _, env := range []string{
		`{"type":"text","data":"client text not used"}`,
		`{"type":"frobnicate","data":"future feature"}`,
		`{"type":"command","data":"north"}`,
	} {
		if err := clientWS.Write(ctx, websocket.MessageText, []byte(env)); err != nil {
			t.Fatalf("client Write: %v", err)
		}
	}

	line := <-got
	if line != "north" {
		t.Errorf("Read line = %q, want north", line)
	}
	clientWS.Close(websocket.StatusNormalClosure, "")
}

func TestConn_Read_SkipsMalformedJSON(t *testing.T) {
	got := make(chan string, 1)
	clientWS, srv := dialServer(t, func(t *testing.T, c *ws.Conn) {
		line, _ := c.Read(context.Background())
		got <- line
		_, _ = c.Read(context.Background())
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := clientWS.Write(ctx, websocket.MessageText, []byte(`not json`)); err != nil {
		t.Fatalf("client Write garbage: %v", err)
	}
	if err := clientWS.Write(ctx, websocket.MessageText, []byte(`{"type":"command","data":"valid"}`)); err != nil {
		t.Fatalf("client Write valid: %v", err)
	}
	if line := <-got; line != "valid" {
		t.Errorf("Read line = %q, want valid", line)
	}
	clientWS.Close(websocket.StatusNormalClosure, "")
}

func TestConn_Read_ReturnsEOFOnNormalClose(t *testing.T) {
	done := make(chan error, 1)
	clientWS, srv := dialServer(t, func(t *testing.T, c *ws.Conn) {
		_, err := c.Read(context.Background())
		done <- err
	})
	defer srv.Close()

	clientWS.Close(websocket.StatusNormalClosure, "client done")
	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Errorf("Read after close = %v, want io.EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Read did not return after client close")
	}
}

func TestConn_Read_ContextCancelReturnsError(t *testing.T) {
	done := make(chan error, 1)
	clientWS, srv := dialServer(t, func(t *testing.T, c *ws.Conn) {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Read(ctx)
			done <- err
		}()
		// Give the Read a moment to block.
		time.Sleep(50 * time.Millisecond)
		cancel()
		wg.Wait()
	})
	defer srv.Close()
	defer clientWS.Close(websocket.StatusNormalClosure, "")

	select {
	case err := <-done:
		// A cancelled Read should surface a non-nil error. The
		// exact value is either ctx.Err or a wrapped form from
		// coder/websocket — both are fine, just non-nil.
		if err == nil {
			t.Errorf("Read after cancel = nil, want non-nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Read did not return after ctx cancel")
	}
}

func TestConn_SupportsPackage_AlwaysTrue(t *testing.T) {
	clientWS, srv := dialServer(t, func(t *testing.T, c *ws.Conn) {
		for _, pkg := range []string{"Char.Vitals", "Char.Effects", "Made.Up"} {
			if !c.SupportsPackage(pkg) {
				t.Errorf("SupportsPackage(%q) = false, want true", pkg)
			}
		}
		_, _ = c.Read(context.Background())
	})
	defer srv.Close()
	clientWS.Close(websocket.StatusNormalClosure, "")
}

func TestConn_ImplementsConnectionInterface(t *testing.T) {
	// Compile-time + runtime assertion that ws.Conn satisfies
	// conn.Connection. Catches API drift the moment it lands.
	var _ conn.Connection = (*ws.Conn)(nil)
}
