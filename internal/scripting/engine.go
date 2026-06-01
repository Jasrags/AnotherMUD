// Package scripting hosts content-pack scripts in a sandboxed Lua
// runtime (Theme D, M17.1a). Scripts run with no filesystem,
// network, or OS access; CPU is bounded per Run by a context
// deadline.
//
// Sandbox boundaries (M17.1a):
//   - Filesystem / OS / dynamic load: BLOCKED. os, io, debug,
//     package, dofile, loadfile, load, loadstring, require all
//     unavailable to scripts.
//   - CPU: BOUNDED. Per-Run timeout (default 50ms) interrupts the
//     VM between instructions via context cancellation.
//   - Call stack: BOUNDED. MaxCallStackSize caps recursion; a
//     non-tail-call infinite recursion raises "stack overflow"
//     within a few hundred microseconds.
//   - VM register stack: BOUNDED. MaxRegistrySize caps gopher-
//     lua's value-register growth.
//   - Heap (Lua table allocations): NOT directly bounded — the
//     wall-clock timeout is the practical defense. gopher-lua's
//     RegistryMaxSize does NOT cap Go-heap allocations behind Lua
//     tables, and its SetMx (which would) calls os.Exit on
//     overflow which is unacceptable in a server. A future slice
//     will add a real allocation counter; until then, keep the
//     default Timeout tight.
//
// This package owns only the SANDBOX SUBSTRATE — execute one
// script, catch errors, enforce limits. Pack discovery (M17.1b),
// the bus bridge (M17.1c), and the engine-side API (subscribe /
// log / room queries) layer on top in later milestones.
//
// Language pre-decision: gopher-lua per user choice. The Engine
// type does not expose *lua.LState directly so a future runtime
// swap (goja, Starlark) is a matter of re-implementing Engine.Run
// + the limited bindings here, not rewriting every script in the
// codebase.
package scripting

