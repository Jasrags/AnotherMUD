package scripting

import (
	"context"
	"fmt"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// Sandbox is a long-lived gopher-lua state owned by the Runtime.
// One Sandbox = one content-pack script: it runs the script body
// once at boot (registering handlers via the engine.* API), then
// stays alive to invoke those handlers when bus events fire.
//
// LStates are NOT safe for concurrent calls. Every Sandbox method
// that touches L (`Run`, `Call`, `Close`) takes the embedded mutex
// so the bus dispatcher and the boot loop don't race.
//
// Sandboxes inherit the Engine's Options for the per-invocation
// wall-clock timeout + memory caps. The same Sandbox is reused
// across many invocations; each invocation gets its own context
// deadline so a slow handler can't delay subsequent dispatches.
type Sandbox struct {
	mu     sync.Mutex
	L      *lua.LState
	opts   Options
	closed bool

	// Identity for error attribution. Set once at NewSandbox time;
	// every error surfaced by this sandbox carries it.
	packID     string
	scriptPath string
}

// NewSandbox returns a Sandbox tied to the supplied identity. The
// LState is constructed with the Engine's sandbox limits and the
// safe stdlib subset (table/string/math + filtered base) loaded.
// The engine.* API bindings (subscribe / log) are NOT installed
// here — the Runtime layers those on via Bind* methods below
// because they need bus / logger handles the scripting package
// doesn't own.
func (e *Engine) NewSandbox(packID, scriptPath string) *Sandbox {
	L := lua.NewState(lua.Options{
		SkipOpenLibs:        true,
		RegistrySize:        1024,
		RegistryMaxSize:     e.opts.MaxRegistrySize,
		RegistryGrowStep:    1024,
		CallStackSize:       e.opts.MaxCallStackSize,
		IncludeGoStackTrace: false,
	})
	openSafeLibs(L)
	return &Sandbox{
		L:          L,
		opts:       e.opts,
		packID:     packID,
		scriptPath: scriptPath,
	}
}

// PackID returns the sandbox's pack identity (for error
// attribution at the Runtime layer).
func (s *Sandbox) PackID() string { return s.packID }

// ScriptPath returns the sandbox's relative script path.
func (s *Sandbox) ScriptPath() string { return s.scriptPath }

// RawState exposes the underlying *lua.LState for binding setup
// (engine.subscribe / engine.log) at the Runtime layer. Callers
// MUST hold s.Lock() / s.Unlock() while touching the returned
// state to honor the per-LState concurrency contract.
//
// Exposed because the Sandbox can't itself know which Go-side
// bindings the host wants to install — that's the composition
// root's choice. Misuse risk is bounded: the scripting package
// is engine-internal and the only callers are the Runtime here.
func (s *Sandbox) RawState() *lua.LState { return s.L }

// Lock acquires the sandbox's internal mutex. Paired with Unlock.
// Callers binding Go-side LGFunctions or reading state must hold
// this lock — gopher-lua's LState is not safe for concurrent use.
func (s *Sandbox) Lock() { s.mu.Lock() }

// Unlock releases the sandbox's internal mutex.
func (s *Sandbox) Unlock() { s.mu.Unlock() }

// Run executes script in this sandbox with the Engine's per-Run
// timeout. Used at boot for the registration pass. Returns
// either nil, context.DeadlineExceeded / Canceled (timeout or
// parent ctx cancel), or *Error wrapping a Lua error with
// attribution.
//
// recover() wraps DoString: a Go-side panic from a binding LGFunction
// (engine.subscribe / log / future API) is converted to an error
// so the caller's goroutine stays alive. Closes the M17.1a-
// deferred MEDIUM trigger.
func (s *Sandbox) Run(ctx context.Context, script string) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("scripting: sandbox closed (pack=%s script=%s)",
			s.packID, s.scriptPath)
	}

	defer func() {
		if r := recover(); r != nil {
			err = &Error{
				PackID:     s.packID,
				ScriptPath: s.scriptPath,
				Cause:      fmt.Errorf("panic: %v", r),
			}
		}
	}()

	runCtx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()
	s.L.SetContext(runCtx)
	if doErr := s.L.DoString(script); doErr != nil {
		if cErr := runCtx.Err(); cErr != nil {
			if pErr := ctx.Err(); pErr != nil {
				return pErr
			}
			return cErr
		}
		return &Error{
			PackID:     s.packID,
			ScriptPath: s.scriptPath,
			Cause:      doErr,
		}
	}
	return nil
}

// Call invokes fn with pre-built args under the sandbox lock + a
// fresh per-call context deadline. `nret` is the number of return
// values the caller expects to pop off the stack after the call
// returns (typically 0 for event handlers; the Runtime ignores
// returns).
//
// IMPORTANT: args MUST NOT include LValues whose construction
// touches the LState (e.g., tables built via s.L.NewTable
// followed by L.SetField with a __newindex metatable). For
// payloads built from per-call data, use CallWithArgs instead —
// it runs the builder closure inside the lock so any LState
// mutation is correctly serialized.
//
// Errors:
//   - context.DeadlineExceeded / Canceled when the call exceeds
//     the per-Call timeout or the parent ctx fires.
//   - *Error wrapping any Lua-side panic or runtime error, with
//     pack + script attribution attached.
//   - Recovered Go panics from LGFunctions become *Error too.
func (s *Sandbox) Call(ctx context.Context, fn *lua.LFunction, nret int, args ...lua.LValue) error {
	return s.CallWithArgs(ctx, fn, nret, func(*lua.LState) []lua.LValue {
		return args
	})
}

// CallWithArgs is the lock-correct form of Call for callers whose
// arguments are built from the target LState itself (e.g., a
// fresh `*lua.LTable` populated under the sandbox lock).
//
// build runs INSIDE the sandbox lock with the live LState; its
// return value becomes the argument list passed to the underlying
// CallByParam. This guarantees any LState mutation incidental to
// arg construction (table allocation, metatable lookups via
// SetField, etc.) is serialized against concurrent dispatch.
//
// Error semantics match Call.
func (s *Sandbox) CallWithArgs(ctx context.Context, fn *lua.LFunction, nret int, build func(L *lua.LState) []lua.LValue) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("scripting: sandbox closed (pack=%s script=%s)",
			s.packID, s.scriptPath)
	}

	defer func() {
		if r := recover(); r != nil {
			err = &Error{
				PackID:     s.packID,
				ScriptPath: s.scriptPath,
				Cause:      fmt.Errorf("panic: %v", r),
			}
		}
	}()

	args := build(s.L)

	callCtx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()
	s.L.SetContext(callCtx)

	pcallErr := s.L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    nret,
		Protect: true,
	}, args...)
	if pcallErr != nil {
		if cErr := callCtx.Err(); cErr != nil {
			if pErr := ctx.Err(); pErr != nil {
				return pErr
			}
			return cErr
		}
		return &Error{
			PackID:     s.packID,
			ScriptPath: s.scriptPath,
			Cause:      pcallErr,
		}
	}
	return nil
}

// Close releases the LState. Idempotent. After Close, every
// further Run / Call returns an error rather than panicking.
func (s *Sandbox) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.L.Close()
}
