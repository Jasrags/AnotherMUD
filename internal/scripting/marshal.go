package scripting

import (
	"reflect"
	"strings"

	lua "github.com/yuin/gopher-lua"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// marshalledPayload is a Go-side intermediate representation of
// an eventbus.Event captured outside any LState lock. The
// dispatcher walks the event's exported fields via reflection
// once per Publish and then builds a per-sandbox Lua table from
// the same intermediate — saves the reflection cost on the
// shared dispatch path and keeps LState-touching work behind the
// per-sandbox mutex.
type marshalledPayload map[string]any

// eventToLuaTable extracts an event's exported fields into a
// Go-side map keyed by snake_case field name. Strings, ints,
// int64s, and bools pass through directly; anything else stringifies
// via fmt.Sprintf to keep the Lua side simple.
//
// Returns nil for a nil event so the per-sandbox marshal can fall
// back to an empty table.
func eventToLuaTable(event eventbus.Event) marshalledPayload {
	if event == nil {
		return nil
	}
	val := reflect.ValueOf(event)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}
	t := val.Type()
	out := make(marshalledPayload, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		key := snakeCase(field.Name)
		fv := val.Field(i)
		out[key] = goValueToLuaCompatible(fv)
	}
	return out
}

// goValueToLuaCompatible returns a representation of v that
// tableForSandbox can push into a Lua table. Anything richer
// than a primitive becomes its String() form (LValue.Format
// equivalent) — fine for M17.1c where event payloads are flat
// records of strings / ids / counts.
func goValueToLuaCompatible(v reflect.Value) any {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(v.Uint())
	case reflect.Float32, reflect.Float64:
		return v.Float()
	default:
		// Named string types (entities.EntityID, world.RoomID, etc.)
		// land here when their kind is String wrapped in a type. The
		// String() above already handles String kinds; this fallback
		// catches the unusual cases.
		if v.CanInterface() {
			if s, ok := v.Interface().(interface{ String() string }); ok {
				return s.String()
			}
		}
		return v.String()
	}
}

// buildLuaTable materializes a marshalledPayload into a new Lua
// table on L. MUST be called with the owning Sandbox lock held;
// `L.SetField` calls `metaOp1` to check for a __newindex
// metatable, and `metaOp1` reads LState-internal fields. Today's
// freshly-allocated tables carry no metatable so the path is
// race-free in practice, but the dispatcher relies on the lock
// to guarantee that property across future gopher-lua revisions.
//
// Returns an empty (non-nil) table for a nil payload so the
// caller's pcall sees a valid `LTable` rather than a Go nil
// crashing through reflection.
func buildLuaTable(L *lua.LState, payload marshalledPayload) *lua.LTable {
	tbl := L.NewTable()
	if payload == nil {
		return tbl
	}
	for k, v := range payload {
		switch x := v.(type) {
		case string:
			L.SetField(tbl, k, lua.LString(x))
		case int64:
			L.SetField(tbl, k, lua.LNumber(x))
		case float64:
			L.SetField(tbl, k, lua.LNumber(x))
		case bool:
			L.SetField(tbl, k, lua.LBool(x))
		default:
			// Fallback: stringify.
			L.SetField(tbl, k, lua.LString(""))
		}
	}
	return tbl
}

// snakeCase converts CamelCase / PascalCase to snake_case so a
// Lua handler accessing `payload.mob_id` matches the marshalled
// key for the Go field `MobID`. ID-style suffixes get the
// expected `mob_id`, not `mob_i_d`.
func snakeCase(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(name) + 4)
	runes := []rune(name)
	for i, r := range runes {
		isUpper := r >= 'A' && r <= 'Z'
		if i > 0 && isUpper {
			prev := runes[i-1]
			prevUpper := prev >= 'A' && prev <= 'Z'
			next := rune(0)
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			nextLower := next >= 'a' && next <= 'z'
			// Insert separator when:
			//   - previous char is lowercase ("MobID" → "Mob_ID"), or
			//   - we're at the end of a run of uppercase that's
			//     followed by lowercase ("XMLParser" → "XML_Parser").
			if !prevUpper || nextLower {
				b.WriteByte('_')
			}
		}
		if isUpper {
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
