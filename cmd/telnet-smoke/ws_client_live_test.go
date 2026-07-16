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

	// Read frames until a text envelope carries the login banner (the account
	// username prompt). Everything the browser sees flows through this same path.
	var acc strings.Builder
	for i := 0; i < 25; i++ {
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
		if strings.Contains(strings.ToLower(acc.String()), "username") {
			t.Log("web-client WS front door verified: dialed /mud, received the login banner as {type:text}")
			return
		}
	}
	t.Fatalf("no login-banner text frame over WS; accumulated:\n%s", acc.String())
}
