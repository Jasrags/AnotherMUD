package scripting

import (
	"context"
	"log/slog"
	"sync"

	lua "github.com/yuin/gopher-lua"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/script"
)

// Runtime is the M17.1c bus bridge — it owns the long-lived
// Sandboxes for every pack script, exposes the `engine.subscribe`
// + `engine.log` API surface to Lua, and dispatches eventbus
// events to the registered Lua handlers.
//
// Lifecycle: `LoadRegistry` constructs Sandboxes for every script
// in the supplied script.Registry, runs each script body once
// (the registration pass), and wires bus subscriptions. `Close`
// unsubscribes + closes every Sandbox.
//
// Concurrency: bus events fire from many goroutines; each
// dispatch acquires the target Sandbox's lock so the per-LState
// "no concurrent calls" contract holds. The Runtime itself
// keeps a single mutex around its subscription map.
type Runtime struct {
	engine *Engine
	bus    *eventbus.Bus

	mu sync.Mutex
	// subs maps an eventbus name to the (sandbox, fn) pairs that
	// want delivery. One bus subscription per name is installed
	// lazily on the first engine.subscribe call.
	subs map[string][]subscription
	// unsubs holds the per-event-name unsubscribe closures the bus
	// returned. Cleared in Close.
	unsubs map[string]func()
	// sandboxes owns every long-lived LState. Index parallel to
	// the original script.Registry.All() entry order — kept so
	// Close releases them deterministically.
	sandboxes []*Sandbox
}

// subscription pairs a Lua handler function with the sandbox
// whose LState owns it. The sandbox handle is needed because
// every Call must take that LState's mutex; the function is
// stored by reference and stays alive as long as the LState
// does (gopher-lua's GC won't reclaim a Go-referenced function).
type subscription struct {
	sb *Sandbox
	fn *lua.LFunction
}

// NewRuntime returns a Runtime ready to LoadRegistry. The
// supplied Engine provides the sandbox limits + per-call
// timeout; the Bus is the event source the engine.subscribe
// binding routes through.
func NewRuntime(engine *Engine, bus *eventbus.Bus) *Runtime {
	return &Runtime{
		engine: engine,
		bus:    bus,
		subs:   make(map[string][]subscription),
		unsubs: make(map[string]func()),
	}
}

// LoadRegistry constructs a Sandbox per script.Entry, installs
// the engine.* API, runs the script body once to register
// handlers, and stashes the Sandbox for later dispatch. A
// failure on any one script aborts the load and returns the
// underlying *Error so the composition root can surface it.
//
// Successfully-loaded sandboxes from prior iterations stay
// alive until Close — so a half-loaded Runtime still cleans
// up correctly if the caller decides to abort.
func (r *Runtime) LoadRegistry(ctx context.Context, registry *script.Registry) error {
	if registry == nil {
		return nil
	}
	entries := registry.All()
	for _, e := range entries {
		sb := r.engine.NewSandbox(e.PackID, e.Path)
		r.bindEngineAPI(sb)
		if err := sb.Run(ctx, e.Source); err != nil {
			r.sandboxes = append(r.sandboxes, sb)
			return err
		}
		r.sandboxes = append(r.sandboxes, sb)
	}
	return nil
}

// Close unsubscribes every bus subscription the Runtime
// installed and closes every Sandbox. Safe to call more than
// once.
func (r *Runtime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, un := range r.unsubs {
		un()
	}
	r.unsubs = nil
	r.subs = nil
	for _, sb := range r.sandboxes {
		sb.Close()
	}
	r.sandboxes = nil
}

// bindEngineAPI installs the `engine` global table on sb's
// LState. Functions:
//
//   - engine.subscribe(name, fn) — record fn as a handler for
//     bus events named `name`. Lazily installs the underlying
//     bus subscription the first time a name is seen.
//   - engine.log(msg) — write msg through the structured logger
//     with pack + script attribution attached.
//
// Both bindings take Sandbox identity by closure capture so the
// Lua side never sees them; identity is stable for the sandbox
// lifetime so a script can't impersonate a different pack.
func (r *Runtime) bindEngineAPI(sb *Sandbox) {
	L := sb.RawState()
	tbl := L.NewTable()
	L.SetField(tbl, "subscribe", L.NewFunction(r.makeSubscribeFn(sb)))
	L.SetField(tbl, "log", L.NewFunction(r.makeLogFn(sb)))
	L.SetGlobal("engine", tbl)
}

