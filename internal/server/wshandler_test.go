package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/server"
)

// echoConnHandler reads one line from the conn and writes it back
// — a minimal handler the WS upgrade integration test can drive.
func echoConnHandler(ctx context.Context, c conn.Connection) error {
	line, err := c.Read(ctx)
	if err != nil {
		return err
	}
	_, err = c.Write(ctx, []byte("echo: "+line))
	return err
}

func TestWebSocketHandler_AcceptAndRoundTrip(t *testing.T) {
	// Spin up the WS handler against an httptest.Server, dial it,
	// send one command envelope, expect one text envelope back.
	srv := &server.Server{Handler: echoConnHandler}
	httpSrv := httptest.NewServer(server.NewWebSocketHandler(srv, server.WebSocketOptions{
		InsecureSkipVerify: true,
		OriginPatterns:     []string{"*"},
	}))
	defer httpSrv.Close()

	wsURL := strings.Replace(httpSrv.URL, "http://", "ws://", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "test done")

	if err := c.Write(ctx, websocket.MessageText, []byte(`{"type":"command","data":"hi"}`)); err != nil {
		t.Fatalf("client Write: %v", err)
	}

	mtype, data, err := c.Read(ctx)
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
	if env.Type != "text" || env.Data != "echo: hi" {
		t.Errorf("envelope = %+v, want text/echo: hi", env)
	}
}

func TestWebSocketHandler_NoHandlerReturns500(t *testing.T) {
	srv := &server.Server{} // Handler nil
	httpSrv := httptest.NewServer(server.NewWebSocketHandler(srv, server.WebSocketOptions{}))
	defer httpSrv.Close()

	resp, err := http.Get(httpSrv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
}