import (
	"context"
	"fmt"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Default sandbox limits. Tuned conservatively — content authors who
// genuinely need more should raise the Options field, not hot-patch
// the defaults.
const (
	// DefaultMaxRegistrySize bounds the Lua state's value table.
	// Each entry is one Lua value; 65536 entries comfortably hold
	// any reasonable content script while preventing pathological
	// growth. gopher-lua's NewRegistryMaxSize halts state growth
	// once this is exceeded (no os.Exit, unlike SetMx).
	DefaultMaxRegistrySize = 64 * 1024
	// DefaultMaxCallStackSize bounds Lua call recursion depth. 256
	// frames matches gopher-lua's documented default but is set
	// explicitly so we control it across runtime upgrades.
	DefaultMaxCallStackSize = 256
	// DefaultTimeout caps a single Run's wall-clock budget. Scripts
	// that need longer should be redesigned around events (subscribe
	// to a tick handler, do incremental work) rather than blocking
	// the runtime.
	DefaultTimeout = 50 * time.Millisecond
)

// Options configures the per-Engine sandbox. Zero values fall back
// to the Default* constants so callers can pass an empty Options
// for the safe defaults.
type Options struct {
	// MaxRegistrySize is the hard cap on gopher-lua's value
	// registry size. Exceeding it raises a Lua error rather than
	// aborting the process. Zero → DefaultMaxRegistrySize.
	MaxRegistrySize int

	// MaxCallStackSize bounds Lua call recursion depth. Zero →
	// DefaultMaxCallStackSize.
	MaxCallStackSize int

	// Timeout is the per-Run wall-clock budget enforced via
	// context cancellation. Zero → DefaultTimeout.
	Timeout time.Duration
}

// Engine hosts the sandboxed Lua runtime. M17.1a keeps Engine
// stateless beyond its options — every Run constructs a fresh
// LState — so concurrent Run calls are safe without internal
// locking. A pooling refactor can land later if profiling shows
// LState construction dominating; the API surface stays the same.
type Engine struct {
	opts Options
}

// New returns an Engine bound to opts. Zero-value fields fall back
// to the package-level Default* constants.
func New(opts Options) *Engine {
	if opts.MaxRegistrySize <= 0 {
		opts.MaxRegistrySize = DefaultMaxRegistrySize
	}
	if opts.MaxCallStackSize <= 0 {
		opts.MaxCallStackSize = DefaultMaxCallStackSize
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	return &Engine{opts: opts}
}

// Run executes script in a fresh sandboxed LState. packID and
// scriptPath are used only for error attribution; the runtime does
// not read them for any execution decision.
//
// The returned error is one of:
//   - nil: the script ran to completion within sandbox limits.
//   - context.DeadlineExceeded (wrapped): the per-Run Timeout fired.
//   - context.Canceled (wrapped): the parent ctx was cancelled.
//   - *Error: any other Lua compile/runtime failure, with PackID +
//     ScriptPath + the underlying gopher-lua error attached.
//
// Run is safe for concurrent use from multiple goroutines (no
// shared mutable Engine state, per-call LState).
func (e *Engine) Run(ctx context.Context, packID, scriptPath, script string) error {
	L := lua.NewState(lua.Options{
		// SkipOpenLibs keeps gopher-lua from auto-loading the full
		// stdlib (which includes os, io, debug, and the package
		// loader). We then load only the safe subset explicitly.
		SkipOpenLibs: true,
		// RegistryMaxSize caps the value registry so a script can't
		// grow it unboundedly. RegistrySize is the starting size;
		// RegistryGrowStep is the increment when growth is needed
		// up to the cap.
		RegistrySize:     1024,
		RegistryMaxSize:  e.opts.MaxRegistrySize,
		RegistryGrowStep: 1024,
		CallStackSize:    e.opts.MaxCallStackSize,
		// IncludeGoStackTrace=false keeps Lua errors from leaking
		// the Go-side stack to scripts (info-disclosure
		// minimization, and a cleaner Lua-side error message).
		IncludeGoStackTrace: false,
	})
	defer L.Close()

	openSafeLibs(L)

	// Bind a timeout context so the Lua VM's mainLoopWithContext
	// interrupts execution between instructions if the deadline
	// fires. The deadline is the min of e.opts.Timeout and any
	// deadline already on ctx, so a caller that wants a shorter
	// budget can pass a tighter ctx.
	runCtx, cancel := context.WithTimeout(ctx, e.opts.Timeout)
	defer cancel()
	L.SetContext(runCtx)

	if err := L.DoString(script); err != nil {
		// gopher-lua's ApiError doesn't implement Unwrap and only
		// populates Cause for syntax/file errors, so a context-
		// cancel during runtime surfaces as a plain ApiError with
		// the context message in Object. We detect cancellation
		// authoritatively by inspecting runCtx itself: if it's
		// done, the script was interrupted (not a real Lua bug).
		if ctxErr := runCtx.Err(); ctxErr != nil {
			// Prefer the parent context's error when it cancelled
			// first — distinguishes "caller cancelled" from
			// "timeout fired" for callers using errors.Is.
			if parentErr := ctx.Err(); parentErr != nil {
				return parentErr
			}
			return ctxErr
		}
		return &Error{
			PackID:     packID,
			ScriptPath: scriptPath,
			Cause:      err,
		}
	}
	return nil
}

// Error wraps a gopher-lua error with the pack + script
// attribution the runtime collected at Run time. Implementations
// of error must surface both the Lua-side message (which already
// includes file:line if the script was loaded from disk) and the
// engine-side pack/script identity so logs and admin tools can
// trace a fault back to its source.
type Error struct {
	PackID     string
	ScriptPath string
	Cause      error
}

// Error implements the error interface. Format:
//
//	scripting: pack=<pack> script=<path>: <lua error>
func (e *Error) Error() string {
	return fmt.Sprintf("scripting: pack=%s script=%s: %v",
		e.PackID, e.ScriptPath, e.Cause)
}

// Unwrap exposes the underlying Lua error so errors.Is / errors.As
// can dig past the attribution wrapper.
func (e *Error) Unwrap() error { return e.Cause }
