package property

import "fmt"

// TaggedValue is the on-disk envelope for an unknown-to-registry
// property value (spec §4.4 step 3). Carries an explicit `kind`
// discriminator so the loader can coerce back to the right
// primitive type even when the registry has lost the entry.
//
// Per PD-2 (locked): only primitives travel through the envelope.
// Complex shapes (lists, maps) fall back to whatever the
// underlying format serializes naturally.
type TaggedValue struct {
	Kind  string      `yaml:"kind"`
	Value interface{} `yaml:"value"`
}

// Wrap returns a TaggedValue carrying v with the appropriate kind
// string. Returns an error for non-primitive values — the envelope
// is for primitives only.
func Wrap(v interface{}) (TaggedValue, error) {
	switch x := v.(type) {
	case string:
		return TaggedValue{Kind: "string", Value: x}, nil
	case int:
		return TaggedValue{Kind: "int", Value: x}, nil
	case int64:
		return TaggedValue{Kind: "int64", Value: x}, nil
	case float64:
		return TaggedValue{Kind: "float64", Value: x}, nil
	case bool:
		return TaggedValue{Kind: "bool", Value: x}, nil
	default:
		return TaggedValue{}, fmt.Errorf("property.Wrap: unsupported type %T", v)
	}
}

// IsTagged reports whether v looks like a tagged envelope. Accepts
// both the strongly-typed TaggedValue and a map[string]any with
// `kind` + `value` keys (YAML's default decode produces the map).
func IsTagged(v interface{}) bool {
	switch x := v.(type) {
	case TaggedValue:
		return x.Kind != ""
	case map[string]interface{}:
		_, hasKind := x["kind"]
		_, hasVal := x["value"]
		return hasKind && hasVal
	case map[interface{}]interface{}:
		_, hasKind := x["kind"]
		_, hasVal := x["value"]
		return hasKind && hasVal
	}
	return false
}

// Unwrap returns the unwrapped value AND the kind string of the
// deepest non-tagged inner value. Per spec §4.5 step 2, accidental
// nested envelopes (`{kind, value: {kind, value: ...}}`) are
// collapsed by walking inward until a non-tagged value is reached;
// the kind returned is the kind of the deepest tag (the most
// authoritative type assertion).
//
// Returns (originalValue, "", false) when v is not a tagged
// envelope at all. nil tolerance and double-wrapping are both
// silent — recoverable bugs in prior serializers must not break
// today's loader.
func Unwrap(v interface{}) (inner interface{}, kind string, wasTagged bool) {
	current := v
	deepestKind := ""
	tagged := false
	for {
		k, inner, ok := unwrapOne(current)
		if !ok {
			return current, deepestKind, tagged
		}
		tagged = true
		deepestKind = k
		current = inner
	}
}

// unwrapOne returns (kind, innerValue, true) if v is a tagged
// envelope (either TaggedValue or a map literal); else
// ("", v, false). One level of unwrapping.
func unwrapOne(v interface{}) (string, interface{}, bool) {
	switch x := v.(type) {
	case TaggedValue:
		if x.Kind == "" {
			return "", v, false
		}
		return x.Kind, x.Value, true
	case map[string]interface{}:
		k, hasKind := x["kind"]
		val, hasVal := x["value"]
		if !hasKind || !hasVal {
			return "", v, false
		}
		ks, _ := k.(string)
		if ks == "" {
			return "", v, false
		}
		return ks, val, true
	case map[interface{}]interface{}:
		k, hasKind := x["kind"]
		val, hasVal := x["value"]
		if !hasKind || !hasVal {
			return "", v, false
		}
		ks, _ := k.(string)
		if ks == "" {
			return "", v, false
		}
		return ks, val, true
	}
	return "", v, false
}
