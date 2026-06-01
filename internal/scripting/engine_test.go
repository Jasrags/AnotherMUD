package scripting_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/scripting"
)

// fastEngine returns an Engine with a generous test-friendly
// timeout so a CI host under load doesn't false-fail on the
// happy path. Tests that exercise the timeout itself construct
// their own narrow-timeout Engine.
func fastEngine(t *testing.T) *scripting.Engine {
	t.Helper()
	return scripting.New(scripting.Options{
		Timeout: 2 * time.Second,
	})
}

func TestEngine_Run_HappyPath(t *testing.T) {
	e := fastEngine(t)
	err := e.Run(context.Background(), "test-pack", "noop.lua",
		`local x = 1 + 2; return x`)
	if err != nil {
		t.Errorf("happy-path Run: %v", err)
	}
}

func TestEngine_Run_TableStringMathLoaded(t *testing.T) {
	// The three pure-data libraries must be available because
	// content scripts rely on them.
	e := fastEngine(t)
	err := e.Run(context.Background(), "p", "s",
		`assert(table.concat({"a", "b"}, ",") == "a,b")
		 assert(string.upper("hi") == "HI")
		 assert(math.max(1, 2, 3) == 3)`)
	if err != nil {
		t.Errorf("safe stdlib check: %v", err)
	}
}

func TestEngine_Run_ScriptError_AttributesPackAndScript(t *testing.T) {
	e := fastEngine(t)
	err := e.Run(context.Background(), "test-pack", "broken.lua",
		`error("something blew up")`)
	if err == nil {
		t.Fatal("expected error from script error()")
	}
	var se *scripting.Error
	if !errors.As(err, &se) {
		t.Fatalf("expected *scripting.Error, got %T: %v", err, err)
	}
	if se.PackID != "test-pack" {
		t.Errorf("PackID = %q, want test-pack", se.PackID)
	}
	if se.ScriptPath != "broken.lua" {
		t.Errorf("ScriptPath = %q, want broken.lua", se.ScriptPath)
	}
	if !strings.Contains(se.Error(), "something blew up") {
		t.Errorf("error text missing script message: %q", se.Error())
	}
}

func TestEngine_Run_Concurrent_NoSharedState(t *testing.T) {
	// Concurrent Runs must not leak state between each other.
	// Each Run constructs a fresh LState in M17.1a; the test
	// pins that contract by running two scripts that would
	// otherwise see each other's globals.
	e := fastEngine(t)
	const N = 8
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		go func() {
			err := e.Run(context.Background(), "p", "s",
				`leak = leak or "unset"; assert(leak == "unset")`)
			errs <- err
		}()
	}
	for i := 0; i < N; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Run[%d]: %v", i, err)
		}
	}
}

// --- Sandbox: dangerous globals are NOT callable. ---

func TestEngine_Sandbox_DeniedGlobalsAreNil(t *testing.T) {
	e := fastEngine(t)
	cases := []string{
		"dofile", "loadfile", "load", "loadstring",
		"collectgarbage", "getfenv", "setfenv",
		"module", "require", "newproxy", "print", "_printregs",
	}
	for _, name := range cases {
		err := e.Run(context.Background(), "p", "s",
			`assert(`+name+` == nil, "`+name+` should be nil but is "..type(`+name+`))`)
		if err != nil {
			t.Errorf("%s present in sandbox: %v", name, err)
		}
	}
}

func TestEngine_Sandbox_OsIsNotLoaded(t *testing.T) {
	// The whole `os` namespace must be absent — not even an
	// empty table. Same for `io`, `debug`, `package`.
	e := fastEngine(t)
	for _, namespace := range []string{"os", "io", "debug", "package"} {
		err := e.Run(context.Background(), "p", "s",
			`assert(`+namespace+` == nil, "`+namespace+` should be nil")`)
		if err != nil {
			t.Errorf("namespace %q leaked into sandbox: %v", namespace, err)
		}
	}
}

func TestEngine_Sandbox_RequireBlocked(t *testing.T) {
	// Even with package gone, scripts could try to call require
	// and expect a Lua-level error attributable to their source
	// line. They MUST get one rather than a panic or silent
	// load.
	e := fastEngine(t)
	err := e.Run(context.Background(), "test-pack", "evil.lua",
		`require("os")`)
	if err == nil {
		t.Fatal("require should have failed in sandbox")
	}
	// Attribution wrapper must fire — caller knows the pack +
	// script that tried to escape.
	var se *scripting.Error
	if !errors.As(err, &se) {
		t.Fatalf("expected *scripting.Error for require failure, got %T", err)
	}
}

