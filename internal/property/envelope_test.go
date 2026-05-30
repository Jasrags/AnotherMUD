package property

import "testing"

func TestWrap_PrimitivesEach(t *testing.T) {
	cases := []struct {
		v        interface{}
		wantKind string
	}{
		{"hello", "string"},
		{42, "int"},
		{int64(9_000_000_000), "int64"},
		{3.14, "float64"},
		{true, "bool"},
	}
	for _, c := range cases {
		tv, err := Wrap(c.v)
		if err != nil {
			t.Errorf("Wrap(%v): %v", c.v, err)
			continue
		}
		if tv.Kind != c.wantKind {
			t.Errorf("Wrap(%v).Kind = %q, want %q", c.v, tv.Kind, c.wantKind)
		}
		if tv.Value != c.v {
			t.Errorf("Wrap(%v).Value = %v", c.v, tv.Value)
		}
	}
}

func TestWrap_UnsupportedType(t *testing.T) {
	if _, err := Wrap([]int{1, 2, 3}); err == nil {
		t.Error("Wrap([]int): want error")
	}
	if _, err := Wrap(map[string]string{"a": "b"}); err == nil {
		t.Error("Wrap(map): want error")
	}
	if _, err := Wrap(nil); err == nil {
		t.Error("Wrap(nil): want error")
	}
}

func TestIsTagged(t *testing.T) {
	if !IsTagged(TaggedValue{Kind: "int", Value: 1}) {
		t.Error("TaggedValue: want tagged")
	}
	if !IsTagged(map[string]interface{}{"kind": "int", "value": 1}) {
		t.Error("map[string]any: want tagged")
	}
	if !IsTagged(map[interface{}]interface{}{"kind": "int", "value": 1}) {
		t.Error("map[any]any: want tagged")
	}
	if IsTagged(TaggedValue{}) {
		t.Error("empty TaggedValue: not tagged")
	}
	if IsTagged(map[string]interface{}{"value": 1}) {
		t.Error("missing kind: not tagged")
	}
	if IsTagged(map[string]interface{}{"kind": "int"}) {
		t.Error("missing value: not tagged")
	}
	if IsTagged("plain string") {
		t.Error("plain string: not tagged")
	}
	if IsTagged(42) {
		t.Error("plain int: not tagged")
	}
}

func TestUnwrap_TaggedValue(t *testing.T) {
	tv := TaggedValue{Kind: "int", Value: 42}
	inner, kind, ok := Unwrap(tv)
	if !ok || kind != "int" || inner != 42 {
		t.Errorf("Unwrap(TaggedValue) = (%v, %q, %v)", inner, kind, ok)
	}
}

func TestUnwrap_MapStringInterface(t *testing.T) {
	v := map[string]interface{}{"kind": "string", "value": "hello"}
	inner, kind, ok := Unwrap(v)
	if !ok || kind != "string" || inner != "hello" {
		t.Errorf("Unwrap(map) = (%v, %q, %v)", inner, kind, ok)
	}
}

func TestUnwrap_MapInterfaceInterface(t *testing.T) {
	v := map[interface{}]interface{}{"kind": "bool", "value": true}
	inner, kind, ok := Unwrap(v)
	if !ok || kind != "bool" || inner != true {
		t.Errorf("Unwrap(yaml-shape map) = (%v, %q, %v)", inner, kind, ok)
	}
}

func TestUnwrap_NestedSelfHealing(t *testing.T) {
	// Spec §4.5 step 2: double-wrapped values must unwrap to the
	// deepest non-tagged inner value, using the deepest kind tag.
	inner := map[string]interface{}{
		"kind":  "int",
		"value": 42,
	}
	outer := map[string]interface{}{
		"kind":  "float64", // wrong kind on outer — deepest wins
		"value": inner,
	}
	got, kind, ok := Unwrap(outer)
	if !ok {
		t.Fatal("nested: want tagged")
	}
	if kind != "int" {
		t.Errorf("nested kind = %q, want int (deepest tag wins)", kind)
	}
	if got != 42 {
		t.Errorf("nested inner = %v, want 42", got)
	}
}

func TestUnwrap_NotTagged(t *testing.T) {
	inner, kind, ok := Unwrap("plain string")
	if ok {
		t.Error("plain string: want not tagged")
	}
	if inner != "plain string" {
		t.Errorf("inner = %v, want unchanged", inner)
	}
	if kind != "" {
		t.Errorf("kind = %q, want empty", kind)
	}
}

func TestUnwrap_EmptyKindIsNotTagged(t *testing.T) {
	v := map[string]interface{}{"kind": "", "value": 1}
	_, _, ok := Unwrap(v)
	if ok {
		t.Error("empty kind: should NOT be treated as tagged")
	}
}
