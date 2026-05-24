// Package slot owns the equipment-slot registry per
// inventory-equipment-items §3.1–§3.2. Slots are registered at boot
// by both the engine (baseline body slots like wield/head) and by
// content packs. Names are global — there is one wield slot in the
// world, not one per pack — so cross-pack collisions are real
// conflicts and surface as registration errors.
//
// Equip / unequip semantics (§3.3–§3.4) live with the inventory
// operations milestone; this package only defines the registry and
// the slot-key naming scheme they will consume.
package slot

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Scope identifies who registered a slot, for diagnostics (§3.1).
// EngineScope is the conventional value for engine-baseline slots;
// pack-registered slots carry the pack namespace.
type Scope string

const EngineScope Scope = "engine"

// Def is a registered slot. Name MUST be snake_case (lowercase
// letters, digits, and underscores, never starting with a digit or
// an underscore). Max is the slot capacity (non-negative); cap 1
// uses a bare slot key, cap > 1 uses "name:index" keys.
type Def struct {
	Name  string
	Label string
	Max   int
	Scope Scope
}

// Errors callers may distinguish at the boundary.
var (
	ErrInvalidName   = errors.New("slot name must be snake_case")
	ErrInvalidMax    = errors.New("slot max must be non-negative")
	ErrDuplicate     = errors.New("slot already registered")
	ErrNotFound      = errors.New("slot not registered")
	ErrInvalidKey    = errors.New("slot key malformed")
	ErrIndexOutOfMax = errors.New("slot index out of range for max")
)

// IsValidName reports whether s is snake_case per §3.1: lowercase
// letters, digits, and underscores; must start with a letter; must
// not end with an underscore; no consecutive underscores; non-empty.
// Hyphens are explicitly rejected.
func IsValidName(s string) bool {
	if s == "" {
		return false
	}
	prevUnderscore := false
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			prevUnderscore = false
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
			prevUnderscore = false
		case r == '_':
			if i == 0 || prevUnderscore {
				return false
			}
			prevUnderscore = true
		default:
			return false
		}
	}
	return !prevUnderscore
}

// BuildKey returns the slot key for a name/index pair given the
// slot's max capacity (§3.2). Cap-1 slots use the bare name; cap > 1
// slots use "name:index". Returns ErrIndexOutOfMax when index is
// outside [0, max).
//
// For cap-1 slots, callers SHOULD pass index 0; any other value is
// treated as ErrIndexOutOfMax so cap-1 callers can't accidentally
// produce "name:1" keys.
func BuildKey(name string, index, max int) (string, error) {
	if max <= 0 {
		return "", ErrIndexOutOfMax
	}
	if index < 0 || index >= max {
		return "", ErrIndexOutOfMax
	}
	if max == 1 {
		return name, nil
	}
	return name + ":" + strconv.Itoa(index), nil
}

// ParseKey returns the base name and index for a slot key. Cap-1
// keys (no ":") return index 0. Returns ErrInvalidKey on malformed
// input. ParseKey does NOT validate against a registered slot's max
// capacity — it is a pure string operation.
func ParseKey(key string) (name string, index int, err error) {
	if key == "" {
		return "", 0, ErrInvalidKey
	}
	i := strings.IndexByte(key, ':')
	if i < 0 {
		return key, 0, nil
	}
	if i == 0 || i == len(key)-1 {
		return "", 0, fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	idx, parseErr := strconv.Atoi(key[i+1:])
	if parseErr != nil || idx < 0 {
		return "", 0, fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return key[:i], idx, nil
}