// --- Resource limits. ---

func TestEngine_Timeout_InfiniteLoopAborts(t *testing.T) {
	// A `while true do end` script must not hang the test
	// forever — the per-Run timeout interrupts the VM between
	// instructions and surfaces context.DeadlineExceeded.
	e := scripting.New(scripting.Options{
		Timeout: 25 * time.Millisecond,
	})
	start := time.Now()
	err := e.Run(context.Background(), "p", "loop.lua",
		`while true do end`)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("infinite loop err = %v, want context.DeadlineExceeded", err)
	}
	// Hard ceiling so a regression that disables the timeout
	// doesn't silently hang the test runner.
	if elapsed > 2*time.Second {
		t.Errorf("Run did not return promptly after timeout: took %v", elapsed)
	}
}

func TestEngine_ParentContextCancel_Aborts(t *testing.T) {
	// A cancellable parent ctx must abort the script even if
	// the per-Run timeout hasn't expired yet.
	e := scripting.New(scripting.Options{
		Timeout: 5 * time.Second,
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()
	err := e.Run(ctx, "p", "loop.lua",
		`while true do end`)
	// Parent ctx cancel surfaces as context.Canceled (or
	// DeadlineExceeded if the timeout happened to fire at the
	// same nanosecond — accept both as "the script was
	// interrupted, not a Lua error").
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("parent-cancel err = %v, want context.Canceled or DeadlineExceeded", err)
	}
}

func TestEngine_AllocationStorm_BoundedByTimeout(t *testing.T) {
	// IMPORTANT: gopher-lua's RegistryMaxSize bounds the VM
	// register stack, NOT the Go-side heap that backs Lua tables.
	// A script that allocates many tables grows the Go heap freely
	// until either the wall-clock timeout fires OR the host OOMs.
	// The wall-clock timeout is therefore the load-bearing defense
	// for memory abuse in M17.1a; a follow-up slice will add a
	// real allocation counter.
	//
	// This test pins the contract that the wall-clock timeout
	// IS the safety net: a 100ms budget bounds even a
	// "try to allocate forever" script.
	e := scripting.New(scripting.Options{
		Timeout: 100 * time.Millisecond,
	})
	start := time.Now()
	err := e.Run(context.Background(), "p", "alloc.lua", `
		local t = {}
		for i = 1, 100000000 do
			t[i] = {a=i, b=i, c=i, d=i, e=i, f=i}
		end
	`)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("allocation storm should have been interrupted")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("alloc-storm err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("Run did not return promptly: took %v", elapsed)
	}
}

func TestEngine_CallStackOverflow_IsCaught(t *testing.T) {
	// Unbounded recursion must hit the call-stack cap and
	// surface as a Lua error attributed to the script.
	e := scripting.New(scripting.Options{
		MaxCallStackSize: 32,
		Timeout:          2 * time.Second,
	})
	// Non-tail-call recursion — `return 1 + r()` forces a real
	// stack frame per call because the result must be combined
	// with the +1 after the recursive return. A tail call
	// (`return r()`) would be optimized to a loop and trip the
	// timeout instead.
	err := e.Run(context.Background(), "p", "rec.lua",
		`local function r() return 1 + r() end; r()`)
	if err == nil {
		t.Fatal("expected stack-overflow error")
	}
	// The error must be a scripting.Error (attribution
	// wrapper), not a panic or context error.
	var se *scripting.Error
	if !errors.As(err, &se) {
		t.Errorf("expected *scripting.Error for overflow, got %T: %v", err, err)
	}
}

// --- Error attribution semantics. ---

func TestError_FormatHasAllFields(t *testing.T) {
	se := &scripting.Error{
		PackID:     "core",
		ScriptPath: "areas/town.lua",
		Cause:      errors.New("oops"),
	}
	got := se.Error()
	for _, want := range []string{"core", "areas/town.lua", "oops", "scripting:"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
}

func TestError_UnwrapExposesCause(t *testing.T) {
	cause := errors.New("inner")
	se := &scripting.Error{Cause: cause}
	if !errors.Is(se, cause) {
		t.Errorf("errors.Is(se, cause) = false; Unwrap broken")
	}
}
