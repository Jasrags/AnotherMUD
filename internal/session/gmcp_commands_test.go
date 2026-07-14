package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
)

// commandCatalogBytes builds the two role tiers of the marshalled catalog from
// the real builtins, mirroring what SetCommandCatalog stores.
func commandCatalogBytes(t *testing.T) (player, admin []byte) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	player, err := marshalCommandCatalog(r.Catalog(false))
	if err != nil {
		t.Fatalf("marshal player catalog: %v", err)
	}
	admin, err = marshalCommandCatalog(r.Catalog(true))
	if err != nil {
		t.Fatalf("marshal admin catalog: %v", err)
	}
	return player, admin
}

func commandFrames(fc *gmcpFakeConn) []gmcp.CharCommands {
	var out []gmcp.CharCommands
	for _, f := range fc.framesSnapshot() {
		if f.pkg != gmcp.PackageCharCommands {
			continue
		}
		var c gmcp.CharCommands
		_ = json.Unmarshal(f.payload, &c)
		out = append(out, c)
	}
	return out
}

func hasCatalogGroup(c gmcp.CharCommands, key string) bool {
	for _, cat := range c.Categories {
		if cat.Key == key {
			return true
		}
	}
	return false
}

func TestFlushGmcpCommands_NoSendBeforeActivation(t *testing.T) {
	a, fc := newGmcpActor("p-1", 50, 100)
	player, admin := commandCatalogBytes(t)
	// active=false (default): nothing goes out.
	a.flushGmcpCommands(context.Background(), player, admin, "admin")
	if got := commandFrames(fc); len(got) != 0 {
		t.Errorf("pre-activation flush emitted %d catalog frames, want 0", len(got))
	}
}

func TestFlushGmcpCommands_EmitOnce(t *testing.T) {
	a, fc := newGmcpActor("p-1", 50, 100)
	fc.setActive(true)
	player, admin := commandCatalogBytes(t)

	a.flushGmcpCommands(context.Background(), player, admin, "admin")
	a.flushGmcpCommands(context.Background(), player, admin, "admin") // second tick: no-op

	frames := commandFrames(fc)
	if len(frames) != 1 {
		t.Fatalf("emitted %d catalog frames, want exactly 1 (emit-once)", len(frames))
	}
	if len(frames[0].Categories) == 0 {
		t.Error("catalog frame carried no categories")
	}
	// A plain player must not see the admin group.
	if hasCatalogGroup(frames[0], "admin") {
		t.Error("player received the admin group")
	}
}

func TestFlushGmcpCommands_AdminTier(t *testing.T) {
	a, fc := newGmcpActor("p-admin", 50, 100)
	fc.setActive(true)
	a.roles = map[string]struct{}{"admin": {}}
	player, admin := commandCatalogBytes(t)

	a.flushGmcpCommands(context.Background(), player, admin, "admin")

	frames := commandFrames(fc)
	if len(frames) != 1 {
		t.Fatalf("emitted %d frames, want 1", len(frames))
	}
	if !hasCatalogGroup(frames[0], "admin") {
		t.Error("admin did not receive the admin group")
	}
}

func TestFlushGmcpCommands_ResetReEmits(t *testing.T) {
	a, fc := newGmcpActor("p-1", 50, 100)
	fc.setActive(true)
	player, admin := commandCatalogBytes(t)

	a.flushGmcpCommands(context.Background(), player, admin, "admin")
	a.resetGmcpCommandsShadow() // link-dead reattach
	a.flushGmcpCommands(context.Background(), player, admin, "admin")

	if got := commandFrames(fc); len(got) != 2 {
		t.Errorf("after reset, emitted %d frames, want 2 (re-emit on reattach)", len(got))
	}
}

func TestFlushGmcpCommands_UnsetCatalogNoSend(t *testing.T) {
	a, fc := newGmcpActor("p-1", 50, 100)
	fc.setActive(true)
	// nil payloads (SetCommandCatalog never called / marshal failed): no send.
	a.flushGmcpCommands(context.Background(), nil, nil, "admin")
	if got := commandFrames(fc); len(got) != 0 {
		t.Errorf("unset catalog emitted %d frames, want 0", len(got))
	}
}
