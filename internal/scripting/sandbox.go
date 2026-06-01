package scripting

import (
	lua "github.com/yuin/gopher-lua"
)

// openSafeLibs loads the subset of the gopher-lua standard library
// that is safe for content scripts and strips dangerous globals
// from the base library.
//
// Loaded:
//   - base (filtered — see deniedBaseFuncs)
//   - table
//   - string
//   - math
//
// NOT loaded:
//   - io: filesystem access
//   - os: filesystem, time, exec
//   - debug: introspection escape (debug.sethook, debug.getinfo,
//     debug.setupvalue), can be used to bypass sandboxing
//   - package: dynamic loading via require, loadlib
//
// The base library is loaded but several globals are then deleted
// because OpenBase pulls in dofile / loadfile / load / loadstring
// (arbitrary code execution from arbitrary sources), collectgarbage
// (lets script mess with GC), and module / require (dynamic loading
// even when package isn't open). newproxy is a hidden low-level
// metatable primitive content has no business touching.
func openSafeLibs(L *lua.LState) {
	// Base — required for assert, error, pcall, type, tostring, etc.
	L.Push(L.NewFunction(lua.OpenBase))
	L.Call(0, 0)
	stripDeniedBaseFuncs(L)

	// Table, string, math — pure-data libraries with no I/O or
	// system access. Safe for content.
	L.Push(L.NewFunction(lua.OpenTable))
	L.Call(0, 0)
	L.Push(L.NewFunction(lua.OpenString))
	L.Call(0, 0)
	L.Push(L.NewFunction(lua.OpenMath))
	L.Call(0, 0)
}

// deniedBaseFuncs names the globals OpenBase registers that we
// reject as too dangerous (arbitrary code load) or too low-level
// (GC manipulation, environment escape, hidden internals).
var deniedBaseFuncs = []string{
	// Arbitrary code load — both string and file source.
	"dofile",
	"loadfile",
	"load",
	"loadstring",
	// GC manipulation — lets script disable / force collection.
	"collectgarbage",
	// Environment escape — deprecated in Lua 5.2+ but present in
	// gopher-lua, and lets a script swap out _ENV / _G.
	"getfenv",
	"setfenv",
	// Module loading — `require` is dangerous even without
	// OpenPackage because OpenBase registers stubs.
	"module",
	"require",
	// Low-level metatable backdoor used by Lua's OOP primitives.
	"newproxy",
	// Debug-print of internal registers; an info-leak channel.
	"_printregs",
	// print writes to stdout — replaced by engine.log when
	// M17.1c lands; for now strip so scripts can't bypass the
	// engine's log infrastructure.
	"print",
}

// stripDeniedBaseFuncs deletes the denied globals from the Lua
// state's global table. After this call, attempting to call any of
// them raises "attempt to call a nil value (global '...')" — a
// normal Lua error attributable to the script's source line.
func stripDeniedBaseFuncs(L *lua.LState) {
	for _, name := range deniedBaseFuncs {
		L.SetGlobal(name, lua.LNil)
	}
}