// makeSubscribeFn returns the LGFunction backing engine.subscribe.
// Lua signature: engine.subscribe(name string, fn function).
//
// The closure captures the sandbox so the dispatcher knows which
// LState the function belongs to. Argument validation raises a
// Lua error (which bubbles back through pcall to the script) on
// bad input — the script's authoring tooling sees a clean
// "string expected" / "function expected" message.
func (r *Runtime) makeSubscribeFn(sb *Sandbox) lua.LGFunction {
	return func(L *lua.LState) int {
		// Use CheckType so a number argument doesn't silently
		// coerce to a string — engine.subscribe(123, fn) should
		// raise rather than register a handler under "123".
		L.CheckType(1, lua.LTString)
		name := L.ToString(1)
		fn := L.CheckFunction(2)
		r.addSubscription(name, sb, fn)
		return 0
	}
}

// addSubscription records (name, sandbox, fn). If this is the
// first subscription for `name`, the Runtime installs a bus
// subscription that dispatches to every registered handler for
// the name. Subsequent subscriptions for the same name reuse
// the existing bus subscription.
func (r *Runtime) addSubscription(name string, sb *Sandbox, fn *lua.LFunction) {
	r.mu.Lock()
	r.subs[name] = append(r.subs[name], subscription{sb: sb, fn: fn})
	_, alreadyHooked := r.unsubs[name]
	r.mu.Unlock()
	if alreadyHooked {
		return
	}
	un := r.bus.Subscribe(name, func(ctx context.Context, event eventbus.Event) {
		r.dispatch(ctx, name, event)
	})
	r.mu.Lock()
	// Defensive: another goroutine may have installed the hook
	// between our check and the bus.Subscribe call. The double-
	// hook would still work but would deliver each event twice
	// to the same handlers — eat the second subscription.
	if _, already := r.unsubs[name]; already {
		r.mu.Unlock()
		un()
		return
	}
	r.unsubs[name] = un
	r.mu.Unlock()
}

// dispatch fans an inbound bus event out to every registered
// handler for the event's name. Each handler runs under its own
// Sandbox lock + per-Call timeout, so a slow handler can't stall
// the next one. Errors are logged + the dispatcher continues —
// bus contract is "one bad listener can't degrade another."
func (r *Runtime) dispatch(ctx context.Context, name string, event eventbus.Event) {
	r.mu.Lock()
	subs := append([]subscription(nil), r.subs[name]...)
	r.mu.Unlock()

	payload := eventToLuaTable(event)
	for _, sub := range subs {
		// CallWithArgs builds the Lua table INSIDE the Sandbox
		// lock so any LState mutation incidental to arg
		// construction (table allocation, SetField metatable
		// lookups) is serialized against any concurrent dispatch
		// to the same Sandbox. Closes the M17.1c review MEDIUM
		// finding about the previous unlocked tableForSandbox
		// call site.
		err := sub.sb.CallWithArgs(ctx, sub.fn, 0, func(L *lua.LState) []lua.LValue {
			return []lua.LValue{
				lua.LString(name),
				buildLuaTable(L, payload),
			}
		})
		if err != nil {
			logging.From(ctx).Warn("scripting handler error",
				slog.String("event", "scripting.handler.err"),
				slog.String("pack", sub.sb.PackID()),
				slog.String("script", sub.sb.ScriptPath()),
				slog.String("bus_event", name),
				slog.Any("err", err))
		}
	}
}

// makeLogFn returns the LGFunction backing engine.log. Lua
// signature: engine.log(msg). Forwards to the structured logger
// with pack + script attribution.
func (r *Runtime) makeLogFn(sb *Sandbox) lua.LGFunction {
	return func(L *lua.LState) int {
		msg := L.CheckString(1)
		// No ctx available in the Lua-call boundary; use the
		// background logger. The pack/script attribution is the
		// load-bearing context here.
		logging.From(context.Background()).Info("scripting.log",
			slog.String("event", "scripting.log"),
			slog.String("pack", sb.PackID()),
			slog.String("script", sb.ScriptPath()),
			slog.String("msg", msg))
		return 0
	}
}
