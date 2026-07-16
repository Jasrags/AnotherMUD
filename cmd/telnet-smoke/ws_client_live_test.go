//go:build unix

package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestLive_WebClientWSFrontDoor proves the P1 web client's connection path
// (clients/web): a real websocket client dials the RUNNING engine's `/mud`
// endpoint and receives the login banner as `{type:"text"}` envelopes — the exact
// frames the browser client parses. This guards the full server HTTP-upgrade +
// session path over WS, which the internal/conn/ws unit tests (a bare Conn behind
// httptest) don't exercise end to end.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_WebClientWSFrontDoor -v
func TestLive_WebClientWSFrontDoor(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	wsAddr := freePort(t)
	// bootEngine waits for the telnet port; the WS listener starts alongside it.
	bootEngine(t, map[string]string{
		"ANOTHERMUD_WS_ADDR":                 wsAddr,
		"ANOTHERMUD_WS_INSECURE_SKIP_VERIFY": "true", // the test client sends no Origin
	})

	url := "ws://" + wsAddr + "/mud"
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var c *websocket.Conn
	deadline := time.Now().Add(15 * time.Second)
	for {
		var err error
		c, _, err = websocket.Dial(ctx, url, nil)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dial %s: %v", url, err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	defer c.Close(websocket.StatusNormalClosure, "done")

	// Read frames; on the username prompt send a fresh username to reach the
	// PASSWORD prompt — the one place login emits telnet IAC echo-mask bytes.
	// Assert NO text frame carries a replacement char: a leaked IAC 0xFF byte
	// JSON-marshals to `�` and renders as garbage (`��`) in the browser.
	var acc strings.Builder
	sentUser, sawPassword := false, false
	for i := 0; i < 40 && !sawPassword; i++ {
		rctx, rcancel := context.WithTimeout(ctx, 5*time.Second)
		_, data, err := c.Read(rctx)
		rcancel()
		if err != nil {
			break
		}
		var env struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(data, &env) != nil || env.Type != "text" {
			continue // gmcp/unknown envelopes — the browser routes them to panels
		}
		var s string
		_ = json.Unmarshal(env.Data, &s)
		acc.WriteString(s)
		lower := strings.ToLower(acc.String())
		if !sentUser && strings.Contains(lower, "username") {
			sentUser = true
			if err := c.Write(ctx, websocket.MessageText, []byte(`{"type":"command","data":"WebProbe"}`)); err != nil {
				t.Fatalf("send username: %v", err)
			}
		}
		if sentUser && strings.Contains(lower, "password") {
			sawPassword = true
		}
	}
	if !sentUser {
		t.Fatalf("never saw the username prompt over WS:\n%s", acc.String())
	}
	if !sawPassword {
		t.Fatalf("never reached the password prompt over WS:\n%s", acc.String())
	}
	if strings.Contains(acc.String(), "�") {
		t.Fatalf("WS login stream carries a replacement char — leaked telnet IAC (the `��` bug):\n%q", acc.String())
	}
	t.Log("web-client WS login verified: username → password prompt, no leaked IAC bytes")
}
