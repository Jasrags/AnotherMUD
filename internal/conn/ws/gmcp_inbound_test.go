package ws

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/conn"
)

// *Conn satisfies the GMCP capability interface the session installs the
// inbound handler through.
var _ conn.GmcpConn = (*Conn)(nil)

func TestDispatchInboundGmcp(t *testing.T) {
	c := &Conn{id: "x"}

	var calls int
	var gotPkg, gotPayload string
	c.SetGmcpHandler(func(_ context.Context, pkg string, payload []byte) {
		calls++
		gotPkg, gotPayload = pkg, string(payload)
	})

	// A real package is dispatched.
	c.dispatchInboundGmcp(context.Background(), "Input.Complete", []byte(`{"line":"get sw"}`))
	if calls != 1 || gotPkg != "Input.Complete" || gotPayload != `{"line":"get sw"}` {
		t.Fatalf("dispatch: calls=%d pkg=%q payload=%q", calls, gotPkg, gotPayload)
	}

	// Core.Supports.* is handled internally (always-active WS) — never
	// dispatched to the package handler.
	c.dispatchInboundGmcp(context.Background(), "Core.Supports.Set", []byte(`["X"]`))
	if calls != 1 {
		t.Errorf("Core.Supports.Set should be skipped, got %d calls", calls)
	}

	// Empty package name is ignored.
	c.dispatchInboundGmcp(context.Background(), "", nil)
	if calls != 1 {
		t.Errorf("empty package should be skipped, got %d calls", calls)
	}

	// A nil handler is safe.
	c.SetGmcpHandler(nil)
	c.dispatchInboundGmcp(context.Background(), "Input.Complete", nil) // must not panic
}
